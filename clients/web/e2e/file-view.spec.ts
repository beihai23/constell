/**
 * File view — AC FILE-VIEW-1. After the dev deployment fix (MINIO_BASE_URL
 * browser-reachable + public-read bucket), a received image attachment
 * actually loads in the recipient's browser.
 */
import { test, expect, registerTestUser, createCommunity, createChannel, joinCommunity } from './fixtures';
import { writeFileSync, mkdirSync, rmSync } from 'fs';
import { join, dirname } from 'path';
import { fileURLToPath } from 'url';

const API_BASE = process.env.E2E_API_URL || 'http://localhost:3000';
const __dirname = dirname(fileURLToPath(import.meta.url));
const tmpDir = join(__dirname, '.tmp-e2e-fileview');

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

// A tiny valid PNG (1x1 transparent).
const PNG_BYTES = Buffer.from(
  'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNkYAAAAAYAAjCB0C8AAAAASUVORK5CYII=',
  'base64',
);

test.describe('File view', () => {
  test.beforeAll(() => mkdirSync(tmpDir, { recursive: true }));
  test.afterAll(() => rmSync(tmpDir, { recursive: true, force: true }));

  test('FILE-VIEW-1: a received image attachment renders for the recipient', async ({ browser }) => {
    const owner = await registerTestUser('fvO');
    const recipient = await registerTestUser('fvR');
    const community = await createCommunity(owner.accessToken, `FV_${Date.now()}`);
    const channel = await createChannel(owner.accessToken, community.id, 'gen');
    await joinCommunity(recipient.accessToken, community.id, recipient.userId);

    // Owner uploads an image and sends it as an attachment.
    const imgPath = join(tmpDir, `pic_${Date.now()}.png`);
    writeFileSync(imgPath, PNG_BYTES);
    const upRes = await fetch(`${API_BASE}/api/v1/files/upload`, {
      method: 'POST',
      headers: { Authorization: `Bearer ${owner.accessToken}` },
      body: (() => {
        const fd = new FormData();
        // @ts-expect-error Node FormData + Blob
        fd.append('file', new Blob([PNG_BYTES], { type: 'image/png' }), 'pic.png');
        return fd;
      })(),
    });
    const fileId = (await upRes.json()).id as string;
    await fetch(`${API_BASE}/api/v1/channels/${channel.id}/messages`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${owner.accessToken}` },
      body: JSON.stringify({ content: 'see pic', file_ids: [fileId] }),
    });

    // Recipient opens the channel and the image renders (naturalWidth > 0).
    const ctx = await browser.newContext();
    const page = await ctx.newPage();
    try {
      await openAs(page, recipient, `/${community.id}/${channel.id}`);
      await expect(page.locator('[aria-label="Send message"]')).toBeVisible({ timeout: 10000 });

      const img = page.locator('img[alt="pic.png"]').first();
      await expect(img).toBeVisible({ timeout: 10000 });
      // The image actually loaded (not broken) — naturalWidth is set.
      await expect(img).toHaveJSProperty('naturalWidth', 1);
    } finally {
      await ctx.close();
    }
  });
});
