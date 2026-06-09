import { test as base, expect, type Page } from '@playwright/test';

// =============================================================================
// API helpers — create test users and communities via REST API
// =============================================================================

const API_BASE = process.env.E2E_API_URL || 'http://localhost:3000';

interface TestUser {
  email: string;
  password: string;
  nickname: string;
  userId: string;
  accessToken: string;
  refreshToken: string;
}

/** Register a test user via the REST API. */
export async function registerTestUser(prefix = 'e2e'): Promise<TestUser> {
  const unique = `${prefix}_${Date.now()}_${Math.random().toString(36).slice(2, 8)}`;
  const email = `${unique}@test.com`;
  const password = 'testPassword123!';
  const nickname = `Test_${unique}`;

  const resp = await fetch(`${API_BASE}/api/v1/auth/register`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ email, password, nickname }),
  });

  if (!resp.ok) {
    const body = await resp.text();
    throw new Error(`register failed (${resp.status}): ${body}`);
  }

  const data = await resp.json();
  return {
    email,
    password,
    nickname,
    userId: data.user_id,
    accessToken: data.access_token,
    refreshToken: data.refresh_token,
  };
}

/** Login a user via the REST API. */
export async function loginTestUser(email: string, password: string): Promise<TestUser> {
  const resp = await fetch(`${API_BASE}/api/v1/auth/login`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ email, password }),
  });

  if (!resp.ok) {
    throw new Error(`login failed (${resp.status})`);
  }

  const data = await resp.json();
  return {
    email,
    password,
    nickname: '',
    userId: data.user_id,
    accessToken: data.access_token,
    refreshToken: data.refresh_token,
  };
}

/** Create a community via REST API. */
export async function createCommunity(token: string, name: string) {
  const resp = await fetch(`${API_BASE}/api/v1/communities`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      Authorization: `Bearer ${token}`,
    },
    body: JSON.stringify({ name, description: 'E2E test community' }),
  });

  if (!resp.ok) throw new Error(`create community failed (${resp.status})`);
  return resp.json() as Promise<{ id: string; name: string }>;
}

/** Create a channel in a community via REST API. */
export async function createChannel(token: string, communityId: string, name: string) {
  const resp = await fetch(`${API_BASE}/api/v1/communities/${communityId}/channels`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      Authorization: `Bearer ${token}`,
    },
    body: JSON.stringify({ name }),
  });

  if (!resp.ok) throw new Error(`create channel failed (${resp.status})`);
  return resp.json() as Promise<{ id: string; name: string }>;
}

/** Add a member to a community via REST API. */
export async function joinCommunity(token: string, communityId: string, userId: string) {
  const resp = await fetch(`${API_BASE}/api/v1/communities/${communityId}/members`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      Authorization: `Bearer ${token}`,
    },
    body: JSON.stringify({ user_id: userId }),
  });

  if (!resp.ok) throw new Error(`join community failed (${resp.status})`);
}

/** Send a channel message via REST API. */
export async function sendChannelMessage(token: string, channelId: string, content: string) {
  const resp = await fetch(`${API_BASE}/api/v1/channels/${channelId}/messages`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      Authorization: `Bearer ${token}`,
    },
    body: JSON.stringify({ content }),
  });

  if (!resp.ok) throw new Error(`send channel message failed (${resp.status})`);
  return resp.json();
}

// =============================================================================
// Browser helpers — inject auth state
// =============================================================================

/** Inject auth tokens into localStorage and navigate to the app. */
export async function injectAuthAndNavigate(page: Page, user: TestUser) {
  // Navigate to the app first to set localStorage on the correct origin.
  await page.goto('/login');

  await page.evaluate(
    ({ accessToken, refreshToken }) => {
      localStorage.setItem('constell_access_token', accessToken);
      localStorage.setItem('constell_refresh_token', refreshToken);
    },
    { accessToken: user.accessToken, refreshToken: user.refreshToken },
  );

  // Navigate to the main page — AuthGuard should pick up the tokens.
  await page.goto('/@me');
}

/** Clear auth tokens from localStorage. */
export async function clearAuth(page: Page) {
  await page.evaluate(() => {
    localStorage.removeItem('constell_access_token');
    localStorage.removeItem('constell_refresh_token');
  });
}

// =============================================================================
// Extended test fixture with pre-created user
// =============================================================================

type E2EFixture = {
  user: TestUser;
  userPage: Page;
};

export const test = base.extend<E2EFixture>({
  user: async ({}, use) => {
    const user = await registerTestUser();
    await use(user);
  },
  userPage: async ({ page, user }, use) => {
    await injectAuthAndNavigate(page, user);
    await use(page);
  },
});

export { expect };
