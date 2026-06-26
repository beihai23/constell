/**
 * Message history UX states — AC MSG-HIST-1 / MSG-HIST-4.
 *
 * These previously had NO tests: loading silently showed "No messages yet"
 * (MSG-HIST-1, loading/empty confusion) and history fetch failure was
 * swallowed (MSG-HIST-4, silent catch). Both are driven via page.route on the
 * REST history endpoint so the states are deterministic.
 */
import { test, expect, registerTestUser, createCommunity, createChannel } from './fixtures';

const API_BASE = process.env.E2E_API_URL || 'http://localhost:3000';

async function openChannel(page: import('@playwright/test').Page, user: ReturnType<typeof registerTestUser> extends Promise<infer U> ? U : never, path: string) {
  await page.goto('/login');
  await page.evaluate(
    (tokens: { at: string; rt: string }) => {
      localStorage.setItem('constell_access_token', tokens.at);
      localStorage.setItem('constell_refresh_token', tokens.rt);
    },
    { at: user.accessToken, rt: user.refreshToken },
  );
  await page.goto(path);
}

test.describe('Message history states', () => {
  test('MSG-HIST-1: a loading skeleton shows before history resolves (not "No messages yet")', async ({ page }) => {
    const user = await registerTestUser('hist1');
    const community = await createCommunity(user.accessToken, `HIST1_${Date.now()}`);
    const channel = await createChannel(user.accessToken, community.id, 'gen');

    // Seed a real message so the delayed response carries content.
    const needle = `seed_${Date.now()}`;
    await fetch(`${API_BASE}/api/v1/channels/${channel.id}/messages`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${user.accessToken}` },
      body: JSON.stringify({ content: needle }),
    });

    // Delay the history GET by 2s, then forward to the real backend.
    await page.route(`**/api/v1/channels/${channel.id}/messages**`, async (route) => {
      await new Promise((r) => setTimeout(r, 2000));
      await route.continue();
    });

    await openChannel(page, user, `/${community.id}/${channel.id}`);

    // While loading, skeletons are visible — NOT the empty-state copy.
    await expect(page.locator('[data-slot="skeleton"]').first()).toBeVisible({ timeout: 1500 });
    expect(await page.locator('[data-slot="skeleton"]').count()).toBeGreaterThan(0);
    await expect(page.getByText('No messages yet')).toHaveCount(0);

    // After the delayed response resolves, the seeded message appears.
    await expect(page.getByText(needle).first()).toBeVisible({ timeout: 10000 });
  });

  test('MSG-HIST-4: a failed history fetch shows an error with retry, and retry recovers', async ({ page }) => {
    const user = await registerTestUser('hist4');
    const community = await createCommunity(user.accessToken, `HIST4_${Date.now()}`);
    const channel = await createChannel(user.accessToken, community.id, 'gen');

    const needle = `seed_${Date.now()}`;
    await fetch(`${API_BASE}/api/v1/channels/${channel.id}/messages`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${user.accessToken}` },
      body: JSON.stringify({ content: needle }),
    });

    // Fail every history fetch until the test flips `allow` (after seeing the
    // error UI). Flag-based rather than count-based so it's robust to the
    // dev-server StrictMode double-mount and any parallel backfill requests.
    const gate = { allow: false };
    await page.route(`**/api/v1/channels/${channel.id}/messages**`, async (route) => {
      if (gate.allow) {
        await route.continue();
      } else {
        await route.fulfill({ status: 500, contentType: 'application/json', body: JSON.stringify({ message: 'boom' }) });
      }
    });

    await openChannel(page, user, `/${community.id}/${channel.id}`);

    // Error state with a Retry button is shown.
    const retryBtn = page.locator('[data-slot="error-state"]').getByRole('button', { name: /retry/i });
    await expect(retryBtn).toBeVisible({ timeout: 10000 });

    // Allow success, then retry → real data flows in.
    gate.allow = true;
    await retryBtn.click({ force: true });
    await expect(page.getByText(needle).first()).toBeVisible({ timeout: 10000 });
  });
});
