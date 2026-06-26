/**
 * New-message "jump to bottom" pill — AC MSG-HIST-6.
 *
 * When the user has scrolled up away from the bottom and a new message
 * arrives, the list must NOT auto-scroll (would yank their reading position)
 * and must surface a "↓ N new" pill that returns them to the bottom on click.
 *
 * Driven from the recipient's view: A is scrolled up in a channel; B sends via
 * REST; A receives it via the real WS push; the pill appears; clicking it
 * scrolls A to the new message.
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

test.describe('New message pill', () => {
  test('MSG-HIST-6: scrolled-up user keeps position + gets a "new messages" pill', async ({ browser }) => {
    const owner = await registerTestUser('np_o');
    const sender = await registerTestUser('np_s');
    const community = await createCommunity(owner.accessToken, `NP_${Date.now()}`);
    const channel = await createChannel(owner.accessToken, community.id, 'gen');
    await joinCommunity(sender.accessToken, community.id, sender.userId);

    // Seed enough history so the list scrolls.
    for (let i = 0; i < 15; i++) {
      await fetch(`${API_BASE}/api/v1/channels/${channel.id}/messages`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${owner.accessToken}` },
        body: JSON.stringify({ content: `seed_${i}_${Date.now()}` }),
      });
    }

    const ctx = await browser.newContext({ viewport: { width: 1024, height: 500 } });
    const page = await ctx.newPage();
    try {
      await openAs(page, owner, `/${community.id}/${channel.id}`);
      await expect(page.locator('[aria-label="Send message"]')).toBeVisible({ timeout: 10000 });
      // Let history load + settle at the bottom.
      await page.waitForTimeout(1500);

      // Scroll the message list up, away from the bottom.
      const scroller = page.locator('.flex-1.overflow-y-auto').first();
      await scroller.evaluate((el: HTMLElement) => {
        el.scrollTop = 0;
        el.dispatchEvent(new Event('scroll', { bubbles: true }));
      });
      await page.waitForTimeout(300);
      const scrollTopBefore = await scroller.evaluate((el: HTMLElement) => el.scrollTop);
      expect(scrollTopBefore).toBeLessThan(50);

      // Sender sends a new message via REST → owner receives it over WS.
      const fresh = `fresh_${Date.now()}`;
      await fetch(`${API_BASE}/api/v1/channels/${channel.id}/messages`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${sender.accessToken}` },
        body: JSON.stringify({ content: fresh }),
      });

      // The "new messages" pill appears.
      const pill = page.locator('[data-slot="new-messages-pill"]');
      await expect(pill).toBeVisible({ timeout: 10000 });

      // Position was NOT yanked to the bottom (still near top).
      const scrollTopMid = await scroller.evaluate((el: HTMLElement) => el.scrollTop);
      expect(scrollTopMid).toBeLessThan(100);

      // Clicking the pill scrolls to the bottom, where the fresh message is visible.
      await pill.click();
      await expect(page.getByText(fresh).first()).toBeVisible({ timeout: 10000 });
    } finally {
      await ctx.close();
    }
  });
});
