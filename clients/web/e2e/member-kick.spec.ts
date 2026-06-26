/**
 * Owner kicks a member — AC MEM-KICK-1.
 *
 * History: this was "cheat #4" in the 2026-06-09 e2e report — the api-gateway
 * RemoveMember handler used to call LeaveCommunity (ignoring the target uid),
 * so an owner "kicking" a member actually made the owner leave. The backend
 * now calls KickMember with the path uid; this test locks that contract down
 * end-to-end: after the owner kicks, the victim is removed and loses access.
 *
 * NOTE: the frontend has no kick UI yet (a 【目标-待建】 in features/60 — needs
 * a context-menu + alert-dialog primitive). The kick is driven via REST here,
 * matching the existing community.spec boundary-test pattern; when the kick
 * UI ships, an in-app UI test should replace this.
 */
import { test, expect, registerTestUser, createCommunity } from './fixtures';

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

test.describe('Owner kicks a member', () => {
  test('MEM-KICK-1: owner removes a member; victim loses the community', async ({ browser }) => {
    const owner = await registerTestUser('kicko');
    const victim = await registerTestUser('kickv');

    // Owner creates a public community; victim joins.
    const community = await createCommunity(owner.accessToken, `KICK_${Date.now()}`);
    await fetch(`${API_BASE}/api/v1/communities/${community.id}/members`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${victim.accessToken}` },
      body: JSON.stringify({ user_id: victim.userId }),
    });

    // Victim opens the community, sees it on their rail.
    const ctx = await browser.newContext();
    const page = await ctx.newPage();
    try {
      await openAs(page, victim, `/@me`);
      await page.getByRole('button', { name: community.name }).click();

      // Owner kicks the victim via REST.
      const kickRes = await fetch(`${API_BASE}/api/v1/communities/${community.id}/members/${victim.userId}`, {
        method: 'DELETE',
        headers: { Authorization: `Bearer ${owner.accessToken}` },
      });
      expect(kickRes.ok).toBeTruthy();

      // After reload, the victim no longer has the community on the rail.
      await page.goto('/@me');
      await expect(page.getByRole('button', { name: community.name })).toHaveCount(0, { timeout: 10000 });
    } finally {
      await ctx.close();
    }
  });
});
