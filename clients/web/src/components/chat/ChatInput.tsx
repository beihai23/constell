import { useState, useRef, useCallback, useEffect, type KeyboardEvent, type ChangeEvent } from 'react';
import { useParams } from 'react-router';
import { toast } from 'sonner';
import { useConstellClient } from '@/hooks/useConstellClient';
import { useMessagesStore } from '@/stores/messagesStore';
import { useAuthStore } from '@/stores/authStore';
import { useUIStore } from '@/stores/uiStore';
import type { Attachment, MessageAck } from '@constell/sdk-js';
import { Plus, Send, X, FileText } from 'lucide-react';

// ---------------------------------------------------------------------------
// Constants
// ---------------------------------------------------------------------------

/** Per-file upload size cap (FILE-SIZE-1). 25 MB — aligns with typical
 *  MinIO/S3 presigned-PUT limits and keeps uploads snappy. */
const MAX_FILE_SIZE_MB = 25;
const MAX_FILE_SIZE = MAX_FILE_SIZE_MB * 1024 * 1024;

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

interface PendingFile {
  file: File;
  preview?: string; // object URL for images
  data: Uint8Array<ArrayBuffer>;
  uploadError?: string; // set when this file's upload failed (FILE-UPLOAD-2)
  uploading?: boolean; // true while the file is being uploaded (FILE-UPLOAD-1)
  progress?: number; // upload fraction 0..1 (FILE-UPLOAD-1)
}

// ---------------------------------------------------------------------------
// ChatInput — bottom input area with file upload
// ---------------------------------------------------------------------------

