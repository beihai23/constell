import { useState, useCallback } from 'react';
import { useNavigate } from 'react-router';
import { useConstellClient } from '@/hooks/useConstellClient';
import { useCommunitiesStore } from '@/stores/communitiesStore';
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
} from '@/components/ui/dialog';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { toast } from 'sonner';

// ---------------------------------------------------------------------------
// Props
// ---------------------------------------------------------------------------

interface CreateChannelDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  communityId: string;
}

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

/**
 * Controlled dialog for creating a text channel in a community. Opened from
 * the "+" next to TEXT CHANNELS. On submit it calls client.createChannel,
 * inserts the result into the store, and navigates into the new channel.
 */
export function CreateChannelDialog({
  open,
  onOpenChange,
  communityId,
}: CreateChannelDialogProps) {
  const navigate = useNavigate();
  const client = useConstellClient();
  const addChannel = useCommunitiesStore((s) => s.addChannel);

  const [name, setName] = useState('');
  const [submitting, setSubmitting] = useState(false);

  // Close + reset form. Every close path funnels through here so the dialog
  // always empties before it can be reopened.
  const handleClose = useCallback(() => {
    setName('');
    setSubmitting(false);
    onOpenChange(false);
  }, [onOpenChange]);

  const handleSubmit = useCallback(
    async (e: React.FormEvent) => {
      e.preventDefault();
      const trimmed = name.trim();
      if (!trimmed || submitting) return;

      setSubmitting(true);
      try {
        const channel = await client.createChannel(communityId, trimmed);
        addChannel(communityId, channel);
        toast.success(`Created #${channel.name}`);
        handleClose();
        navigate(`/${communityId}/${channel.id}`);
      } catch {
        toast.error('Failed to create channel');
      } finally {
        setSubmitting(false);
      }
    },
    [name, submitting, communityId, client, addChannel, handleClose, navigate],
  );

  const canSubmit = name.trim().length > 0 && !submitting;

  return (
    <Dialog open={open} onOpenChange={(next) => { if (!next) handleClose(); }}>
      <DialogContent className="gap-4 rounded-xl border border-[#313244] bg-[#1e1e2e] p-5 text-[#cdd6f4] shadow-2xl sm:max-w-md">
        <DialogHeader>
          <DialogTitle className="text-base font-semibold text-[#cdd6f4]">
            Create Channel
          </DialogTitle>
          <DialogDescription className="text-[#585b70]">
            Create a new text channel in this community.
          </DialogDescription>
        </DialogHeader>

        <form onSubmit={handleSubmit} className="flex flex-col gap-4">
          <div className="flex flex-col gap-1.5">
            <label
              htmlFor="create-channel-name"
              className="text-xs font-semibold tracking-wide text-[#585b70] uppercase"
            >
              Channel Name
            </label>
            <div className="flex items-center gap-2 rounded-lg border border-[#313244] bg-[#11111b] px-2.5 focus-within:ring-3 focus-within:ring-[#585b70]">
              <span className="text-[#585b70]">#</span>
              <Input
                id="create-channel-name"
                autoFocus
                value={name}
                maxLength={50}
                onChange={(e) => setName(e.target.value)}
                placeholder="new-channel"
                className="border-0 bg-transparent px-0 text-[#cdd6f4] placeholder:text-[#585b70] focus-visible:border-0 focus-visible:ring-0"
              />
            </div>
          </div>

          <div className="flex justify-end gap-2 pt-1">
            <Button
              type="button"
              variant="ghost"
              onClick={handleClose}
              disabled={submitting}
              className="text-[#a6adc8] hover:bg-[#313244] hover:text-[#cdd6f4]"
            >
              Cancel
            </Button>
            <Button type="submit" disabled={!canSubmit}>
              {submitting ? 'Creating…' : 'Create'}
            </Button>
          </div>
        </form>
      </DialogContent>
    </Dialog>
  );
}
