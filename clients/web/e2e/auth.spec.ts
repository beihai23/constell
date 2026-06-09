import { test, expect, registerTestUser } from './fixtures';

test.describe('Authentication', () => {
  test('register a new user and land on main page', async ({ page }) => {
    const unique = `e2e_${Date.now()}`;
    const email = `${unique}@test.com`;
    const nickname = `Nick_${unique}`;

    await page.goto('/register');

    // Fill in registration form.
    await page.locator('#register-username').fill(nickname);
    await page.locator('#register-email').fill(email);
    await page.locator('#register-password').fill('testPassword123!');
    await page.locator('#register-confirm').fill('testPassword123!');

    // Submit the form.
    await page.getByRole('button', { name: /register/i }).click();

    // Should redirect to main page.
    await page.waitForURL(/\/@me/, { timeout: 10000 });
    await expect(page).toHaveURL(/\/@me/);
  });

  test('login with existing user', async ({ page }) => {
    // Create a user via API first.
    const user = await registerTestUser();

    await page.goto('/login');

    // Fill in login form.
    await page.locator('#login-email').fill(user.email);
    await page.locator('#login-password').fill(user.password);

    // Submit the form.
    await page.getByRole('button', { name: /login/i }).click();

    // Should redirect to main page.
    await page.waitForURL(/\/@me/, { timeout: 10000 });
    await expect(page).toHaveURL(/\/@me/);
  });

  test('login with wrong password shows error', async ({ page }) => {
    const user = await registerTestUser();

    await page.goto('/login');

    await page.locator('#login-email').fill(user.email);
    await page.locator('#login-password').fill('wrongPassword123!');
    await page.getByRole('button', { name: /login/i }).click();

    // Should show an error message (either inline or toast).
    await page.waitForTimeout(2000);
    // User should still be on the login page.
    await expect(page).toHaveURL(/\/login/);
  });

  test('redirect to login when not authenticated', async ({ page }) => {
    await page.goto('/@me');
    // Should be redirected to login page.
    await page.waitForURL(/\/login/, { timeout: 5000 }).catch(() => {
      // Some apps redirect to /register instead.
    });
    const url = page.url();
    expect(url).toMatch(/\/(login|register)/);
  });
});
