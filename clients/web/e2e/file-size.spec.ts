/**
 * File size limit — AC FILE-SIZE-1.
 *
 * Selecting a file over the 25 MB cap must be rejected at the client (never
 * uploaded) with a clear message, and must not appear as a pending preview.
 */
import { test, expect, registerTestUser, createCommunity, createChannel } from './fixtures';
import { mkdirSync, rmSync, writeFileSync } from 'fs';
import { join, dirname } from 'path';
import { fileURLToPath } from 'url';
import type { Page } from '@playwright/test';

const __dirname = dirname(fileURLToPath(import.meta.url));
const tmpDir = join(__dirname, '.tmp-e2e-size');

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

test.describe('File size limit', () => {
  test.beforeAll(() => mkdirSync(tmpDir, { recursive: true }));
  test.afterAll(() => rmSync(tmpDir, { recursive: true, force: true }));

  test('FILE-SIZE-1: an oversized file is rejected with a message and never previewed', async ({ page, user }) => {
    const community = await createCommunity(user.accessToken, `SIZE_${Date.now()}`);
    const channel = await createChannel(user.accessToken, community.id, 'gen');

    // 26 MB file — just over the 25 MB cap.
    const bigPath = join(tmpDir, `big_${Date.now()}.bin`);
    writeFileSync(bigPath, Buffer.alloc(26 * 1024 * 1024, 0));

    // A small valid file, to prove the picker itself still works.
    const okPath = join(tmpDir, `ok_${Date.now()}.txt`);
    writeFileSync(okPath, 'small');

    await openAs(page, user, `/${community.id}/${channel.id}`);
    await expect(page.locator('[aria-label="Send message"]')).toBeVisible({ timeout: 10000 });

    const attach = page.locator('[aria-label="Attach file"]').first();

    // Try the oversized file.
    const [bigChooser] = await Promise.all([
      page.waitForEvent('filechooser', { timeout: 5000 }),
      attach.click(),
    ]);
    await bigChooser.setFiles(bigPath);

    // Rejection surfaces as a toast; no preview for the big file.
    await expect(page.locator('[data-sonner-toast]').first()).toBeVisible({ timeout: 5000 });
    await expect(page.locator('text=big_').first()).toHaveCount(0);

    // A small file is still accepted (preview appears) — the limit isn't blocking everything.
    const [okChooser] = await Promise.all([
      page.waitForEvent('filechooser', { timeout: 5000 }),
      attach.click(),
    ]);
    await okChooser.setFiles(okPath);
    await expect(page.locator('text=ok_').first()).toBeVisible({ timeout: 5000 });
  });
});
