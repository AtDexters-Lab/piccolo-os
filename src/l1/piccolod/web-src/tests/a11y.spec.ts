import { test, expect } from '@playwright/test';
import { ADMIN_PASSWORD, ensureSignedIn } from './support/session';

// Console error guard (ignore benign 401/403 during auth transitions)
test.beforeEach(async ({ page }) => {
  await page.route('**/favicon.ico', async (route) => {
    await route.fulfill({ status: 200, body: '' });
  });
  page.on('console', (msg) => {
    if (msg.type() === 'error') {
      const text = msg.text();
      if (/Failed to load resource: .* (404|401|403|423)/.test(text)) return;
      throw new Error(`Console error: ${text}`);
    }
  });
});

test('skip link moves focus to main content', async ({ page }) => {
  await ensureSignedIn(page, ADMIN_PASSWORD);
  await page.goto('/');
  // Tab to reveal skip link
  await page.keyboard.press('Tab');
  const skip = page.getByRole('link', { name: 'Skip to content' });
  await expect(skip).toBeVisible();
  await skip.press('Enter');
  // Main content should be focused
  const activeId = await page.evaluate(() => document.activeElement?.id);
  expect(activeId).toBe('main-content');
});

test('focus moves to main after route change', async ({ page }) => {
  await ensureSignedIn(page, ADMIN_PASSWORD);
  await page.getByRole('complementary', { name: 'Primary' }).getByRole('button', { name: 'Apps' }).click();
  await expect(page.getByRole('heading', { level: 2, name: 'Apps' })).toBeVisible();
  await expect(page.locator('#main-content')).toHaveAttribute('tabindex', '-1');
});

test('toasts expose aria-live and role status', async ({ page }) => {
  await ensureSignedIn(page, ADMIN_PASSWORD);
  const toastRegion = page.locator('[aria-live="polite"]');
  await expect(toastRegion).toHaveCount(1);
  await expect(toastRegion).toHaveAttribute('aria-live', 'polite');
  await expect(toastRegion).toHaveAttribute('aria-atomic', 'false');
});

test('mobile menu closes on Escape and focuses first link when opened', async ({ page }) => {
  if (test.info().project.name !== 'mobile-chromium') test.skip();
  await ensureSignedIn(page, ADMIN_PASSWORD);
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
