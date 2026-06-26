/**
 * Community feature E2E — the "is the community feature actually usable?" suite.
 *
 * Context: a prior e2e-test report (docs/e2e-test-report-2026-06-09.md) documented
 * ten "cheats" that systematically weakened tests to hide community regressions
 * (self-join instead of owner-kick, skip the realtime channel push, .catch(()=>{})
 * swallowing selector waits, only asserting on the sender's own page). The result
 * was a suite that stayed green while the feature was unusable.
 *
 * Design rules for THIS file (do not violate):
 *  1. Cross-user flows are asserted on a SECOND browser context, never just the
 *     sender's own page. This is the only shape that exercises the full
 *     UI -> SDK -> WS -> NATS -> WS -> SDK -> store -> UI chain.
 *  2. Messages are sent via the real UI (textarea + Enter), never REST, when the
 *     point of the test is to verify sending/realtime.
 *  3. No `.catch(() => {})` to swallow selector waits — let timeouts fail honestly.
 *  4. Every test uses unique random content/names so concurrent runs can't collide.
 */

import { test, expect } from '@playwright/test';
import {
  registerTestUser,
  createCommunity,
  createChannel,
  joinCommunity,
  type TestUser,
} from './fixtures';

const API_BASE = process.env.E2E_API_URL || 'http://localhost:3000';

// ---------------------------------------------------------------------------
// Local helpers
// ---------------------------------------------------------------------------

/** Unique token per test run — keeps concurrent runs/cold DBs collision-free. */
function uid(prefix: string): string {
  return `${prefix}_${Date.now().toString(36)}_${Math.random().toString(36).slice(2, 6)}`;
}

/**
 * Open the app as `user` and navigate to `path` with auth already in place.
 * Mirrors the inline-inject pattern in realtime.spec.ts (lines 24-32) but
 * reusable. `injectAuthAndNavigate` in fixtures hardcodes a jump to /@me; we
 * usually want a channel URL, so we inject then go to the given path.
 */
async function openAs(
  page: import('@playwright/test').Page,
  user: TestUser,
  path: string,
) {
  await page.goto('/login');
  await page.evaluate(
    (tokens: { at: string; rt: string }) => {
      localStorage.setItem('constell_access_token', tokens.at);
      localStorage.setItem('constell_refresh_token', tokens.rt);
    },
    { at: user.accessToken, rt: user.refreshToken },
  );
  await page.goto(path);
}

/** Wait for the chat view to be interactive on a page (send button visible). */
async function waitForChatReady(page: import('@playwright/test').Page) {
  await expect(page.locator('[aria-label="Send message"]')).toBeVisible({ timeout: 10000 });
}

/** Type `content` into the chat input and submit via Enter. */
async function sendViaUI(page: import('@playwright/test').Page, content: string) {
  await page.locator('textarea').first().fill(content);
  await page.keyboard.press('Enter');
}

/** REST: fetch the seeded/first channel id for a community as the given user. */
async function firstChannelId(token: string, communityId: string): Promise<string> {
  const res = await fetch(`${API_BASE}/api/v1/communities/${communityId}/channels`, {
    headers: { Authorization: `Bearer ${token}` },
  });
  if (!res.ok) throw new Error(`getChannels ${res.status}`);
  const body = await res.json();
  if (!body.channels?.length) throw new Error('no channels in community');
  return body.channels[0].id as string;
}

/** REST: owner removes a member from a community. */
async function kickMember(token: string, communityId: string, userId: string) {
  const res = await fetch(`${API_BASE}/api/v1/communities/${communityId}/members/${userId}`, {
    method: 'DELETE',
    headers: { Authorization: `Bearer ${token}` },
  });
  if (!res.ok) throw new Error(`kick ${res.status}`);
}

// ---------------------------------------------------------------------------
// A. Core happy-path tests — proves "the community feature works"
// ---------------------------------------------------------------------------

