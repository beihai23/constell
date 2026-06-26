import { useCallback } from 'react';
import { toast } from 'sonner';
import { useConstellClient } from '@/hooks/useConstellClient';
import { useMessagesStore } from '@/stores/messagesStore';
import type { Attachment, ChannelMessage, DMMessage, MessageAck } from '@constell/sdk-js';

/**
 * Shared message-send + retry logic (MSG-SEND-3).
 *
 * `send` does the optimistic insert + ACK reconciliation used by ChatInput.
 * `retry` re-sends a message whose send previously failed (status ===
 * 'failed'): it looks the optimistic message up by its temp id, re-issues the
 * send with the same content + file ids, and reconciles on ACK — the same
 * path the MessageBubble "Failed — tap to retry" affordance calls.
 *
 * Both target a channel XOR a DM, mirroring ChatInput/MessageList routing.
 */
export function useMessageSend(target: { channelId?: string; peerId?: string }) {
  const { channelId, peerId } = target;
  const client = useConstellClient();

  const appendChannelMessage = useMessagesStore((s) => s.appendChannelMessage);
  const removeChannelMessage = useMessagesStore((s) => s.removeChannelMessage);
  const channelMessages = useMessagesStore((s) => s.channelMessages);
  const appendDMMessage = useMessagesStore((s) => s.appendDMMessage);
  const removeDMMessage = useMessagesStore((s) => s.removeDMMessage);
  const dmMessages = useMessagesStore((s) => s.dmMessages);
  const setMessageStatus = useMessagesStore((s) => s.setMessageStatus);
  const removeMessageStatus = useMessagesStore((s) => s.removeMessageStatus);

  /** Reconcile an optimistic temp-id'd message with the real server message. */
  const reconcile = useCallback(
    (tempId: string, ack: MessageAck, optimisticChannel: ChannelMessage | null, optimisticDM: DMMessage | null) => {
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
    },
    [channelId, peerId, appendChannelMessage, removeChannelMessage, appendDMMessage, removeDMMessage, setMessageStatus, removeMessageStatus],
  );

  /**
   * Retry a failed send. `tempId` is the optimistic message's id (status
   * 'failed'). Re-sends content + fileIds and reconciles on ACK.
   */
  const retry = useCallback(
    async (tempId: string) => {
      if (!channelId && !peerId) return;
      // Locate the failed optimistic message to recover its content + file ids.
      const existing = channelId
        ? (channelMessages.get(channelId) ?? []).find((m) => m.id === tempId)
        : peerId
          ? (dmMessages.get(peerId) ?? []).find((m) => m.id === tempId)
          : undefined;
      if (!existing) return;
      const fileIds = (existing.attachments ?? [])
        .map((a: Attachment) => a.fileId)
        .filter((id): id is string => !!id);

      setMessageStatus(tempId, 'sending');
      try {
        let ack: MessageAck | undefined;
        if (channelId) {
          ack = await client.sendChannelMessage(channelId, existing.content, fileIds.length > 0 ? fileIds : undefined);
        } else if (peerId) {
          ack = await client.sendDM(peerId, existing.content, fileIds.length > 0 ? fileIds : undefined);
        }
        if (ack) {
          reconcile(tempId, ack, channelId ? (existing as ChannelMessage) : null, peerId ? (existing as DMMessage) : null);
        } else {
          setMessageStatus(tempId, 'sent');
        }
      } catch (err) {
        setMessageStatus(tempId, 'failed');
        toast.error(err instanceof Error ? err.message : 'Failed to send message');
      }
    },
    [channelId, peerId, channelMessages, dmMessages, client, setMessageStatus, reconcile],
  );

  return { retry };
}
