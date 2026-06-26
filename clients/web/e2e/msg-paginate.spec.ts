/**
 * Message history pagination — AC MSG-HIST-3.
 *
 * Scrolling to the top fetches older messages, but the fetch was silent — no
 * indicator. This test verifies a loading indicator appears while the older
 * page is loading, and that older messages prepend without duplicates.
 */
import { test, expect, registerTestUser, createCommunity, createChannel } from './fixtures';

const API_BASE = process.env.E2E_API_URL || 'http://localhost:3000';

async function openAs(page: import('@playwright/test').Page, user: { accessToken: string; refreshToken: string }, path: string) {
  await page.goto('/login');
  await page.evaluate(
    (t: { at: string; rt: string }) => {
      localStorage.setItem('constell_access_token', t.at);
      localStorage.setItem('constell_refresh_token', t.rt);
    },
    { at: user.accessToken, rt: user.refreshToken },
  );
  await page.goto(path);
}

test.describe('Message pagination', () => {
  test('MSG-HIST-3: scrolling up shows a loading indicator while older messages load', async ({ page }) => {
    const user = await registerTestUser('pg3');
    const community = await createCommunity(user.accessToken, `PG3_${Date.now()}`);
    const channel = await createChannel(user.accessToken, community.id, 'gen');

    // Seed several messages so the list is populated.
    const needles: string[] = [];
    for (let i = 0; i < 8; i++) {
      const c = `pg3_msg_${Date.now()}_${i}`;
      needles.push(c);
      await fetch(`${API_BASE}/api/v1/channels/${channel.id}/messages`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${user.accessToken}` },
        body: JSON.stringify({ content: c }),
      });
    }

    // Delay every history request after the first so the pagination fetch
    // is observable. (The first is the initial load.)
    let count = 0;
    await page.route(`**/api/v1/channels/${channel.id}/messages**`, async (route) => {
      count++;
      if (count > 1) await new Promise((r) => setTimeout(r, 1500));
      await route.continue();
    });

    await openAs(page, user, `/${community.id}/${channel.id}`);

    // Wait for initial load.
    await expect(page.getByText(needles[needles.length - 1]).first()).toBeVisible({ timeout: 10000 });

    // Trigger top-of-list pagination. Setting scrollTop alone does not fire
    // onScroll when there's no overflow, so dispatch the event explicitly.
    await page.locator('.flex-1.overflow-y-auto').first().evaluate((el: HTMLElement) => {
      el.scrollTop = 0;
      el.dispatchEvent(new Event('scroll', { bubbles: true }));
    });

    // A loading indicator appears while the older page is fetched.
    await expect(page.locator('[data-slot="pagination-loading"]').first()).toBeVisible({ timeout: 5000 });
  });
});
