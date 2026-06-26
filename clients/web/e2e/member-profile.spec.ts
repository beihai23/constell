/**
 * Member profile card — AC MEM-PROFILE-1.
 *
 * Clicking a member surfaced their profile (nickname, email, online status)
 * with a way to start a DM. Implemented as a modal dialog (reuses the Dialog
 * primitive) rather than a hover popover — reliable + accessible.
 */
import { test, expect, registerTestUser, createCommunity, createChannel, joinCommunity } from './fixtures';

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

test.describe('Member profile', () => {
  test('MEM-PROFILE-1: clicking a member opens their profile with a DM entry', async ({ browser }) => {
    const owner = await registerTestUser('pfo');
    const member = await registerTestUser('pfm');
    const community = await createCommunity(owner.accessToken, `PF_${Date.now()}`);
    const channel = await createChannel(owner.accessToken, community.id, 'gen');
    await joinCommunity(member.accessToken, community.id, member.userId);

    const ctx = await browser.newContext();
    const page = await ctx.newPage();
    try {
      await openAs(page, owner, `/${community.id}/${channel.id}`);
      await expect(page.locator('[aria-label="Send message"]')).toBeVisible({ timeout: 10000 });

      // Open the member list and click the member row.
      await page.getByLabel('Toggle member list').click();
      await expect(page.getByText(member.nickname).first()).toBeVisible({ timeout: 10000 });
      await page.getByText(member.nickname).first().click();

      // Profile dialog shows nickname + email + a Send DM button.
      const dialog = page.getByRole('dialog');
      await expect(dialog).toBeVisible({ timeout: 5000 });
      await expect(dialog.getByText(member.nickname)).toBeVisible();
      await expect(dialog.getByText(member.email)).toBeVisible();

      // DM entry navigates to the DM with that member.
      await dialog.getByRole('button', { name: /send.*dm|message/i }).click();
      await expect(page).toHaveURL(new RegExp(`/@me/${member.userId}`), { timeout: 10000 });
    } finally {
      await ctx.close();
    }
  });
});
