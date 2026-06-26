import { useMemo, useState } from 'react';
import { useResolvedUser } from '@/hooks/useResolvedUser';
import type { ChannelMessage, DMMessage, Attachment } from '@constell/sdk-js';
import { Clock, AlertCircle, Image, FileText, Trash2, Pencil } from 'lucide-react';

// ---------------------------------------------------------------------------
// Props
// ---------------------------------------------------------------------------

interface MessageBubbleProps {
  message: ChannelMessage | DMMessage;
  isOwn: boolean;
  status?: 'sending' | 'sent' | 'failed';
  onRetry?: () => void;
  /** Show a delete affordance (only meaningful for own, channel messages). */
  canDelete?: boolean;
  onDelete?: () => void;
  /** Edit affordance (own channel messages only). */
  canEdit?: boolean;
  onEdit?: (content: string) => Promise<void>;
}

// ---------------------------------------------------------------------------
// Catppuccin Mocha color palette for usernames
// ---------------------------------------------------------------------------

const USERNAME_COLORS = [
  '#f38ba8', // red
  '#fab387', // peach
  '#f9e2af', // yellow
  '#a6e3a1', // green
  '#94e2d5', // teal
  '#89b4fa', // blue
  '#cba6f7', // mauve
  '#f5c2e7', // pink
];

/** Deterministic color from a user ID string. */
function usernameColor(userId: string): string {
  let hash = 0;
  for (let i = 0; i < userId.length; i++) {
    hash = userId.charCodeAt(i) + ((hash << 5) - hash);
  }
  return USERNAME_COLORS[Math.abs(hash) % USERNAME_COLORS.length];
}

/** Format timestamp to HH:MM. */
function formatTime(ts: number): string {
  const d = new Date(ts);
  return d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
}

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