export function ChatInput() {
  const { channelId, peerId } = useParams();
  const client = useConstellClient();
  const appendChannelMessage = useMessagesStore((s) => s.appendChannelMessage);
  const removeChannelMessage = useMessagesStore((s) => s.removeChannelMessage);
  const appendDMMessage = useMessagesStore((s) => s.appendDMMessage);
  const removeDMMessage = useMessagesStore((s) => s.removeDMMessage);
  const setMessageStatus = useMessagesStore((s) => s.setMessageStatus);
  const removeMessageStatus = useMessagesStore((s) => s.removeMessageStatus);
  const user = useAuthStore((s) => s.user);
  const wsStatus = useUIStore((s) => s.wsStatus);

  const [content, setContent] = useState('');
  const [pendingFiles, setPendingFiles] = useState<PendingFile[]>([]);
  const [sending, setSending] = useState(false);
  const fileInputRef = useRef<HTMLInputElement>(null);
  const textareaRef = useRef<HTMLTextAreaElement>(null);

  const isConnected = wsStatus === 'CONNECTED';
  // A send target is required: a channel (channel view) or a peer (DM).
  // Without one (e.g. a community with no channel selected) there's nowhere to
  // deliver, so the input is disabled instead of silently no-oping on Enter.
  const hasTarget = Boolean(channelId || peerId);

  // Auto-grow textarea
  useEffect(() => {
    const ta = textareaRef.current;
    if (!ta) return;
    ta.style.height = 'auto';
    const maxH = 5 * 24; // ~5 lines
    ta.style.height = `${Math.min(ta.scrollHeight, maxH)}px`;
  }, [content]);

  // Cleanup object URLs
  useEffect(() => {
    return () => {
      pendingFiles.forEach((f) => {
        if (f.preview) URL.revokeObjectURL(f.preview);
      });
    };
  }, [pendingFiles]);

  // Open file picker
  const openFilePicker = useCallback(() => {
    fileInputRef.current?.click();
  }, []);

  // Handle file selection
  const handleFileSelect = useCallback(async (e: ChangeEvent<HTMLInputElement>) => {
    const files = e.target.files;
    if (!files) return;

    const newFiles: PendingFile[] = [];
    let rejected = 0;
    for (let i = 0; i < files.length; i++) {
      const file = files[i];
      if (file.size > MAX_FILE_SIZE) {
        rejected++;
        continue;
      }
      const buffer = await file.arrayBuffer();
      const data = new Uint8Array(buffer);
      const preview = file.type.startsWith('image/') ? URL.createObjectURL(file) : undefined;
      newFiles.push({ file, preview, data });
    }
    if (rejected > 0) {
      toast.error(
        rejected === 1
          ? `File exceeds the ${MAX_FILE_SIZE_MB} MB limit`
          : `${rejected} files exceed the ${MAX_FILE_SIZE_MB} MB limit`,
      );
    }
    if (newFiles.length > 0) {
      setPendingFiles((prev) => [...prev, ...newFiles]);
    }

    // Reset input so the same file can be re-selected
    e.target.value = '';
  }, []);

  // Remove pending file
  const removeFile = useCallback((index: number) => {
    setPendingFiles((prev) => {
      const removed = prev[index];
      if (removed?.preview) URL.revokeObjectURL(removed.preview);
      return prev.filter((_, i) => i !== index);
    });
  }, []);

  // Send message
  const handleSend = useCallback(async () => {
    // Nowhere to send (no channel selected, not a DM). The input is disabled in
    // this state, but guard regardless so send can't silently no-op.
    if (!channelId && !peerId) return;
    const trimmed = content.trim();
    if ((!trimmed && pendingFiles.length === 0) || sending) return;

    setSending(true);
    try {
      // 1. Upload pending files first so the optimistic bubble can render them.
      //    A per-file upload failure is surfaced on that preview and the file
      //    is skipped (FILE-UPLOAD-2) — it no longer fails the whole send.
      const fileIds: string[] = [];
      const attachments: Attachment[] = [];
      const failed: number[] = [];
      // Mark all pending files as uploading so previews show progress (FILE-UPLOAD-1).
      setPendingFiles((prev) => prev.map((p) => ({ ...p, uploading: true, progress: 0 })));
      await Promise.all(
        pendingFiles.map(async (pf, idx) => {
          try {
            const info = await client.uploadFile(
              pf.data,
              pf.file.name,
              pf.file.type,
              (fraction) => {
                setPendingFiles((prev) =>
                  prev.map((p, i) => (i === idx ? { ...p, progress: fraction } : p)),
                );
              },
            );
            fileIds.push(info.id);
            attachments.push({
              id: info.id,
              fileId: info.id,
              filename: info.filename,
              contentType: info.contentType,
              size: info.size,
              url: info.url,
              thumbnailUrl: pf.preview ?? info.thumbnailUrl,
            });
          } catch {
            failed.push(idx);
          }
        }),
      );
      if (failed.length > 0) {
        // Keep only the failed previews so the user can remove or re-send
        // them. Abort the whole send: dropping the failed files silently or
        // sending a partial message would both be surprising. (FILE-UPLOAD-2)
        setPendingFiles((prev) =>
          prev
            .filter((_, idx) => failed.includes(idx))
            .map((pf) => ({
              ...pf,
              uploading: false,
              progress: undefined,
              uploadError: 'Upload failed — remove or retry',
            })),
        );
        toast.error(`${failed.length} file(s) failed to upload`);
        return;
      }

      // 2. Optimistic insert. The ws-gateway ACK carries the created message's
      //    id + seq (no full body), and the notify-service only pushes to the
      //    *other* participants, so without inserting locally the sender would
      //    never see their own message. The temp id is reconciled to the real
      //    server id + seq on ACK (step 3) so the message sorts correctly and
      //    dedups against history / realtime.
      const tempId = `opt-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`;
      const createdAt = Date.now();
      const optimisticChannel = channelId
        ? {
            id: tempId,
            seq: 0,
            channelId,
            authorId: user?.id ?? '',
            content: trimmed,
            createdAt,
            updatedAt: createdAt,
            attachments,
          }
        : null;
      const optimisticDM = peerId
        ? {
            id: tempId,
            seq: 0,
            conversationId: '',
            senderId: user?.id ?? '',
            content: trimmed,
            createdAt,
            attachments,
          }
        : null;
      if (optimisticChannel && channelId) {
        appendChannelMessage(channelId, optimisticChannel);
      } else if (optimisticDM && peerId) {
        appendDMMessage(peerId, optimisticDM);
      }
      setMessageStatus(tempId, 'sending');

      // 3. Send message; on ACK reconcile the optimistic copy with the real
      //    server message (real id + authoritative seq).
      let ack: MessageAck | undefined;
      try {
        if (channelId) {
          ack = await client.sendChannelMessage(channelId, trimmed, fileIds.length > 0 ? fileIds : undefined);
        } else if (peerId) {
          ack = await client.sendDM(peerId, trimmed, fileIds.length > 0 ? fileIds : undefined);
        }
      } catch (sendErr) {
        setMessageStatus(tempId, 'failed');
        throw sendErr;
      }
      if (ack && ack.messageId && ack.messageId !== tempId) {
        if (optimisticChannel && channelId) {
          removeChannelMessage(channelId, tempId);
          appendChannelMessage(channelId, { ...optimisticChannel, id: ack.messageId, seq: ack.seq });
        } else if (optimisticDM && peerId) {
          removeDMMessage(peerId, tempId);
          appendDMMessage(peerId, { ...optimisticDM, id: ack.messageId, seq: ack.seq });
        }
        setMessageStatus(ack.messageId, 'sent');
        removeMessageStatus(tempId);
      } else {
        setMessageStatus(tempId, 'sent');
      }

      // 4. Clear input
      setContent('');
      setPendingFiles([]);
      if (textareaRef.current) {
        textareaRef.current.style.height = 'auto';
      }
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to send message');
    } finally {
      setSending(false);
    }
  }, [content, pendingFiles, sending, channelId, peerId, client, user?.id, appendChannelMessage, removeChannelMessage, appendDMMessage, removeDMMessage, setMessageStatus, removeMessageStatus]);

  // Keyboard handling: Enter sends, Shift+Enter adds newline
  const handleKeyDown = useCallback((e: KeyboardEvent<HTMLTextAreaElement>) => {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault();
      handleSend();
    }
  }, [handleSend]);

  return (
    <div className="shrink-0 px-4 pb-4 pt-2">
      {/* File preview area */}
      {pendingFiles.length > 0 && (
        <div className="mb-2 flex flex-wrap gap-2">
          {pendingFiles.map((pf, idx) => (
            <FilePreview key={idx} pendingFile={pf} onRemove={() => removeFile(idx)} />
          ))}
        </div>
      )}

      {/* Input container */}
      <div className="flex items-end gap-2 rounded-lg bg-[#313244] px-3 py-2">
        {/* Attach button */}
        <button
          className="flex h-8 w-8 shrink-0 items-center justify-center rounded text-[#585b70] transition-colors hover:bg-[#45475a] hover:text-[#cdd6f4]"
          onClick={openFilePicker}
          disabled={!isConnected || !hasTarget}
          aria-label="Attach file"
        >
          <Plus className="h-5 w-5" />
        </button>
        <input
          ref={fileInputRef}
          type="file"
          className="hidden"
          multiple
          onChange={handleFileSelect}
        />

        {/* Textarea */}
        <textarea
          ref={textareaRef}
          value={content}
          onChange={(e) => setContent(e.target.value)}
          onKeyDown={handleKeyDown}
          placeholder={
            !isConnected
              ? 'Connecting...'
              : !hasTarget
                ? 'Select a channel to message'
                : channelId
                  ? 'Message #channel'
                  : 'Message user'
          }
          disabled={!isConnected || !hasTarget}
          rows={1}
          className="max-h-[120px] min-h-[24px] flex-1 resize-none bg-transparent text-sm text-[#cdd6f4] placeholder:text-[#585b70] focus:outline-none disabled:opacity-50"
        />

        {/* Send button */}
        <button
          className="flex h-8 w-8 shrink-0 items-center justify-center rounded text-[#585b70] transition-colors hover:bg-[#45475a] hover:text-[#cdd6f4] disabled:opacity-50"
          onClick={handleSend}
          disabled={!isConnected || !hasTarget || sending || (!content.trim() && pendingFiles.length === 0)}
          aria-label="Send message"
        >
          <Send className="h-4 w-4" />
        </button>
      </div>
    </div>
  );
}

