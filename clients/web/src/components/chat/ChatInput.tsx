import { useState, useRef, useCallback, useEffect, type KeyboardEvent, type ChangeEvent } from 'react';
import { useParams } from 'react-router';
import { toast } from 'sonner';
import { useConstellClient } from '@/hooks/useConstellClient';
import { useMessagesStore } from '@/stores/messagesStore';
import { useAuthStore } from '@/stores/authStore';
import { useUIStore } from '@/stores/uiStore';
import type { Attachment } from '@constell/sdk-js';
import { Plus, Send, X, FileText } from 'lucide-react';

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

interface PendingFile {
  file: File;
  preview?: string; // object URL for images
  data: Uint8Array<ArrayBuffer>;
}

// ---------------------------------------------------------------------------
// ChatInput — bottom input area with file upload
// ---------------------------------------------------------------------------

export function ChatInput() {
  const { channelId, peerId } = useParams();
  const client = useConstellClient();
  const appendChannelMessage = useMessagesStore((s) => s.appendChannelMessage);
  const appendDMMessage = useMessagesStore((s) => s.appendDMMessage);
  const setMessageStatus = useMessagesStore((s) => s.setMessageStatus);
  const user = useAuthStore((s) => s.user);
  const wsStatus = useUIStore((s) => s.wsStatus);

  const [content, setContent] = useState('');
  const [pendingFiles, setPendingFiles] = useState<PendingFile[]>([]);
  const [sending, setSending] = useState(false);
  const fileInputRef = useRef<HTMLInputElement>(null);
  const textareaRef = useRef<HTMLTextAreaElement>(null);

  const isConnected = wsStatus === 'CONNECTED';

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
    for (let i = 0; i < files.length; i++) {
      const file = files[i];
      const buffer = await file.arrayBuffer();
      const data = new Uint8Array(buffer);
      const preview = file.type.startsWith('image/') ? URL.createObjectURL(file) : undefined;
      newFiles.push({ file, preview, data });
    }
    setPendingFiles((prev) => [...prev, ...newFiles]);

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
    const trimmed = content.trim();
    if ((!trimmed && pendingFiles.length === 0) || sending) return;

    setSending(true);
    try {
      // 1. Upload pending files first so the optimistic bubble can render them.
      const fileIds: string[] = [];
      const attachments: Attachment[] = [];
      for (const pf of pendingFiles) {
        const info = await client.uploadFile(pf.data, pf.file.name, pf.file.type);
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
      }

      // 2. Optimistic insert. The ws-gateway returns a bare ACK (no message
      //    data) and notify-service only pushes to the *other* participants,
      //    so without inserting locally the sender would never see their own
      //    message. The temp id is keyed into messageStatus; on ACK it flips
      //    to 'sent', on failure to 'failed'.
      const tempId = `opt-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`;
      const createdAt = Date.now();
      if (channelId) {
        appendChannelMessage(channelId, {
          id: tempId,
          seq: 0,
          channelId,
          authorId: user?.id ?? '',
          content: trimmed,
          createdAt,
          updatedAt: createdAt,
          attachments,
        });
      } else if (peerId) {
        appendDMMessage(peerId, {
          id: tempId,
          seq: 0,
          conversationId: '',
          senderId: user?.id ?? '',
          content: trimmed,
          createdAt,
          attachments,
        });
      }
      setMessageStatus(tempId, 'sending');

      // 3. Send message; flip status based on the ACK / error.
      try {
        if (channelId) {
          await client.sendChannelMessage(channelId, trimmed, fileIds.length > 0 ? fileIds : undefined);
        } else if (peerId) {
          await client.sendDM(peerId, trimmed, fileIds.length > 0 ? fileIds : undefined);
        }
        setMessageStatus(tempId, 'sent');
      } catch (sendErr) {
        setMessageStatus(tempId, 'failed');
        throw sendErr;
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
  }, [content, pendingFiles, sending, channelId, peerId, client, user?.id, appendChannelMessage, appendDMMessage, setMessageStatus]);

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
          disabled={!isConnected}
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
              : channelId
                ? 'Message #channel'
                : peerId
                  ? 'Message user'
                  : 'Type a message...'
          }
          disabled={!isConnected}
          rows={1}
          className="max-h-[120px] min-h-[24px] flex-1 resize-none bg-transparent text-sm text-[#cdd6f4] placeholder:text-[#585b70] focus:outline-none disabled:opacity-50"
        />

        {/* Send button */}
        <button
          className="flex h-8 w-8 shrink-0 items-center justify-center rounded text-[#585b70] transition-colors hover:bg-[#45475a] hover:text-[#cdd6f4] disabled:opacity-50"
          onClick={handleSend}
          disabled={!isConnected || sending || (!content.trim() && pendingFiles.length === 0)}
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
    <div className="group relative flex items-center gap-2 rounded-lg border border-[#45475a] bg-[#181825] px-2 py-1.5">
      {isImage && pendingFile.preview ? (
        <img
          src={pendingFile.preview}
          alt={pendingFile.file.name}
          className="h-12 w-12 rounded object-cover"
        />
      ) : (
        <FileText className="h-6 w-6 shrink-0 text-[#585b70]" />
      )}
      <div className="min-w-0">
        <p className="max-w-[120px] truncate text-xs text-[#cdd6f4]">{pendingFile.file.name}</p>
        <p className="text-xs text-[#585b70]">{formatFileSize(pendingFile.file.size)}</p>
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
