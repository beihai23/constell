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
import { Textarea } from '@/components/ui/textarea';
import { toast } from 'sonner';

// ---------------------------------------------------------------------------
// Props
// ---------------------------------------------------------------------------

interface CreateCommunityDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

/**
 * Controlled dialog for creating a new community. Opened from the "+" rail
 * button. On submit it calls client.createCommunity, inserts the result into
 * the communities store, best-effort fetches the (usually empty) channel list
 * so ChannelList has an entry, then navigates into the new community.
 */
export function CreateCommunityDialog({
  open,
  onOpenChange,
}: CreateCommunityDialogProps) {
  const navigate = useNavigate();
  const client = useConstellClient();
  const addCommunity = useCommunitiesStore((s) => s.addCommunity);
  const setChannels = useCommunitiesStore((s) => s.setChannels);

  const [name, setName] = useState('');
  const [description, setDescription] = useState('');
  const [submitting, setSubmitting] = useState(false);

  // Close + reset form state. Funnels every close path (X button, Escape,
  // overlay click, Cancel button, successful submit) through one place so the
  // dialog always empties before it can be reopened.
  const handleClose = useCallback(() => {
    setName('');
    setDescription('');
    setSubmitting(false);
    onOpenChange(false);
  }, [onOpenChange]);

  const handleSubmit = useCallback(
    async (e: React.FormEvent) => {
      e.preventDefault();
      const trimmedName = name.trim();
      if (!trimmedName || submitting) return;

      setSubmitting(true);
      try {
        const community = await client.createCommunity(
          trimmedName,
          description.trim() || undefined,
        );
        addCommunity(community);

        // Populate the channel list for the new community. The backend does
        // not seed a default channel, so this is usually empty — fetch it
        // anyway so ChannelList has a real entry and future default-channel
        // seeding flows through transparently.
        try {
          const channels = await client.getChannels(community.id);
          setChannels(community.id, channels);
        } catch {
          // Non-fatal — ChannelList tolerates a missing entry.
        }

        toast.success(`Created "${community.name}"`);
        handleClose();
        navigate(`/${community.id}`);
      } catch {
        toast.error('Failed to create community');
      } finally {
        setSubmitting(false);
      }
    },
    [name, description, submitting, client, addCommunity, setChannels, handleClose, navigate],
  );

  const canSubmit = name.trim().length > 0 && !submitting;

  return (
    <Dialog open={open} onOpenChange={(next) => { if (!next) handleClose(); }}>
      <DialogContent className="gap-4 rounded-xl border border-[#313244] bg-[#1e1e2e] p-5 text-[#cdd6f4] shadow-2xl sm:max-w-md">
        <DialogHeader>
          <DialogTitle className="text-base font-semibold text-[#cdd6f4]">
            Create a Community
          </DialogTitle>
          <DialogDescription className="text-[#585b70]">
            Give your new community a name. You'll become the owner.
          </DialogDescription>
        </DialogHeader>

        <form onSubmit={handleSubmit} className="flex flex-col gap-4">
          <div className="flex flex-col gap-1.5">
            <label
              htmlFor="create-community-name"
              className="text-xs font-semibold tracking-wide text-[#585b70] uppercase"
            >
              Community Name
            </label>
            <Input
              id="create-community-name"
              // Base UI Dialog moves focus to the first focusable element on
              // open; autoFocus is belt-and-suspenders.
              autoFocus
              value={name}
              maxLength={100}
              onChange={(e) => setName(e.target.value)}
              placeholder="e.g. Gophers United"
              className="border-[#313244] bg-[#11111b] text-[#cdd6f4] placeholder:text-[#585b70] focus-visible:ring-[#585b70]"
            />
          </div>

          <div className="flex flex-col gap-1.5">
            <label
              htmlFor="create-community-desc"
              className="text-xs font-semibold tracking-wide text-[#585b70] uppercase"
            >
              Description{' '}
              <span className="font-normal normal-case text-[#45475a]">
                (optional)
              </span>
            </label>
            <Textarea
              id="create-community-desc"
              value={description}
              maxLength={500}
              rows={3}
              onChange={(e) => setDescription(e.target.value)}
              placeholder="What's this community about?"
              className="border-[#313244] bg-[#11111b] text-[#cdd6f4] placeholder:text-[#585b70] focus-visible:ring-[#585b70]"
            />
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
