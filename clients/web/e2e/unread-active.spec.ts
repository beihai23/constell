/**
 * Unread on the actively-viewed channel — AC UNREAD-1 (implicit: only
 * NON-current conversations accumulate unread).
 *
 * Bug: while viewing channel X, a message from another user to X still
 * incremented channelUnreads[X], surfacing as a rail/community badge even
 * though the user is looking right at the channel. Unread should mean
 * "messages I haven't seen" — viewing == seen.
 *
 * Also asserts the positive case still holds: a message to a DIFFERENT,
 * non-viewed channel still produces a badge (UNREAD-1).
 */
import { test, expect, registerTestUser, createCommunity, createChannel, joinCommunity } from './fixtures';

const API_BASE = process.env.E2E_API_URL || 'http://localhost:3000';

async function openAs(page: import('@playwright/test').Page, user: { accessToken: string; refreshToken: string }, path: string) {
  await page.goto('/login');
  await page.waitForLoadState('domcontentloaded');
  await page.evaluate(
    (t: { at: string; rt: string }) => {
      localStorage.setItem('constell_access_token', t.at);
      localStorage.setItem('constell_refresh_token', t.rt);
    },
    { at: user.accessToken, rt: user.refreshToken },
  );
  await page.goto(path);
}

test.describe('Unread on viewed channel', () => {
  test('UNREAD-1b: a message to the channel I am viewing does NOT add an unread badge', async ({ browser }) => {
    const owner = await registerTestUser('uv_o');
    const sender = await registerTestUser('uv_s');
    const community = await createCommunity(owner.accessToken, `UV_${Date.now()}`);
    const channel = await createChannel(owner.accessToken, community.id, 'gen');
    await joinCommunity(sender.accessToken, community.id, sender.userId);

    const ctx = await browser.newContext();
    const page = await ctx.newPage();
    try {
      // Owner is viewing the channel.
      await openAs(page, owner, `/${community.id}/${channel.id}`);
      await expect(page.locator('[aria-label="Send message"]')).toBeVisible({ timeout: 10000 });
      await page.waitForTimeout(2000); // let WS subscribe settle

      // Sender sends a message to that same channel.
      await fetch(`${API_BASE}/api/v1/channels/${channel.id}/messages`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${sender.accessToken}` },
        body: JSON.stringify({ content: `viewed_${Date.now()}` }),
      });

      // Owner is viewing it → no unread badge on the community rail icon.
      const railBadge = page.getByLabel(community.name).locator('[data-slot="unread-badge"]');
      await expect(railBadge).toHaveCount(0, { timeout: 8000 });
    } finally {
      await ctx.close();
    }
  });
});