export function MessageBubble({ message, isOwn, status, onRetry, canDelete, onDelete, canEdit, onEdit }: MessageBubbleProps) {
  const authorId = 'authorId' in message ? message.authorId : message.senderId;
  // Resolve the author's nickname lazily via REST (pull-based, cached in
  // usersStore). Falls back to the raw id while loading or if lookup fails,
  // so the bubble always renders something.
  const author = useResolvedUser(authorId);
  const authorName = author?.nickname || authorId;
  const color = useMemo(() => usernameColor(authorId), [authorId]);

  // Inline edit mode (MSG-EDIT-1). Editing is local to the bubble; commit
  // delegates to onEdit (which calls the SDK + updates the store).
  const [editing, setEditing] = useState(false);
  const [draft, setDraft] = useState(message.content);
  const [saving, setSaving] = useState(false);

  // A message is "edited" when its updated_at advanced past created_at.
  const updatedAt = 'updatedAt' in message ? message.updatedAt : undefined;
  const edited = typeof updatedAt === 'number' && updatedAt > message.createdAt;

  const startEdit = () => {
    setDraft(message.content);
    setEditing(true);
  };
  const cancelEdit = () => {
    setEditing(false);
    setDraft(message.content);
  };
  const saveEdit = async () => {
    if (!onEdit) return;
    const trimmed = draft.trim();
    if (!trimmed || trimmed === message.content || saving) {
      setEditing(false);
      return;
    }
    setSaving(true);
    try {
      await onEdit(trimmed);
      setEditing(false);
    } finally {
      setSaving(false);
    }
  };

  return (
    <div className="flex gap-3 px-4 py-0.5 hover:bg-[#1e1e2e]/50 group">
      {/* Avatar */}
      <div className="mt-0.5 shrink-0">
        <div className="flex h-10 w-10 items-center justify-center overflow-hidden rounded-full bg-[#313244]">
          <span className="text-sm font-semibold text-[#cdd6f4]">
            {authorName.charAt(0).toUpperCase()}
          </span>
        </div>
      </div>

      {/* Content */}
      <div className="min-w-0 flex-1">
        {/* Header row: username + timestamp + hover actions (own msgs) */}
        <div className="flex items-baseline gap-2">
          <span className="text-sm font-semibold" style={{ color }}>
            {authorName}
          </span>
          <span className="text-xs text-[#585b70]">
            {formatTime(message.createdAt)}
          </span>
          <div className="ml-auto -mt-0.5 flex items-center gap-0.5 opacity-0 transition-opacity group-hover:opacity-100">
            {canEdit && onEdit && !editing && (
              <button
                onClick={startEdit}
                aria-label="Edit message"
                title="Edit message"
                className="flex size-5 items-center justify-center rounded text-[#585b70] hover:bg-[#313244] hover:text-[#cdd6f4]"
              >
                <Pencil className="size-3" />
              </button>
            )}
            {canDelete && onDelete && (
              <button
                onClick={onDelete}
                aria-label="Delete message"
                title="Delete message"
                className="flex size-5 items-center justify-center rounded text-[#585b70] hover:bg-[#313244] hover:text-[#f38ba8]"
              >
                <Trash2 className="size-3" />
              </button>
            )}
          </div>
        </div>

        {/* Message body — inline editor when editing, else content */}
        {editing ? (
          <div className="mt-1">
            <textarea
              value={draft}
              onChange={(e) => setDraft(e.target.value)}
              autoFocus
              rows={2}
              className="w-full resize-none rounded-lg border border-[#313244] bg-[#11111b] px-2 py-1 text-sm text-[#cdd6f4] outline-none focus:border-[#585b70]"
            />
            <div className="mt-1 flex gap-2">
              <button
                onClick={saveEdit}
                disabled={saving || !draft.trim() || draft.trim() === message.content}
                className="rounded bg-[#cba6f7] px-2 py-0.5 text-xs font-medium text-[#11111b] disabled:opacity-50"
              >
                {saving ? 'Saving…' : 'Save'}
              </button>
              <button
                onClick={cancelEdit}
                disabled={saving}
                className="rounded px-2 py-0.5 text-xs text-[#585b70] hover:text-[#cdd6f4]"
              >
                Cancel
              </button>
            </div>
          </div>
        ) : (
          <>
            <div className="mt-0.5 text-sm text-[#cdd6f4] whitespace-pre-wrap break-words">
              {message.content}
              {edited && (
                <span className="ml-1 text-[10px] text-[#585b70]">(edited)</span>
              )}
            </div>

            {/* Attachments */}
            {message.attachments.length > 0 && (
              <div className="mt-1 flex flex-wrap gap-2">
                {message.attachments.map((att) => (
                  <AttachmentPreview key={att.id} attachment={att} />
                ))}
              </div>
            )}
          </>
        )}

        {/* Status indicator for own messages */}
        {isOwn && status && status !== 'sent' && (
          <div className="mt-1 flex items-center gap-1">
            {status === 'sending' && (
              <>
                <Clock className="h-3 w-3 text-[#585b70]" />
                <span className="text-xs text-[#585b70]">Sending...</span>
              </>
            )}
            {status === 'failed' && (
              <button
                className="flex items-center gap-1 text-[#f38ba8] hover:underline"
                onClick={onRetry}
              >
                <AlertCircle className="h-3 w-3" />
                <span className="text-xs">Failed — tap to retry</span>
              </button>
            )}
          </div>
        )}
      </div>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Attachment preview
// ---------------------------------------------------------------------------

function AttachmentPreview({ attachment }: { attachment: Attachment }) {
  const isImage = attachment.contentType.startsWith('image/');

  if (isImage) {
    return (
      <a
        href={attachment.url}
        target="_blank"
        rel="noopener noreferrer"
        className="block max-w-[300px] overflow-hidden rounded-lg border border-[#313244] transition-colors hover:border-[#585b70]"
      >
        {attachment.thumbnailUrl || attachment.url ? (
          <img
            src={attachment.thumbnailUrl || attachment.url}
            alt={attachment.filename}
            className="h-auto w-full object-cover"
            loading="lazy"
          />
        ) : (
          <div className="flex h-24 items-center justify-center bg-[#181825]">
            <Image className="h-8 w-8 text-[#585b70]" />
          </div>
        )}
      </a>
    );
  }

  // File info card
  return (
    <a
      href={attachment.url}
      target="_blank"
      rel="noopener noreferrer"
      className="flex items-center gap-2 rounded-lg border border-[#313244] bg-[#181825] px-3 py-2 transition-colors hover:border-[#585b70]"
    >
      <FileText className="h-5 w-5 shrink-0 text-[#585b70]" />
      <div className="min-w-0">
        <p className="truncate text-sm text-[#cdd6f4]">{attachment.filename}</p>
        <p className="text-xs text-[#585b70]">{formatFileSize(attachment.size)}</p>
      </div>
    </a>
  );
}

function formatFileSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
}
