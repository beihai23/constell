import { test, expect, registerTestUser, createCommunity, createChannel, joinCommunity } from './fixtures';

/**
 * Real-time messaging tests.
 *
 * The channel branch below was previously weakened by a `.catch(() => {})`
 * that swallowed the chat-ready wait — so the test stayed green even when the
 * view never loaded. It is rewritten to follow the community.spec.ts rules:
 * hard assertions on the readiness signal, no swallowed selector waits, and a
 * check on BOTH the sender's and the recipient's page.
 */

/** Unique token per run so parallel/serial runs can't collide on content/names. */
function uid(prefix: string): string {
  return `${prefix}_${Date.now().toString(36)}_${Math.random().toString(36).slice(2, 6)}`;
}

test.describe('Real-time messaging', () => {
  test('user B sees channel message from user A in real-time', async ({ browser, user }) => {
    // Create a second user.
    const userB = await registerTestUser('b');

    // Setup community + channel with unique names (no cross-run collision).
    const community = await createCommunity(user.accessToken, uid('RT'));
    const channel = await createChannel(user.accessToken, community.id, uid('rt'));

    // User B joins the community so the WS push targets them as a member.
    await joinCommunity(userB.accessToken, community.id, userB.userId);

    // Open two browser contexts.
    const ctxA = await browser.newContext();
    const ctxB = await browser.newContext();

    const pageA = await ctxA.newPage();
    const pageB = await ctxB.newPage();

    try {
      // Inject auth for both users and navigate to the channel.
      for (const [page, u] of [
        [pageA, user],
        [pageB, userB],
      ] as const) {
        await page.goto('/login');
        await page.evaluate(
          (tokens: { at: string; rt: string }) => {
            localStorage.setItem('constell_access_token', tokens.at);
            localStorage.setItem('constell_refresh_token', tokens.rt);
          },
          { at: u.accessToken, rt: u.refreshToken },
        );
        await page.goto(`/${community.id}/${channel.id}`);
      }

      // Wait for BOTH chat views to be interactive — NO swallowed .catch().
      // If the view fails to load this throws and the test fails honestly.
      await expect(pageA.locator('[aria-label="Send message"]')).toBeVisible({ timeout: 10000 });
      await expect(pageB.locator('[aria-label="Send message"]')).toBeVisible({ timeout: 10000 });

      // Ensure B's WS is connected AND the channel subscription has settled.
      // "Send button visible" doesn't guarantee subscribe completed — B can
      // miss the live push if A sends too soon. Wait for the connection bar
      // (shown only when not CONNECTED) to be absent, then a short settle.
      await expect(pageB.getByText(/Connecting\.\.\.|Reconnecting\.\.\.|Disconnected/)).toHaveCount(0, { timeout: 10000 });
      await pageB.waitForTimeout(1500);

      // User A sends a message via the real UI (textarea + Enter).
      const rtMessage = uid('Realtime');
      await pageA.locator('textarea').first().fill(rtMessage);
      await pageA.keyboard.press('Enter');

      // A sees its own message (catches a broken optimistic-insert / send path).
      await expect(pageA.getByText(rtMessage).first()).toBeVisible({ timeout: 10000 });
      // B receives it live via WS push — the assertion that mattered all along.
      await expect(pageB.getByText(rtMessage).first()).toBeVisible({ timeout: 15000 });
    } finally {
      await ctxA.close();
      await ctxB.close();
    }
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
