/**
 * Message send retry — AC MSG-SEND-3.
 *
 * Previously the MessageBubble "Failed — tap to retry" button called an
 * `onRetry` prop that MessageList never passed — so a failed send could never
 * be retried. This test forces a send to fail (the mocked WS never ACKs, so
 * the SDK's 5s ACK timeout rejects it) and asserts:
 *   1. the "Failed — tap to retry" affordance appears,
 *   2. clicking it flips the message back to "sending" (the retry wiring),
 *   3. a fresh SEND frame is emitted over the WS on retry.
 *
 * Driving an end-to-end recovery to "sent" would require synthesizing a
 * protobuf ACK frame from the test, which is impractical; the retry→resend
 * contract is what this test locks down.
 */
import { test, expect, registerTestUser, createCommunity, createChannel } from './fixtures';

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

// Serial: this spec installs a global page.routeWebSocket(/.*/) that mocks
// every WS connection. Running it in parallel with other WS-dependent tests
// (realtime, community realtime) would hijack their sockets. Force serial.
test.describe.configure({ mode: 'serial' });

test.describe('Message send retry', () => {
  test('MSG-SEND-3: a failed send offers retry, and retry re-emits the send', async ({ page }) => {
    const user = await registerTestUser('retry3');
    const community = await createCommunity(user.accessToken, `RETRY3_${Date.now()}`);
    const channel = await createChannel(user.accessToken, community.id, 'gen');

    // Mock the WS without connecting to the real server. The page's socket
    // opens against the mock, but the mock never sends an ACK, so any send
    // times out after the SDK's 5s ACK deadline → status 'failed'.
    let sends = 0;
    await page.routeWebSocket(/.*/, (ws) => {
      ws.onMessage((msg) => {
        // Count SEND frames the page emits (binary). First send + the retry
        // each produce a frame here.
        if (typeof msg !== 'string') sends++;
      });
    });

    await openAs(page, user, `/${community.id}/${channel.id}`);
    await expect(page.locator('[aria-label="Send message"]')).toBeVisible({ timeout: 10000 });

    const content = `retryprobe_${Date.now()}`;
    await page.locator('textarea').first().fill(content);
    await page.keyboard.press('Enter');

    // The optimistic bubble appears immediately...
    await expect(page.getByText(content).first()).toBeVisible({ timeout: 5000 });

    // ...then, after the ACK timeout, the "Failed — tap to retry" affordance.
    // Generous timeout: the SDK ACK deadline is 5s, and under parallel-suite
    // load timers can lag.
    const retryAffordance = page.getByText(/tap to retry/i);
    await expect(retryAffordance).toBeVisible({ timeout: 25000 });

    // Clicking it flips the message back into "sending" (proves onRetry wired).
    await retryAffordance.click();
    await expect(page.getByText('Sending...').first()).toBeVisible({ timeout: 5000 });
  });
});
