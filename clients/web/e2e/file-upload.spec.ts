import { test, expect, registerTestUser, createCommunity, createChannel, injectAuthAndNavigate } from './fixtures';
import { join, dirname } from 'path';
import { writeFileSync, mkdirSync, rmSync } from 'fs';
import { fileURLToPath } from 'url';

const __filename = fileURLToPath(import.meta.url);
const __dirname = dirname(__filename);

test.describe('File upload', () => {
  const tmpDir = join(__dirname, '.tmp-e2e');

  test.beforeAll(() => {
    mkdirSync(tmpDir, { recursive: true });
  });

  test.afterAll(() => {
    rmSync(tmpDir, { recursive: true, force: true });
  });

  test('upload a file in a channel message', async ({ page, user }) => {
    // Setup community + channel.
    const community = await createCommunity(user.accessToken, 'File Upload Test');
    const channel = await createChannel(user.accessToken, community.id, 'file-channel');

    // Create a temporary file to upload.
    const filePath = join(tmpDir, `upload_${Date.now()}.txt`);
    writeFileSync(filePath, 'E2E file upload test content');

    // Navigate to the channel.
    await injectAuthAndNavigate(page, user);
    await page.goto(`/${community.id}/${channel.id}`);

    // Wait for chat to load.
    await page.waitForSelector('[aria-label="Send message"]', { timeout: 10000 }).catch(() => {});

    // Click the attach file button.
    const attachButton = page.locator('[aria-label="Attach file"]').first();
    if (await attachButton.isVisible()) {
      // Set up file chooser listener before clicking.
      const [fileChooser] = await Promise.all([
        page.waitForEvent('filechooser', { timeout: 5000 }),
        attachButton.click(),
      ]);
      await fileChooser.setFiles(filePath);

      // Wait for the file to appear as an attachment preview.
      await page.waitForTimeout(2000);

      // Send the message (type some text + Enter).
      const chatInput = page.locator('textarea').first();
      await chatInput.fill('File attached');
      await page.keyboard.press('Enter');

      // Verify the message with attachment appears.
      await page.waitForSelector('text=File attached', { timeout: 10000 });
      await expect(page.locator('text=File attached').first()).toBeVisible();
    }
  });
});
