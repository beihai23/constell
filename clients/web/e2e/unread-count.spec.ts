/**
 * Unread COUNT correctness through the web client — covers the invariants the
 * "move unread read-state to Postgres" refactor introduced, at the UI layer.
 *
 * The refactor's three headline guarantees:
 *   1. Exact counts (3 messages → badge "3", not just "some").
 *   2. Sender exclusion (a user's own message does not tally their unread).
 *   3. Reload stability / no drift (Postgres-backed count survives reload —
 *      the class of bug behind "count was wrong on reload").
 *
 * WHY EVERY DECISIVE ASSERTION IS POST-RELOAD:
 * Two independent reasons converge on the same design.
 *  (a) The client already excludes own messages locally (useClientEvents guard
 *      `msg.authorId !== user.id`), so a live-only assertion would mostly test
 *      the client guard, not the server refactor. Only after a reload is the
 *      server's Postgres count the sole source of truth.
 *  (b) The live WS push is racy with subscription setup: if the push is missed,
 *      the badge never appears in that session no matter how long we poll, so a
 *      live assertion is structurally flaky. A reload fetches the authoritative
 *      server count and is deterministic.
 *
 * So each test sends/acts, then reloads (or full-navigates) before asserting.
 * If the server regressed, the reloaded state would diverge and the test fails.
 *
 * Complementary to backend/tests/integration/notify_e2e_test.go, which pins the
 * same invariants via the raw /notify/unread API.
 */
import { test, expect, registerTestUser, createCommunity, createChannel, joinCommunity, sendChannelMessage } from './fixtures';

async function openAs(
  page: import('@playwright/test').Page,
  user: { accessToken: string; refreshToken: string },
  path: string,
) {
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

/** The "Send message" button only renders once a channel view has mounted. */
const SEND_BTN = '[aria-label="Send message"]';

/** A fresh full navigation re-boots the app and re-fetches the server count. */
async function reloadReady(page: import('@playwright/test').Page) {
  await page.reload();
  await expect(page.locator(SEND_BTN)).toBeVisible({ timeout: 10000 });
}

test.describe('Unread count (Postgres read-state refactor)', () => {
  test('UNREAD-EXACT-RELOAD: 3 messages to a non-viewed channel badge as "3" after reload', async ({ browser }) => {
    const owner = await registerTestUser('ex');
    const sender = await registerTestUser('ex');
    const community = await createCommunity(owner.accessToken, `EX_${Date.now()}`);
    const current = await createChannel(owner.accessToken, community.id, `cur_${Date.now()}`);
    // Capture the name locally so targeting does not depend on the API echo.
    const watchedName = `watched_${Date.now()}`;
    const watched = await createChannel(owner.accessToken, community.id, watchedName);
    await joinCommunity(sender.accessToken, community.id, sender.userId);

    const ctx = await browser.newContext();
    const page = await ctx.newPage();
    try {
      // Owner views "current" so "watched" stays unselected → its badge can show.
      await openAs(page, owner, `/${community.id}/${current.id}`);
      await expect(page.locator(SEND_BTN)).toBeVisible({ timeout: 10000 });

      for (let i = 0; i < 3; i++) {
        await sendChannelMessage(sender.accessToken, watched.id, `m_${i}`);
      }

      // Reload: the server's Postgres-backed count must report exactly 3.
      const watchedRow = page.locator('button').filter({ hasText: watchedName });
      await reloadReady(page);
      await expect(watchedRow.locator('[data-slot="unread-badge"]')).toHaveText('3', { timeout: 10000 });
    } finally {
      await ctx.close();
    }
  });

  test('UNREAD-MARKREAD-RELOAD: viewing the channel clears the badge server-side (survives reload)', async ({ browser }) => {
    const owner = await registerTestUser('mr');
    const sender = await registerTestUser('mr');
    const community = await createCommunity(owner.accessToken, `MR_${Date.now()}`);
    const current = await createChannel(owner.accessToken, community.id, `cur_${Date.now()}`);
    const watchedName = `watched_${Date.now()}`;
    const watched = await createChannel(owner.accessToken, community.id, watchedName);
    await joinCommunity(sender.accessToken, community.id, sender.userId);

    const ctx = await browser.newContext();
    const page = await ctx.newPage();
    try {
      await openAs(page, owner, `/${community.id}/${current.id}`);
      await expect(page.locator(SEND_BTN)).toBeVisible({ timeout: 10000 });

      await sendChannelMessage(sender.accessToken, watched.id, 'unread msg');
      const watchedRow = page.locator('button').filter({ hasText: watchedName });

      // Server-authoritative: reload confirms the unread actually exists before
      // we clear it (so the post-mark-read "absent" assertion is meaningful).
      await reloadReady(page);
      await expect(watchedRow.locator('[data-slot="unread-badge"]')).toHaveText('1', { timeout: 10000 });

      // Clicking the channel mounts ChannelView, which calls markChannelRead on
      // the SERVER (not just a local clear) — the f121719 fix.
      await watchedRow.click();
      await expect(page.locator(SEND_BTN)).toBeVisible({ timeout: 10000 });
      await page.waitForTimeout(1500); // let the async mark-read persist

      // Full navigation back to "current" re-fetches the server count. "watched"
      // is unselected again, so a present badge would prove the mark-read did
      // NOT persist. It must be absent.
      await page.goto(`/${community.id}/${current.id}`);
      await expect(page.locator(SEND_BTN)).toBeVisible({ timeout: 10000 });
      await expect(watchedRow.locator('[data-slot="unread-badge"]')).toHaveCount(0, { timeout: 10000 });
    } finally {
      await ctx.close();
    }
  });

  test('UNREAD-SELF-RELOAD: a user does not see their own channel message as unread after reload', async ({ browser }) => {
    const owner = await registerTestUser('se');
    const community = await createCommunity(owner.accessToken, `SE_${Date.now()}`);
    const chanAName = `chanA_${Date.now()}`;
    const chanBName = `chanB_${Date.now()}`;
    const chanA = await createChannel(owner.accessToken, community.id, chanAName);
    const chanB = await createChannel(owner.accessToken, community.id, chanBName);

    const ctx = await browser.newContext();
    const page = await ctx.newPage();
    try {
      // Owner views B so A is unselected.
      await openAs(page, owner, `/${community.id}/${chanB.id}`);
      await expect(page.locator(SEND_BTN)).toBeVisible({ timeout: 10000 });

      // Owner sends to A themselves (owner is the sender).
      await sendChannelMessage(owner.accessToken, chanA.id, 'my own msg');

      // Reload: the server must report 0 unread on A for the sender. Post-reload
      // the client's own-message guard is out of the picture — only the server's
      // sender-cursor-advance can keep this at 0.
      await reloadReady(page);
      const aRow = page.locator('button').filter({ hasText: chanAName });
      await expect(aRow.locator('[data-slot="unread-badge"]')).toHaveCount(0, { timeout: 10000 });
    } finally {
      await ctx.close();
    }
  });
});
