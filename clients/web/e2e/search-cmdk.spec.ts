/**
 * ⌘K global search palette — AC SEARCH-GLOBAL-1 / SEARCH-GLOBAL-3 / SEARCH-GLOBAL-7.
 *
 * The app shipped a 481-line SearchDialog (command palette) that was never
 * mounted — dead code — while the sidebar inline search did double duty. This
 * test locks the palette as the ⌘K surface: it opens on ⌘K, shows an empty-
 * query guide, and closes on Esc.
 */
import { test, expect, registerTestUser, createCommunity } from './fixtures';

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

// Serial: drives a global ⌘K keydown handler + dialog focus; parallel workers
// race the keyboard event timing. Stable when serialized.
test.describe.configure({ mode: 'serial' });

test.describe('⌘K global search palette', () => {
  test('SEARCH-GLOBAL-1: ⌘K opens the palette; SEARCH-GLOBAL-3: empty query shows guidance', async ({ page }) => {
    const user = await registerTestUser('cmdk1');
    const community = await createCommunity(user.accessToken, `CMDK_${Date.now()}`);
    await openAs(page, user, `/${community.id}`);
    await expect(page.locator('[aria-label="Send message"]')).toBeVisible({ timeout: 10000 });

    // No palette yet.
    await expect(page.getByPlaceholder('Search users, messages...')).toHaveCount(0);

    // ⌘K opens the palette.
    await page.keyboard.press('Control+KeyK');
    const paletteInput = page.getByPlaceholder('Search users, messages...');
    await expect(paletteInput).toBeVisible({ timeout: 5000 });

    // Empty-query guidance (SEARCH-GLOBAL-3).
    await expect(page.getByText('Start typing to search...')).toBeVisible();
  });

  test('SEARCH-GLOBAL-7: Esc closes the palette', async ({ page }) => {
    const user = await registerTestUser('cmdk2');
    const community = await createCommunity(user.accessToken, `CMDK2_${Date.now()}`);
    await openAs(page, user, `/${community.id}`);
    await expect(page.locator('[aria-label="Send message"]')).toBeVisible({ timeout: 10000 });

    await page.keyboard.press('Control+KeyK');
    await expect(page.getByPlaceholder('Search users, messages...')).toBeVisible({ timeout: 5000 });

    await page.keyboard.press('Escape');
    await expect(page.getByPlaceholder('Search users, messages...')).toHaveCount(0);
  });
});
