import { test, expect, registerTestUser, createCommunity, createChannel, injectAuthAndNavigate } from './fixtures';

test.describe('Search', () => {
  test('search for messages shows results', async ({ page, user }) => {
    // Setup: create community + channel, send a message with a unique keyword.
    const community = await createCommunity(user.accessToken, 'Search Test Community');
    const channel = await createChannel(user.accessToken, community.id, 'search-channel');

    const uniqueKeyword = `e2e_search_${Date.now()}`;
    await fetch(`${process.env.E2E_API_URL || 'http://localhost:3000'}/api/v1/channels/${channel.id}/messages`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${user.accessToken}`,
      },
      body: JSON.stringify({ content: `find me ${uniqueKeyword}` }),
    });

    // Navigate to the app.
    await injectAuthAndNavigate(page, user);

    // Open search dialog (Ctrl+K or click search button).
    const searchButton = page.locator('[aria-label="Search"]').first();
    if (await searchButton.isVisible()) {
      await searchButton.click();
    } else {
      await page.keyboard.press('Meta+k');
    }

    // Wait for search dialog to open.
    await page.waitForSelector('input[placeholder*="Search"], [role="dialog"] input', { timeout: 5000 }).catch(() => {});

    // Type the search query.
    const searchInput = page.locator('input[placeholder*="Search"], [role="dialog"] input').first();
    await searchInput.fill(uniqueKeyword);

    // Wait for results to appear.
    await page.waitForSelector(`text=${uniqueKeyword}`, { timeout: 10000 });
    await expect(page.locator(`text=${uniqueKeyword}`).first()).toBeVisible();
  });
});
