/**
 * Message edit — AC MSG-EDIT-1. Author edits their own message inline; the new
 * content shows with an "(edited)" marker, persisted across reload.
 */
import { test, expect, registerTestUser, createCommunity, createChannel } from './fixtures';

const API_BASE = process.env.E2E_API_URL || 'http://localhost:3000';

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

test.describe('Message edit', () => {
  test('MSG-EDIT-1: author edits their own message inline; shows (edited) and persists', async ({ page }) => {
    const user = await registerTestUser('ed1');
    const community = await createCommunity(user.accessToken, `EDT1_${Date.now()}`);
    const channel = await createChannel(user.accessToken, community.id, 'gen');

    const original = `orig_${Date.now()}`;
    await fetch(`${API_BASE}/api/v1/channels/${channel.id}/messages`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${user.accessToken}` },
      body: JSON.stringify({ content: original }),
    });

    await openAs(page, user, `/${community.id}/${channel.id}`);
    await expect(page.locator('[aria-label="Send message"]')).toBeVisible({ timeout: 10000 });
    await expect(page.getByText(original).first()).toBeVisible({ timeout: 10000 });
    // No edited marker before editing.
    await expect(page.getByText('(edited)')).toHaveCount(0);

    // Hover the message → Edit → inline edit → save.
    await page.getByText(original).first().hover();
    await page.getByRole('button', { name: 'Edit message' }).click();
    const editor = page.locator('textarea').first();
    await editor.fill('');
    const revised = `revised_${Date.now()}`;
    await editor.fill(revised);
    await page.getByRole('button', { name: 'Save' }).click();

    // New content + (edited) marker appear.
    await expect(page.getByText(revised).first()).toBeVisible({ timeout: 10000 });
    await expect(page.getByText('(edited)')).toBeVisible({ timeout: 5000 });
    await expect(page.getByText(original)).toHaveCount(0);

    // Persists across reload.
    await page.reload();
    await expect(page.locator('[aria-label="Send message"]')).toBeVisible({ timeout: 10000 });
    await expect(page.getByText(revised).first()).toBeVisible({ timeout: 10000 });
    await expect(page.getByText('(edited)')).toBeVisible({ timeout: 5000 });
  });
});
