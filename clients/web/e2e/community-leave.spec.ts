/**
 * Leave a community — AC COMM-LEAVE-1.
 *
 * History: the app let you join a community but had no way to leave (the
 * proto/backend LeaveCommunity RPC existed but was never exposed through
 * api-gateway or SDK, and no UI). This test covers the end-to-end: a non-owner
 * member leaves, and the community disappears from their rail.
 */
import { test, expect, registerTestUser, createCommunity, joinCommunity } from './fixtures';

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

test.describe('Leave a community', () => {
  test('COMM-LEAVE-1: a member leaves and the community is gone from the rail', async ({ page }) => {
    const owner = await registerTestUser('leaveO');
    const leaver = await registerTestUser('leaveM');
    const community = await createCommunity(owner.accessToken, `LEAVE_${Date.now()}`);
    await joinCommunity(leaver.accessToken, community.id, leaver.userId);

    await openAs(page, leaver, `/${community.id}`);
    // Community is on the rail.
    await expect(page.getByRole('button', { name: community.name })).toBeVisible({ timeout: 10000 });

    // Open the leave affordance and confirm.
    await page.getByLabel('Leave community').click();
    await expect(page.getByRole('heading', { name: /leave/i })).toBeVisible({ timeout: 5000 });
    await page.getByRole('button', { name: /^leave$/i }).click();

    // Navigated to DM home and the community is gone from the rail.
    await expect(page).toHaveURL(/\/@me/, { timeout: 10000 });
    await expect(page.getByRole('button', { name: community.name })).toHaveCount(0, { timeout: 10000 });
  });
});
