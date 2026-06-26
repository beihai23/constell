/**
 * File upload failure — AC FILE-UPLOAD-2.
 *
 * Previously: an upload failure was bundled into the whole send and surfaced
 * only as a generic "Failed to send message" toast, and the pending preview
 * was discarded. The fix surfaces an upload-specific error on the preview and
 * keeps it so the user can retry or remove.
 */
import { test, expect, registerTestUser, createCommunity, createChannel } from './fixtures';
import { writeFileSync, mkdirSync, rmSync } from 'fs';
import { join, dirname } from 'path';
import { fileURLToPath } from 'url';
import type { Page } from '@playwright/test';

const __dirname = dirname(fileURLToPath(import.meta.url));
const tmpDir = join(__dirname, '.tmp-e2e-filefail');

async function openAs(page: Page, user: { accessToken: string; refreshToken: string }, path: string) {
  // Dev server HMR can briefly destroy the execution context after a goto;
  // wait for the page to settle before touching localStorage.
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

test.describe('File upload failure', () => {
  test.beforeAll(() => mkdirSync(tmpDir, { recursive: true }));
  test.afterAll(() => rmSync(tmpDir, { recursive: true, force: true }));

  test('FILE-UPLOAD-2: an upload failure surfaces a clear error and keeps the preview', async ({ page, user }) => {
    const community = await createCommunity(user.accessToken, `FILE2_${Date.now()}`);
    const channel = await createChannel(user.accessToken, community.id, 'gen');

    const filePath = join(tmpDir, `fail_${Date.now()}.txt`);
    writeFileSync(filePath, 'this upload will fail');

    // Inject auth + open the channel.
    await openAs(page, user, `/${community.id}/${channel.id}`);
    await expect(page.locator('[aria-label="Send message"]')).toBeVisible({ timeout: 10000 });

    // Fail the upload endpoint with 500.
    await page.route('**/api/v1/files/upload**', (route) =>
      route.fulfill({ status: 500, contentType: 'application/json', body: '{"message":"storage down"}' }),
    );

    // Pick the file.
    const attach = page.locator('[aria-label="Attach file"]').first();
    const [fileChooser] = await Promise.all([
      page.waitForEvent('filechooser', { timeout: 5000 }),
      attach.click(),
    ]);
    await fileChooser.setFiles(filePath);

    // Preview appears.
    await expect(page.locator('text=fail_').first()).toBeVisible({ timeout: 5000 });

    // Type text + send → upload fails.
    await page.locator('textarea').first().fill('with attachment');
    await page.keyboard.press('Enter');

    // An upload-specific error surfaces on the preview (not just a generic
    // send toast), and the preview is retained so the user can act on it.
    await expect(page.locator('[data-slot="upload-error"], [role="alert"]').first()).toBeVisible({ timeout: 10000 });
    await expect(page.locator('text=fail_').first()).toBeVisible({ timeout: 5000 });
  });
});
