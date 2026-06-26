/**
 * Search failure — AC SEARCH-GLOBAL-5.
 *
 * Previously: a failed /search request was swallowed
 * (`.catch(() => setSearchResults(null))`) and looked identical to "no
 * results". The fix surfaces an inline error with a Retry in the search pane.
 */
import { test, expect, registerTestUser, createCommunity } from './fixtures';

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

test.describe('Search failure state', () => {
  test('SEARCH-GLOBAL-5: a failed search shows an error with retry, and retry recovers', async ({ page }) => {
    const owner = await registerTestUser('srch5');
    const name = `Findable_${Date.now()}`;
    const community = await createCommunity(owner.accessToken, name);

    await openAs(page, owner, '/@me');

    const searchInput = page.getByPlaceholder('Search... (Enter to search all)');
    await expect(searchInput).toBeVisible({ timeout: 10000 });

    // Fail /search until the test flips the gate after seeing the error UI.
    const gate = { allow: false };
    await page.route('**/api/v1/search**', async (route) => {
      if (gate.allow) await route.continue();
      else await route.fulfill({ status: 500, contentType: 'application/json', body: '{}' });
    });

    await searchInput.click();
    await searchInput.fill(name);
    await page.keyboard.press('Enter');

    // Inline error with Retry surfaces (not the silent "No results found").
    const retryBtn = page.locator('[data-slot="error-state"]').getByRole('button', { name: /retry/i });
    await expect(retryBtn).toBeVisible({ timeout: 10000 });
    await expect(page.getByText('No results found')).toHaveCount(0);

    // Retry → real result flows in.
    gate.allow = true;
    await retryBtn.click({ force: true });
    await expect(page.getByRole('button', { name: new RegExp(name) })).toBeVisible({ timeout: 10000 });
  });
});