// ---------------------------------------------------------------------------
// FilePreview — thumbnail or file info card with remove button
// ---------------------------------------------------------------------------

function FilePreview({ pendingFile, onRemove }: { pendingFile: PendingFile; onRemove: () => void }) {
  const isImage = pendingFile.file.type.startsWith('image/');

  return (
    <div
      className={[
        'group relative flex items-center gap-2 rounded-lg border bg-[#181825] px-2 py-1.5',
        pendingFile.uploadError ? 'border-[#f38ba8]' : 'border-[#45475a]',
      ].join(' ')}
    >
      {isImage && pendingFile.preview ? (
        <img
          src={pendingFile.preview}
          alt={pendingFile.file.name}
          className="h-12 w-12 rounded object-cover"
        />
      ) : (
        <FileText className="h-6 w-6 shrink-0 text-[#585b70]" />
      )}
      <div className="min-w-0 flex-1">
        <p className="max-w-[120px] truncate text-xs text-[#cdd6f4]">{pendingFile.file.name}</p>
        <p className="text-xs text-[#585b70]">{formatFileSize(pendingFile.file.size)}</p>
        {pendingFile.uploading && (
          <div data-slot="upload-progress" className="mt-1 h-1.5 w-32 max-w-full overflow-hidden rounded-full bg-[#313244]" role="progressbar" aria-valuemin={0} aria-valuemax={100} aria-valuenow={Math.round((pendingFile.progress ?? 0) * 100)}>
            <div
              className="h-full rounded-full bg-[#cba6f7] transition-[width]"
              style={{ width: `${Math.round((pendingFile.progress ?? 0) * 100)}%` }}
            />
          </div>
        )}
        {pendingFile.uploadError && (
          <p
            role="alert"
            data-slot="upload-error"
            className="max-w-[140px] truncate text-xs text-[#f38ba8]"
          >
            {pendingFile.uploadError}
          </p>
        )}
      </div>
      <button
        className="absolute -right-1 -top-1 flex h-4 w-4 items-center justify-center rounded-full bg-[#f38ba8] text-[#11111b] opacity-0 transition-opacity group-hover:opacity-100"
        onClick={onRemove}
        aria-label="Remove file"
      >
        <X className="h-3 w-3" />
      </button>
    </div>
  );
}

function formatFileSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
}
