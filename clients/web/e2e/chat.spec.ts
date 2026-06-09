import {
  test,
  expect,
  registerTestUser,
  createCommunity,
  createChannel,
  injectAuthAndNavigate,
} from './fixtures';

test.describe('Chat', () => {
  test('send and view a channel message', async ({ page, user }) => {
    // Setup: create community + channel via API.
    const community = await createCommunity(user.accessToken, 'Chat Test Community');
    const channel = await createChannel(user.accessToken, community.id, 'general');

    // Navigate to the channel.
    await injectAuthAndNavigate(page, user);
    await page.goto(`/${community.id}/${channel.id}`);

    // Wait for the chat input to be visible.
    await page.waitForSelector('[aria-label="Send message"]', { timeout: 10000 });

    // Type and send a message.
    const messageContent = `Hello E2E! ${Date.now()}`;
    const chatInput = page.locator('textarea').first();
    await chatInput.fill(messageContent);
    await page.keyboard.press('Enter');

    // Wait for the message to appear in the message list.
    await page.waitForSelector(`text=${messageContent}`, { timeout: 10000 });
    await expect(page.locator(`text=${messageContent}`).first()).toBeVisible();
  });

  test('view message history in a channel', async ({ page, user }) => {
    // Setup: create community + channel, send a message via API.
    const community = await createCommunity(user.accessToken, 'History Test Community');
    const channel = await createChannel(user.accessToken, community.id, 'history-channel');

    const msgContent = `History message ${Date.now()}`;
    await fetch(`${process.env.E2E_API_URL || 'http://localhost:3000'}/api/v1/channels/${channel.id}/messages`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${user.accessToken}`,
      },
      body: JSON.stringify({ content: msgContent }),
    });

    // Navigate to the channel.
    await injectAuthAndNavigate(page, user);
    await page.goto(`/${community.id}/${channel.id}`);

    // Wait for the message to appear in history.
    await page.waitForSelector(`text=${msgContent}`, { timeout: 10000 });
    await expect(page.locator(`text=${msgContent}`).first()).toBeVisible();
  });

  test('send and view a DM', async ({ page, user }) => {
    // Create a second user to DM.
    const peer = await registerTestUser();

    // Send DM via API to create the conversation.
    const dmContent = `DM from E2E ${Date.now()}`;
    await fetch(`${process.env.E2E_API_URL || 'http://localhost:3000'}/api/v1/dm/send`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${user.accessToken}`,
      },
      body: JSON.stringify({ target_user_id: peer.userId, content: dmContent }),
    });

    // Navigate to DM view.
    await injectAuthAndNavigate(page, user);
    await page.goto(`/@me/${peer.userId}`);

    // Wait for the DM to appear.
    await page.waitForSelector(`text=${dmContent}`, { timeout: 10000 });
    await expect(page.locator(`text=${dmContent}`).first()).toBeVisible();
  });
});
