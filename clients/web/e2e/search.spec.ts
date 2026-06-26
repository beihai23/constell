import { test, expect, registerTestUser, createCommunity, type TestUser } from './fixtures';

/**
 * Message + community search E2E.
 *
 * Previously this spec targeted a `[aria-label="Search"]` button and a ⌘K
 * command-palette dialog — but the palette (SearchDialog.tsx) is dead code
 * (never mounted) and the Search button only exists inside ChatHeader (channel
 * view), while the test navigated to /@me. A `.catch(() => {})` then swallowed
 * the dialog-ready wait so the test stayed green regardless. Rewritten to drive
 * the REAL flow: the inline sidebar search input + Enter, with hard assertions.
 */

const API_BASE = process.env.E2E_API_URL || 'http://localhost:3000';

/** Unique token per run so content/query never collide across runs. */
function uid(prefix: string): string {
  return `${prefix}_${Date.now().toString(36)}_${Math.random().toString(36).slice(2, 6)}`;
}

async function openAs(page: import('@playwright/test').Page, user: TestUser, path: string) {
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

test.describe('Search', () => {
  test('searching for a channel message returns it and clicking navigates to the channel', async ({ page }) => {
    const owner = await registerTestUser('searcher');

    // Seed a channel with a message whose content is unique and searchable.
    const community = await createCommunity(owner.accessToken, uid('Src'));
    const channelsRes = await fetch(`${API_BASE}/api/v1/communities/${community.id}/channels`, {
      headers: { Authorization: `Bearer ${owner.accessToken}` },
    });
    const channels = await channelsRes.json();
    const channelId = channels.channels[0].id;

    const needle = uid('findme');
    const sendRes = await fetch(`${API_BASE}/api/v1/channels/${channelId}/messages`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${owner.accessToken}` },
      body: JSON.stringify({ content: needle }),
    });
    expect(sendRes.ok).toBeTruthy();

    // The search-service indexes messages asynchronously; give it a moment.
    await page.waitForTimeout(1500);

    await openAs(page, owner, `/${community.id}/${channelId}`);

    // The real trigger: the sidebar search input + Enter. No dialog involved.
    const searchInput = page.getByPlaceholder('Search... (Enter to search all)');
    await expect(searchInput).toBeVisible({ timeout: 10000 });
    await searchInput.click();
    await searchInput.fill(needle);
    await page.keyboard.press('Enter');

    // The "Messages" section heading must appear (NOT swallowed by .catch()).
    await expect(page.getByText('Messages').first()).toBeVisible({ timeout: 10000 });

    // The matched message row: a button whose first span is the content and
    // whose label spans reference the channel (e.g. "general").
    const messageRow = page.getByRole('button', { name: new RegExp(needle) }).filter({
      hasText: 'general',
    });
    await expect(messageRow).toBeVisible({ timeout: 10000 });

    // Clicking it navigates to the channel (no per-message anchor in the app).
    await messageRow.click({ force: true });
    await expect(page).toHaveURL(new RegExp(`/${community.id}/`), { timeout: 10000 });
    await expect(page.locator('[aria-label="Send message"]')).toBeVisible({ timeout: 10000 });

    // And the actual message text is present in the opened channel history.
    await expect(page.getByText(needle).first()).toBeVisible({ timeout: 10000 });
  });
});
