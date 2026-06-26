/**
 * Message delete — AC MSG-DEL-1 (own message deletable) + MSG-DEL-2 (others'
 * messages not). End-to-end through the new REST delete path; the message is
 * removed from the DB so it stays gone after reload.
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

test.describe('Message delete', () => {
  test('MSG-DEL-1: author deletes their own message and it stays gone after reload', async ({ page }) => {
    const user = await registerTestUser('del1');
    const community = await createCommunity(user.accessToken, `DEL1_${Date.now()}`);
    const channel = await createChannel(user.accessToken, community.id, 'gen');

    // Seed a message via REST so it has a real id (not an optimistic temp id).
    const content = `deleteme_${Date.now()}`;
    await fetch(`${API_BASE}/api/v1/channels/${channel.id}/messages`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${user.accessToken}` },
      body: JSON.stringify({ content }),
    });

    await openAs(page, user, `/${community.id}/${channel.id}`);
    await expect(page.locator('[aria-label="Send message"]')).toBeVisible({ timeout: 10000 });
    await expect(page.getByText(content).first()).toBeVisible({ timeout: 10000 });

    // Hover the message row to reveal the delete button, then delete via confirm.
    await page.getByText(content).first().hover();
    await page.getByRole('button', { name: 'Delete message' }).click();
    await expect(page.getByRole('heading', { name: /delete message/i })).toBeVisible({ timeout: 5000 });
    await page.getByRole('button', { name: /^delete$/i }).click();

    // Gone from the list, and stays gone after a reload (DB delete).
    await expect(page.getByText(content)).toHaveCount(0, { timeout: 10000 });
    await page.reload();
    await expect(page.locator('[aria-label="Send message"]')).toBeVisible({ timeout: 10000 });
    await expect(page.getByText(content)).toHaveCount(0, { timeout: 10000 });
  });

  test('MSG-DEL-2: a message from someone else has no delete affordance', async ({ browser }) => {
    const owner = await registerTestUser('del2o');
    const other = await registerTestUser('del2m');
    const community = await createCommunity(owner.accessToken, `DEL2_${Date.now()}`);
    const channel = await createChannel(owner.accessToken, community.id, 'gen');
    await joinCommunity(other.accessToken, community.id, other.userId);

    const otherContent = `theirs_${Date.now()}`;
    await fetch(`${API_BASE}/api/v1/channels/${channel.id}/messages`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${other.accessToken}` },
      body: JSON.stringify({ content: otherContent }),
    });

    const ctx = await browser.newContext();
    const page = await ctx.newPage();
    try {
      await openAs(page, owner, `/${community.id}/${channel.id}`);
      await expect(page.getByText(otherContent).first()).toBeVisible({ timeout: 10000 });
      await page.getByText(otherContent).first().hover();
      // Owner can't delete someone else's message — no delete button.
      await expect(page.getByRole('button', { name: 'Delete message' })).toHaveCount(0);
    } finally {
      await ctx.close();
    }
  });
});
