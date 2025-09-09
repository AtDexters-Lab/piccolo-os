import { test, expect } from '@playwright/test';

// Run on the 'mobile-chromium' project (Pixel 5) from config
test.describe('Mobile layout', () => {
  test('no horizontal scroll and nav toggle works + app action', async ({ page }) => {
    if (test.info().project.name !== 'mobile-chromium') test.skip();
    await page.goto('/');
    // Assert no horizontal scroll
    const noHScroll = await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth);
    expect(noHScroll).toBeTruthy();

    // Open the menu and ensure links are present
    const menuBtn = page.getByRole('button', { name: 'Menu' });
    await menuBtn.click();
    await expect(menuBtn).toHaveAttribute('aria-expanded', 'true');
    await page.goto('/#/apps');
    await expect(page.getByRole('heading', { name: 'Apps' })).toBeVisible();

    // Perform a Start action on first card
    const startBtn = page.getByRole('button', { name: 'Start' }).first();
    await startBtn.click();
    await expect(page.getByText('Started', { exact: false }).last()).toBeVisible();
  });
});
