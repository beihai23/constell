/**
 * Channel selection highlight — AC CHAN-LIST-2 (the selected-state half).
 *
 * Bug: ChannelList derived `selected` from `communitiesStore.currentChannelId`,
 * a field that NOTHING ever set (selectChannel was never called). So the
 * channel the user was viewing was never highlighted as selected. Selection
 * must be derived from the route.
 */
import { test, expect, registerTestUser, createCommunity, createChannel } from './fixtures';

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

test.describe('Channel selection highlight', () => {
  test('CHAN-LIST-2: the channel currently being viewed is marked selected', async ({ page }) => {
    const user = await registerTestUser('sel');
    const community = await createCommunity(user.accessToken, `SEL_${Date.now()}`);
    const chA = await createChannel(user.accessToken, community.id, 'chan-a');
    const chB = await createChannel(user.accessToken, community.id, 'chan-b');

    await openAs(page, user, `/${community.id}/${chB.id}`);
    await expect(page.locator('[aria-label="Send message"]')).toBeVisible({ timeout: 10000 });
    await page.waitForTimeout(1000);

    // The viewed channel (B) is marked current; the other (A) is not.
    const rowB = page.getByRole('button', { name: /chan-b/ });
    const rowA = page.getByRole('button', { name: /chan-a/ });
    await expect(rowB).toHaveAttribute('aria-current', 'page');
    await expect(rowA).toHaveAttribute('aria-current', 'false');
  });
});
