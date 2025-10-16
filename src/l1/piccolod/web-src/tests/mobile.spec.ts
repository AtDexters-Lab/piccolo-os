import { test, expect } from '@playwright/test';
import { ADMIN_PASSWORD, ensureSignedIn } from './support/session';

// Run on the 'mobile-chromium' project (Pixel 5) from config
test.describe('Mobile layout', () => {
  test('no horizontal scroll and nav toggle works + app action', async ({ page }) => {
    if (test.info().project.name !== 'mobile-chromium') test.skip();
    await ensureSignedIn(page, ADMIN_PASSWORD);
    await page.goto('/');
    // Assert no horizontal scroll
    const noHScroll = await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth);
    expect(noHScroll).toBeTruthy();

    // Open the menu and ensure links are present
    const menuBtn = page.getByRole('button', { name: 'Menu' });
    await menuBtn.click();
    await expect(menuBtn).toHaveAttribute('aria-expanded', 'true');
    await expect(page.locator('#main-nav')).toBeVisible();
    await page.goto('/#/apps');
    await expect(page.getByRole('heading', { name: 'Apps' })).toBeVisible();

    // Perform a Start action on first card
    const startBtn = page.getByRole('button', { name: 'Start' }).first();
    await startBtn.click();
    await expect(page.getByText('Started', { exact: false }).last()).toBeVisible();
  });

  test('visual tour (mobile screenshots)', async ({ page }) => {
    if (test.info().project.name !== 'mobile-chromium') test.skip();
    await ensureSignedIn(page, ADMIN_PASSWORD);
    const shots: Array<{ url: string; waitFor: string; name: string }> = [
      { url: '/', waitFor: 'h2:text("Dashboard")', name: 'm00_dashboard' },
      { url: '/#/apps', waitFor: 'h2:text("Apps")', name: 'm10_apps' },
      { url: '/#/storage', waitFor: 'h2:text("Storage")', name: 'm20_storage' },
      { url: '/#/updates', waitFor: 'h2:text("Updates")', name: 'm30_updates' },
      { url: '/#/remote', waitFor: 'h2:text("Remote")', name: 'm40_remote' },
    ];
    for (const s of shots) {
      await page.goto(s.url);
      await page.locator(s.waitFor).first().waitFor({ state: 'visible' });
      const out = test.info().outputPath(`${s.name}.png`);
      await page.screenshot({ path: out, fullPage: true });
    }
  });
  test('menu button is clearly tappable on mobile', async ({ page }) => {
    if (test.info().project.name !== 'mobile-chromium') test.skip();
    await ensureSignedIn(page, ADMIN_PASSWORD);
    await page.goto('/');
    const menuBtn = page.getByRole('button', { name: 'Menu' });
    await expect(menuBtn).toBeVisible();
    const box = await menuBtn.boundingBox();
    expect(box).not.toBeNull();
    // Assert touch target height >= 44px per mobile guidelines
    expect((box!.height)).toBeGreaterThanOrEqual(44);
    // Optional: cursor pointer indicates interactivity (may be no-op on mobile but should be set)
    const cursor = await menuBtn.evaluate((el) => getComputedStyle(el as HTMLElement).cursor);
    expect(cursor === 'pointer' || cursor === '').toBeTruthy();
    // Toggle visibility on and off
    await menuBtn.click();
    await expect(page.locator('#main-nav')).toBeVisible();
    await menuBtn.click();
    await expect(page.locator('#main-nav')).toBeHidden();
  });
});
