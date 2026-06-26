/**
 * Member kick UI — AC MEM-KICK-2 (non-owner can't kick) + the UI half of
 * MEM-KICK-1 (owner can kick from the profile card).
 *
 * The backend KickMember RPC was already wired + tested via REST; this adds
 * the in-app affordance: the community owner sees a Kick button on a member's
 * profile, a non-owner does NOT.
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

test.describe('Member kick UI', () => {
  test('MEM-KICK-2: a non-owner does not see a Kick button on a member profile', async ({ browser }) => {
    const owner = await registerTestUser('kuO');
    const nonOwner = await registerTestUser('kuN');
    const other = await registerTestUser('kuM');
    const community = await createCommunity(owner.accessToken, `KU_${Date.now()}`);
    const channel = await createChannel(owner.accessToken, community.id, 'gen');
    await joinCommunity(nonOwner.accessToken, community.id, nonOwner.userId);
    await joinCommunity(other.accessToken, community.id, other.userId);

    const ctx = await browser.newContext();
    const page = await ctx.newPage();
    try {
      // Non-owner views the community and opens another member's profile.
      await openAs(page, nonOwner, `/${community.id}/${channel.id}`);
      await expect(page.locator('[aria-label="Send message"]')).toBeVisible({ timeout: 10000 });
      await page.getByLabel('Toggle member list').click();
      await page.getByText(other.nickname).first().click();
      const dialog = page.getByRole('dialog');
      await expect(dialog).toBeVisible({ timeout: 5000 });

      // Non-owner must NOT see any Kick affordance.
      await expect(dialog.getByRole('button', { name: /^kick$/i })).toHaveCount(0);
      await expect(dialog.getByRole('button', { name: /confirm kick/i })).toHaveCount(0);
    } finally {
      await ctx.close();
    }
  });

  test('MEM-KICK-1 (UI): owner can kick a member from the profile card', async ({ browser }) => {
    const owner = await registerTestUser('kO2');
    const victim = await registerTestUser('kV2');
    const community = await createCommunity(owner.accessToken, `KU2_${Date.now()}`);
    const channel = await createChannel(owner.accessToken, community.id, 'gen');
    await joinCommunity(victim.accessToken, community.id, victim.userId);

    const ctx = await browser.newContext();
    const page = await ctx.newPage();
    try {
      await openAs(page, owner, `/${community.id}/${channel.id}`);
      await expect(page.locator('[aria-label="Send message"]')).toBeVisible({ timeout: 10000 });
      await page.getByLabel('Toggle member list').click();
      await expect(page.getByText(victim.nickname).first()).toBeVisible({ timeout: 10000 });

      // Open the victim's profile and kick (two-step confirm).
      await page.getByText(victim.nickname).first().click();
      const dialog = page.getByRole('dialog');
      await expect(dialog).toBeVisible({ timeout: 5000 });
      await dialog.getByRole('button', { name: /^kick$/i }).click();
      await dialog.getByRole('button', { name: /confirm kick/i }).click();

      // Dialog closes and the victim is gone from the member list.
      await expect(dialog).toHaveCount(0, { timeout: 5000 });
      await expect(page.getByText(victim.nickname)).toHaveCount(0, { timeout: 5000 });
    } finally {
      await ctx.close();
    }
  });
});
