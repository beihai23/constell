/**
 * File upload progress — AC FILE-UPLOAD-1.
 *
 * A large upload used to look frozen (fetch can't report request-body progress,
 * and the preview had no affordance while the upload was in flight). The SDK
 * now uploads via XHR with a progress callback; the pending-file preview shows
 * a progress bar while uploading.
 */
import { test, expect, registerTestUser, createCommunity, createChannel } from './fixtures';
import { writeFileSync, mkdirSync, rmSync } from 'fs';
import { join, dirname } from 'path';
import { fileURLToPath } from 'url';
import type { Page } from '@playwright/test';

const __dirname = dirname(fileURLToPath(import.meta.url));
const tmpDir = join(__dirname, '.tmp-e2e-progress');

async function openAs(page: Page, user: { accessToken: string; refreshToken: string }, path: string) {
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

test.describe('File upload progress', () => {
  test.beforeAll(() => mkdirSync(tmpDir, { recursive: true }));
  test.afterAll(() => rmSync(tmpDir, { recursive: true, force: true }));

  test('FILE-UPLOAD-1: a progress indicator shows while a file uploads', async ({ page, user }) => {
    const community = await createCommunity(user.accessToken, `UP_${Date.now()}`);
    const channel = await createChannel(user.accessToken, community.id, 'gen');

    const filePath = join(tmpDir, `progress_${Date.now()}.txt`);
    writeFileSync(filePath, 'progress-test content');

    // Delay the upload response so the in-flight upload window is observable.
    await page.route('**/api/v1/files/upload**', async (route) => {
      await new Promise((r) => setTimeout(r, 2000));
      await route.continue();
    });

    await openAs(page, user, `/${community.id}/${channel.id}`);
    await expect(page.locator('[aria-label="Send message"]')).toBeVisible({ timeout: 10000 });

    // Pick the file + send (upload starts on send).
    const attach = page.locator('[aria-label="Attach file"]').first();
    const [chooser] = await Promise.all([
      page.waitForEvent('filechooser', { timeout: 5000 }),
      attach.click(),
    ]);
    await chooser.setFiles(filePath);
    await page.locator('textarea').first().fill('with attachment');
    await page.keyboard.press('Enter');

    // While the upload is in flight, the preview shows a progress bar.
    await expect(page.locator('[data-slot="upload-progress"]').first()).toBeVisible({ timeout: 5000 });
  });
});