test.describe('Community — core usability', () => {
  test('create a community via UI lands in the seeded general channel ready to type', async ({ page }) => {
    const user = await registerTestUser('creator');
    await openAs(page, user, '/@me');

    await page.getByLabel('Add a Community').click();
    const name = uid('Comm');
    await page.getByLabel('Community name').fill(name);
    await page.getByRole('button', { name: /^create$/i }).click();

    // Lands inside the new community's default channel.
    await expect(page).toHaveURL(/\/[0-9a-f-]{36}\/[0-9a-f-]{36}/, { timeout: 10000 });
    await waitForChatReady(page);

    // Seeded "general" channel appears in the list.
    await expect(page.getByText('Text Channels')).toBeVisible({ timeout: 5000 });
    await expect(page.getByText('general').first()).toBeVisible({ timeout: 5000 });

    // The new community appears on the left rail.
    await expect(page.getByRole('button', { name })).toBeVisible({ timeout: 5000 });
  });

  test('B sees a channel message typed+sent by A in real-time (full WS chain)', async ({ browser }) => {
    const owner = await registerTestUser('owner');
    const listener = await registerTestUser('listener');

    const community = await createCommunity(owner.accessToken, uid('RT'));
    const channelId = await firstChannelId(owner.accessToken, community.id);
    // B joins so the WS push targets them as a member.
    await joinCommunity(listener.accessToken, community.id, listener.userId);

    const ctxA = await browser.newContext();
    const ctxB = await browser.newContext();
    const pageA = await ctxA.newPage();
    const pageB = await ctxB.newPage();
    try {
      await openAs(pageA, owner, `/${community.id}/${channelId}`);
      await openAs(pageB, listener, `/${community.id}/${channelId}`);
      await waitForChatReady(pageA);
      await waitForChatReady(pageB);

      const content = uid('hello');
      await sendViaUI(pageA, content);

      // A sees its own message.
      await expect(pageA.getByText(content).first()).toBeVisible({ timeout: 10000 });
      // B receives it live via WS push — this is the assertion that was skipped before.
      await expect(pageB.getByText(content).first()).toBeVisible({ timeout: 15000 });
    } finally {
      await ctxA.close();
      await ctxB.close();
    }
  });

  test('a sent message persists across a full page reload', async ({ page }) => {
    const user = await registerTestUser('persist');
    const community = await createCommunity(user.accessToken, uid('Persist'));
    const channelId = await firstChannelId(user.accessToken, community.id);

    await openAs(page, user, `/${community.id}/${channelId}`);
    await waitForChatReady(page);

    const content = uid('persisted');
    await sendViaUI(page, content);
    await expect(page.getByText(content).first()).toBeVisible({ timeout: 10000 });

    // The optimistic insert shows the message before the server ACK persists
    // it. Under load the persistence window widens, so a naive reload races.
    // Poll the REST history until the message is actually durably stored,
    // THEN reload — the assertion (history comes back) stays honest.
    const deadline = Date.now() + 10000;
    let persisted = false;
    while (Date.now() < deadline) {
      const res = await fetch(`${API_BASE}/api/v1/channels/${channelId}/messages`, {
        headers: { Authorization: `Bearer ${user.accessToken}` },
      });
      const body = await res.text();
      if (body.includes(content)) {
        persisted = true;
        break;
      }
      await page.waitForTimeout(300);
    }
    expect(persisted, 'message never persisted on server').toBeTruthy();

    // Reload — history must come back from getChannelHistory (MessageList mount effect).
    await page.reload();
    await waitForChatReady(page);
    await expect(page.getByText(content).first()).toBeVisible({ timeout: 10000 });
  });

  test('a joining member can read prior channel history (the old "cheat #1" scenario)', async ({ page }) => {
    const owner = await registerTestUser('owner');
    const joiner = await registerTestUser('joiner');

    const community = await createCommunity(owner.accessToken, uid('Hist'));
    const channelId = await firstChannelId(owner.accessToken, community.id);

    const seeded = uid('owner-said');
    await fetch(`${API_BASE}/api/v1/channels/${channelId}/messages`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${owner.accessToken}` },
      body: JSON.stringify({ content: seeded }),
    });

    await joinCommunity(joiner.accessToken, community.id, joiner.userId);

    await openAs(page, joiner, `/${community.id}/${channelId}`);
    await waitForChatReady(page);

    // A non-owner member must see the owner's message — this used to 403.
    await expect(page.getByText(seeded).first()).toBeVisible({ timeout: 10000 });
  });

  test('a joining member can send a message in a channel', async ({ page }) => {
    const owner = await registerTestUser('owner');
    const joiner = await registerTestUser('joiner');

    const community = await createCommunity(owner.accessToken, uid('Send'));
    const channelId = await firstChannelId(owner.accessToken, community.id);
    await joinCommunity(joiner.accessToken, community.id, joiner.userId);

    await openAs(page, joiner, `/${community.id}/${channelId}`);
    await waitForChatReady(page);

    const content = uid('joiner-says');
    await sendViaUI(page, content);
    await expect(page.getByText(content).first()).toBeVisible({ timeout: 10000 });
  });
});

// ---------------------------------------------------------------------------
// B. Multi-user / state tests
// ---------------------------------------------------------------------------

test.describe('Community — multi-user & state', () => {
  test('member list shows both members when two users are present', async ({ browser }) => {
    const owner = await registerTestUser('owner');
    const member = await registerTestUser('member');

    const community = await createCommunity(owner.accessToken, uid('Members'));
    const channelId = await firstChannelId(owner.accessToken, community.id);
    await joinCommunity(member.accessToken, community.id, member.userId);

    const ctxA = await browser.newContext();
    const ctxB = await browser.newContext();
    const pageA = await ctxA.newPage();
    const pageB = await ctxB.newPage();
    try {
      await openAs(pageA, owner, `/${community.id}/${channelId}`);
      await openAs(pageB, member, `/${community.id}/${channelId}`);
      await waitForChatReady(pageA);
      await waitForChatReady(pageB);

      // The member list is hidden by default (uiStore.showMemberList=false).
      // Toggle it on via the chat header button so MemberList mounts + fetches.
      await pageA.getByLabel('Toggle member list').click();

      // Right-side member list header reflects count.
      await expect(pageA.getByText(/MEMBERS.*2/)).toBeVisible({ timeout: 10000 });
      // Both nicknames render.
      await expect(pageA.getByText(owner.nickname)).toBeVisible({ timeout: 10000 });
      await expect(pageA.getByText(member.nickname)).toBeVisible({ timeout: 10000 });
    } finally {
      await ctxA.close();
      await ctxB.close();
    }
  });

  test('an unread badge appears on the channel for the recipient (notify -> WS -> store -> UI)', async ({ browser }) => {
    const owner = await registerTestUser('owner');
    const member = await registerTestUser('member');

    const community = await createCommunity(owner.accessToken, uid('Unread'));
    const channelId = await firstChannelId(owner.accessToken, community.id);
    await joinCommunity(member.accessToken, community.id, member.userId);

    const ctxA = await browser.newContext();
    const ctxB = await browser.newContext();
    const pageA = await ctxA.newPage();
    const pageB = await ctxB.newPage();
    try {
      // B opens the community but on a DIFFERENT channel view? We keep it simple:
      // B sits on the community home (no channel selected) so the channel stays
      // unselected and its unread badge is allowed to render.
      await openAs(pageA, owner, `/${community.id}/${channelId}`);
      await openAs(pageB, member, `/@me`);
      // Navigate B to the community root (channel list, no active channel).
      await pageB.getByRole('button', { name: community.name }).click();
      await waitForChatReady(pageA);

      // Mark a baseline by having B briefly view the channel to clear any seed.
      // (The "general" channel is empty so there is nothing to clear; we proceed.)

      const content = uid('unread-probe');
      await sendViaUI(pageA, content);
      await expect(pageA.getByText(content).first()).toBeVisible({ timeout: 10000 });

      // B's channel list shows an unread badge with count "1" on the channel.
      const channelRow = pageB.locator('button', { hasText: 'general' });
      await expect(channelRow.first()).toBeVisible({ timeout: 10000 });
      await expect(channelRow.locator('text=1').first()).toBeVisible({ timeout: 15000 });
    } finally {
      await ctxA.close();
      await ctxB.close();
    }
  });

  test('both users appear online to each other (presence)', async ({ browser }) => {
    const owner = await registerTestUser('owner');
    const member = await registerTestUser('member');

    const community = await createCommunity(owner.accessToken, uid('Presence'));
    const channelId = await firstChannelId(owner.accessToken, community.id);
    await joinCommunity(member.accessToken, community.id, member.userId);

    const ctxA = await browser.newContext();
    const ctxB = await browser.newContext();
    const pageA = await ctxA.newPage();
    const pageB = await ctxB.newPage();
    try {
      await openAs(pageA, owner, `/${community.id}/${channelId}`);
      await openAs(pageB, member, `/${community.id}/${channelId}`);
      await waitForChatReady(pageA);
      await waitForChatReady(pageB);

      // The member list is hidden by default; toggle it on so presence groups render.
      await pageA.getByLabel('Toggle member list').click();

      // Both connected => ONLINE group has both members. Presence is eventually
      // consistent (Redis pull + WS push), so allow generous timeout.
      await expect(pageA.getByText(/ONLINE.*2/)).toBeVisible({ timeout: 20000 });
    } finally {
      await ctxA.close();
      await ctxB.close();
    }
  });

  test('a user can discover a public community via search and join it', async ({ page }) => {
    const owner = await registerTestUser('owner');
    const seeker = await registerTestUser('seeker');

    const name = uid('Findable');
    const community = await createCommunity(owner.accessToken, name);
    const channelId = await firstChannelId(owner.accessToken, community.id);

    await openAs(page, seeker, '/@me');

    // The app uses a sidebar search input (ChannelList) — ⌘K/Ctrl+K focuses it,
    // typing + Enter runs the /search API and renders results inline.
    const searchInput = page.getByPlaceholder('Search... (Enter to search all)');
    await expect(searchInput).toBeVisible({ timeout: 10000 });
    await searchInput.click();
    await searchInput.fill(name);

    // The community result row is a <div> carrying the name <p> plus an inner
    // <button>Join</button> (a div, not a button, so the Join button can be a
    // real <button> with click-stopping). Locate the row by its name text, then
    // click the Join button within it. force:true because the result row
    // re-renders on Enter and Playwright's actionability check occasionally
    // mis-targets.
    const resultName = page.getByText(name, { exact: true });

    // search-service indexes new communities asynchronously, so the first search
    // may run before the row is indexed. Poll: re-issue the search until the
    // community is found (eventual consistency), without weakening the assertion.
    const deadline = Date.now() + 20000;
    let found = false;
    while (Date.now() < deadline) {
      await page.keyboard.press('Enter');
      try {
        await expect(resultName).toBeVisible({ timeout: 3000 });
        found = true;
        break;
      } catch {
        // not indexed yet — retry
      }
    }
    expect(found, 'public community never became searchable').toBeTruthy();

    // Click the Join button in that row.
    const joinButton = page.getByRole('button', { name: 'Join' });
    await joinButton.click({ force: true });

    // After joining, the community must appear on the rail immediately — the
    // onClick handler now pushes it into communitiesStore (no reload needed).
    // This was a real bug: previously the rail only updated after a reload.
    await expect(page.getByRole('button', { name })).toBeVisible({ timeout: 10000 });
    void channelId; // channelId referenced only to keep the helper call symmetric
  });
});

// ---------------------------------------------------------------------------
// C. Boundary / error paths (regression guards)
// ---------------------------------------------------------------------------

test.describe('Community — boundary & error paths', () => {
  test('an owner kicking a member removes them from the community on next view', async ({ page }) => {
    const owner = await registerTestUser('owner');
    const victim = await registerTestUser('victim');

    const community = await createCommunity(owner.accessToken, uid('Kick'));
    const channelId = await firstChannelId(owner.accessToken, community.id);
    await joinCommunity(victim.accessToken, community.id, victim.userId);

    // Victim loads the channel to confirm membership works first.
    await openAs(page, victim, `/${community.id}/${channelId}`);
    await waitForChatReady(page);

    // Owner kicks them via REST.
    await kickMember(owner.accessToken, community.id, victim.userId);

    // After reload, the victim no longer has the community.
    await page.goto('/@me');
    await expect(page.getByRole('button', { name: community.name })).toHaveCount(0, { timeout: 10000 });
  });

  test('a non-member cannot read a channel\'s messages', async ({ page }) => {
    const owner = await registerTestUser('owner');
    const stranger = await registerTestUser('stranger');

    const community = await createCommunity(owner.accessToken, uid('Private'));
    const channelId = await firstChannelId(owner.accessToken, community.id);

    const secret = uid('secret');
    await fetch(`${API_BASE}/api/v1/channels/${channelId}/messages`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${owner.accessToken}` },
      body: JSON.stringify({ content: secret }),
    });

    // Stranger never joins. Open the channel URL directly.
    await openAs(page, stranger, `/${community.id}/${channelId}`);

    // They must NOT see the owner's secret message within a reasonable window.
    await expect(page.getByText(secret)).toHaveCount(0, { timeout: 8000 });
  });

  test('reload restores joined communities on the rail (useInitialData re-fetch)', async ({ page }) => {
    const user = await registerTestUser('reloader');
    const name = uid('Reload');
    const community = await createCommunity(user.accessToken, name);
    await openAs(page, user, '/@me');

    await expect(page.getByRole('button', { name })).toBeVisible({ timeout: 10000 });

    // Full reload: useInitialData must repopulate the rail.
    await page.reload();
    await expect(page.getByRole('button', { name })).toBeVisible({ timeout: 10000 });
  });

  test('owner can create a new channel via the UI and lands in it', async ({ page }) => {
    const owner = await registerTestUser('owner');
    const community = await createCommunity(owner.accessToken, uid('Channels'));
    const channelId = await firstChannelId(owner.accessToken, community.id);

    await openAs(page, owner, `/${community.id}/${channelId}`);
    await waitForChatReady(page);

    // Open the create-channel dialog from the "+" next to TEXT CHANNELS.
    await page.getByLabel('Create channel').click();
    await expect(page.getByRole('heading', { name: 'Create Channel' })).toBeVisible({ timeout: 5000 });

    const channelName = uid('chan');
    await page.locator('#create-channel-name').fill(channelName);
    await page.getByRole('button', { name: 'Create' }).click();

    // The dialog closes and the app navigates into the new channel.
    await expect(page).toHaveURL(/\/[0-9a-f-]{36}\/[0-9a-f-]{36}/, { timeout: 10000 });
    // The new channel appears in the channel list.
    await expect(page.getByText(channelName).first()).toBeVisible({ timeout: 5000 });
    await waitForChatReady(page);
  });

  test('creating a second community keeps both on the rail and switchable', async ({ page }) => {
    const user = await registerTestUser('multi');
    await openAs(page, user, '/@me');

    const first = uid('First');
    const second = uid('Second');

    for (const name of [first, second]) {
      await page.getByLabel('Add a Community').click();
      await page.getByLabel('Community name').fill(name);
      await page.getByRole('button', { name: /^create$/i }).click();
      await expect(page).toHaveURL(/\/[0-9a-f-]{36}\/[0-9a-f-]{36}/, { timeout: 10000 });
      await waitForChatReady(page);
      // Return to DM home between creations so the rail "+" is reachable.
      await page.goto('/@me');
    }

    // Both communities present.
    await expect(page.getByRole('button', { name: first })).toBeVisible({ timeout: 10000 });
    await expect(page.getByRole('button', { name: second })).toBeVisible({ timeout: 10000 });

    // Clicking the first rail icon takes you into the first community. Rail
    // buttons navigate to the community root (/:communityId, no channel) — so we
    // assert the URL reflects the first community, not a channel shape.
    await page.getByRole('button', { name: first }).click();
    await expect(page).toHaveURL(/\/[0-9a-f-]{36}$/, { timeout: 10000 });
    // Navigating into the first community must not have disturbed the second on the rail.
    await expect(page.getByRole('button', { name: second })).toBeVisible({ timeout: 5000 });
  });
});
