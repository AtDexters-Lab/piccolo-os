import { test, expect } from '@playwright/test';

test.describe('Auth flows (demo)', () => {
  test('login success redirects to dashboard and sets session', async ({ page }) => {
    await page.goto('/#/login');
    await page.getByLabel('Username').fill('admin');
    await page.getByLabel('Password').fill('password');
    await page.getByRole('button', { name: 'Sign in' }).click();
    await expect(page.locator('h2')).toHaveText('Dashboard');
    await expect(page.getByText('Signed in')).toBeVisible();
    await expect(page.getByText(/"authenticated":true/)).toBeVisible();
  });

  test('login invalid credentials shows error (401 demo)', async ({ page }) => {
    await page.goto('/#/login');
    await page.getByRole('button', { name: 'Simulate 401' }).click();
    await expect(page.getByText(/Sign in failed|invalid|unauthorized/i)).toBeVisible();
  });

  test('login rate limited shows message (429 demo)', async ({ page }) => {
    await page.goto('/#/login');
    await page.getByRole('button', { name: 'Simulate 429' }).click();
    await expect(page.getByText(/try again|rate|too many/i)).toBeVisible();
  });

  test('first-run setup creates admin and redirects', async ({ page }) => {
    await page.goto('/#/setup');
    await page.getByLabel('Password', { exact: true }).fill('supersecret');
    await page.getByLabel('Confirm password').fill('supersecret');
    await page.getByRole('button', { name: 'Create Admin' }).click();
    await expect(page.locator('h2')).toHaveText('Dashboard');
    await expect(page.getByText(/"authenticated":true/)).toBeVisible();
  });

  test('session timeout shows banner and redirects to login', async ({ page }) => {
    await page.goto('/#/apps');
    await page.evaluate(() => {
      window.dispatchEvent(new Event('piccolo-session-expired'));
    });
    await expect(page.locator('h2')).toHaveText('Sign in');
    await expect(page.getByText('Session expired')).toBeVisible();
    // Click Sign in button on banner should keep us on login route
    await page.getByRole('alert').getByRole('button', { name: 'Sign in' }).click();
    await expect(page.locator('h2')).toHaveText('Sign in');
  });
});
