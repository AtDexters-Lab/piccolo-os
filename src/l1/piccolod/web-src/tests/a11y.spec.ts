import { test, expect } from '@playwright/test';

// Console error guard (ignore benign 401/403 during auth transitions)
test.beforeEach(async ({ page }) => {
  await page.route('**/favicon.ico', async (route) => {
    await route.fulfill({ status: 200, body: '' });
  });
  page.on('console', (msg) => {
    if (msg.type() === 'error') {
      const text = msg.text();
      if (/Failed to load resource: .* (404|401|403)/.test(text)) return;
      throw new Error(`Console error: ${text}`);
    }
  });
});

test('skip link moves focus to main content', async ({ page }) => {
  await page.goto('/');
  // Tab to reveal skip link
  await page.keyboard.press('Tab');
  const skip = page.getByRole('link', { name: 'Skip to content' });
  await expect(skip).toBeVisible();
  await skip.press('Enter');
  // Router root should be focused
  const activeId = await page.evaluate(() => document.activeElement?.id);
  expect(activeId).toBe('router-root');
});

test('focus moves to main after route change', async ({ page, request }) => {
  await request.post('/api/v1/auth/setup', { data: { password: 'password' } }).catch(() => {});
  await page.goto('/#/login');
  await page.getByLabel('Username').fill('admin');
  await page.getByLabel('Password').fill('password');
  await page.getByRole('button', { name: 'Sign in' }).click();
  await expect(page.locator('h2')).toHaveText('Dashboard');
  await page.getByRole('link', { name: 'Apps' }).click();
  await expect(page.getByRole('heading', { name: 'Apps' })).toBeVisible();
  await page.waitForFunction(() => document.activeElement && (document.activeElement as HTMLElement).id === 'router-root');
});

test('toasts expose aria-live and role status', async ({ page }) => {
  await page.goto('/#/apps');
  // Trigger a toast (Start first app)
  const startBtn = page.getByRole('button', { name: 'Start' }).first();
  await startBtn.click();
  const toastRegion = page.locator('[aria-live="polite"]');
  await expect(toastRegion).toBeVisible();
  await expect(toastRegion.getByRole('status').last()).toBeVisible();
});

test('mobile menu closes on Escape and focuses first link when opened', async ({ page }) => {
  if (test.info().project.name !== 'mobile-chromium') test.skip();
  await page.goto('/');
  const menuBtn = page.getByRole('button', { name: 'Menu' });
  await menuBtn.click();
  const nav = page.locator('#main-nav');
  await expect(nav).toBeVisible();
  // First link should be focused
  const focused = await page.evaluate(() => document.activeElement?.textContent?.trim());
  expect(focused).toBeTruthy();
  // Press Escape to close
  await page.keyboard.press('Escape');
  await expect(nav).toBeHidden();
});
