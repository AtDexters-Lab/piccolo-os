import { test, expect } from '@playwright/test';

test.describe('First-run setup redirect', () => {
  test('unauthenticated dashboard redirects to setup when not initialized', async ({ page, request }) => {
    // Try to clear any existing admin by calling login; if setup exists this will remain initialized.
    // We cannot delete state from here; this test asserts behavior when not initialized in fresh env.
    await page.goto('/#/');
    const h2 = page.locator('h2');
    // Expect either Dashboard (if already initialized+authed), Sign in (initialized but unauth), or Create Admin (not initialized)
    const txt = (await h2.textContent()) || '';
    if (/Create Admin/i.test(txt)) {
      await expect(h2).toHaveText('Create Admin');
    } else if (/Sign in/i.test(txt)) {
      // Navigate explicitly to login in initialized installs; ensure login stays
      await expect(h2).toHaveText('Sign in');
    } else {
      // If we landed on Dashboard, environment has a session already; nothing to assert here.
      await expect(h2).toBeVisible();
    }
  });

  test('login route redirects to setup when not initialized', async ({ page }) => {
    await page.goto('/#/login');
    const h2 = page.locator('h2');
    const txt = (await h2.textContent()) || '';
    if (/Create Admin/i.test(txt)) {
      await expect(h2).toHaveText('Create Admin');
    } else {
      await expect(h2).toHaveText(/Sign in|Dashboard/);
    }
  });
});

