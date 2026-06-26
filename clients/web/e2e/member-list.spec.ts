/**
 * Member list UX states — AC MEM-LIST-2 / MEM-LIST-3.
 *
 * Previously: members fetch was silent — loading showed "No members found"
 * (MEM-LIST-2) and fetch failure was swallowed (MEM-LIST-3). Driven via
 * page.route on the REST members endpoint.
 */
import { test, expect, registerTestUser, createCommunity, createChannel, joinCommunity } from './fixtures';

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

test.describe('Member list states', () => {
  test('MEM-LIST-2: a skeleton shows before members resolve (not "No members found")', async ({ browser }) => {
    const owner = await registerTestUser('mem2o');
    const member = await registerTestUser('mem2m');
    const community = await createCommunity(owner.accessToken, `MEM2_${Date.now()}`);
    const channel = await createChannel(owner.accessToken, community.id, 'gen');
    await joinCommunity(member.accessToken, community.id, member.userId);

    const ctx = await browser.newContext();
    const page = await ctx.newPage();
    try {
      // Delay the members GET so the loading state is observable.
      await page.route(`**/api/v1/communities/${community.id}/members**`, async (route) => {
        await new Promise((r) => setTimeout(r, 2000));
        await route.continue();
      });

      await openAs(page, owner, `/${community.id}/${channel.id}`);
      await expect(page.locator('[aria-label="Send message"]')).toBeVisible({ timeout: 10000 });

      // Member list is hidden by default — toggle it on so it mounts + fetches.
      await page.getByLabel('Toggle member list').click();

      // While loading, skeletons show — NOT the empty-state copy.
      await expect(page.locator('[data-slot="skeleton"]').first()).toBeVisible({ timeout: 1500 });
      await expect(page.getByText('No members found')).toHaveCount(0);

      // After resolve, the member appears.
      await expect(page.getByText(member.nickname).first()).toBeVisible({ timeout: 10000 });
    } finally {
      await ctx.close();
    }
  });

  test('MEM-LIST-3: a failed members fetch shows an error with retry, and retry recovers', async ({ browser }) => {
    const owner = await registerTestUser('mem3o');
    const member = await registerTestUser('mem3m');
    const community = await createCommunity(owner.accessToken, `MEM3_${Date.now()}`);
    const channel = await createChannel(owner.accessToken, community.id, 'gen');
    await joinCommunity(member.accessToken, community.id, member.userId);

    const ctx = await browser.newContext();
    const page = await ctx.newPage();
    try {
      // Fail until the test flips the gate after seeing the error UI.
      const gate = { allow: false };
      await page.route(`**/api/v1/communities/${community.id}/members**`, async (route) => {
        if (gate.allow) await route.continue();
        else await route.fulfill({ status: 500, contentType: 'application/json', body: '{}' });
      });

      await openAs(page, owner, `/${community.id}/${channel.id}`);
      await expect(page.locator('[aria-label="Send message"]')).toBeVisible({ timeout: 10000 });
      await page.getByLabel('Toggle member list').click();

      const retryBtn = page.locator('[data-slot="error-state"]').getByRole('button', { name: /retry/i });
      await expect(retryBtn).toBeVisible({ timeout: 10000 });

      gate.allow = true;
      await retryBtn.click({ force: true });
      await expect(page.getByText(member.nickname).first()).toBeVisible({ timeout: 10000 });
    } finally {
      await ctx.close();
    }
  });
});
