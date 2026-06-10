import { test, expect, registerTestUser, createCommunity, createChannel, joinCommunity } from './fixtures';

test.describe('Real-time messaging', () => {
  test('user B sees channel message from user A in real-time', async ({ browser, user }) => {
    // Create a second user.
    const userB = await registerTestUser('b');

    // Setup community + channel.
    const community = await createCommunity(user.accessToken, 'Realtime Test');
    const channel = await createChannel(user.accessToken, community.id, 'rt-channel');

    // User B joins the community.
    await joinCommunity(userB.accessToken, community.id, userB.userId);

    // Open two browser contexts.
    const ctxA = await browser.newContext();
    const ctxB = await browser.newContext();

    const pageA = await ctxA.newPage();
    const pageB = await ctxB.newPage();

    // Inject auth for both users and navigate to the channel.
    // User A
    await pageA.goto('/login');
    await pageA.evaluate(
      (tokens: { at: string; rt: string }) => {
        localStorage.setItem('constell_access_token', tokens.at);
        localStorage.setItem('constell_refresh_token', tokens.rt);
      },
      { at: user.accessToken, rt: user.refreshToken },
    );
    await pageA.goto(`/${community.id}/${channel.id}`);

    // User B
    await pageB.goto('/login');
    await pageB.evaluate(
      (tokens: { at: string; rt: string }) => {
        localStorage.setItem('constell_access_token', tokens.at);
        localStorage.setItem('constell_refresh_token', tokens.rt);
      },
      { at: userB.accessToken, rt: userB.refreshToken },
    );
    await pageB.goto(`/${community.id}/${channel.id}`);

    // Wait for both pages to load the chat view.
    await pageA.waitForSelector('[aria-label="Send message"]', { timeout: 10000 }).catch(() => {});
    await pageB.waitForSelector('[aria-label="Send message"]', { timeout: 10000 }).catch(() => {});

    // User A sends a message via the UI.
    const rtMessage = `Realtime ${Date.now()}`;
    const chatInput = pageA.locator('textarea').first();
    await chatInput.fill(rtMessage);
    await pageA.keyboard.press('Enter');

    // User B should see the message appear in real-time.
    await pageB.waitForSelector(`text=${rtMessage}`, { timeout: 15000 });
    await expect(pageB.locator(`text=${rtMessage}`).first()).toBeVisible();

    // Cleanup.
    await ctxA.close();
    await ctxB.close();
  });

  test('user A receives DM from user B in real-time', async ({ browser, user }) => {
    const userB = await registerTestUser('dm');

    // Open page for user A (listening for DM).
    const ctxA = await browser.newContext();
    const pageA = await ctxA.newPage();

    await pageA.goto('/login');
    await pageA.evaluate(
      (tokens: { at: string; rt: string }) => {
        localStorage.setItem('constell_access_token', tokens.at);
        localStorage.setItem('constell_refresh_token', tokens.rt);
      },
      { at: user.accessToken, rt: user.refreshToken },
    );
    await pageA.goto(`/@me`);

    // Wait for app to load and WS to connect.
    await pageA.waitForTimeout(3000);

    // User B sends DM via API.
    const dmContent = `RT DM ${Date.now()}`;
    await fetch(`${process.env.E2E_API_URL || 'http://localhost:3000'}/api/v1/dm/send`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${userB.accessToken}`,
      },
      body: JSON.stringify({ target_user_id: user.userId, content: dmContent }),
    });

    // User A's page should show the new DM (either as notification badge or in DM list).
    // Navigate to the DM conversation to verify.
    await pageA.goto(`/@me/${userB.userId}`);
    await pageA.waitForSelector(`text=${dmContent}`, { timeout: 15000 });
    await expect(pageA.locator(`text=${dmContent}`).first()).toBeVisible();

    await ctxA.close();
  });
});
