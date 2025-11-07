import { test, expect } from '@playwright/test';
import { ADMIN_PASSWORD, ensureSignedIn } from './support/session';

// Run on the 'mobile-chromium' project (Pixel 5) from config
test.describe('Mobile layout', () => {
  test('no horizontal scroll and bottom nav works', async ({ page }) => {
    await ensureSignedIn(page, ADMIN_PASSWORD);
    await page.goto('/');
    const bottomNav = page.locator('nav.app-shell__bottom-nav');
    await expect(bottomNav).toBeVisible();
    // Assert no horizontal scroll
    const noHScroll = await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth);
    expect(noHScroll).toBeTruthy();

    // Use bottom nav to reach Apps
    const appsBtn = page.locator('nav.app-shell__bottom-nav').getByRole('button', { name: 'Apps' });
    await appsBtn.click({ force: true });
    await expect(page.getByRole('heading', { level: 2, name: 'Apps' })).toBeVisible();

    // Navigate back home
    const homeBtn = page.locator('nav.app-shell__bottom-nav').getByRole('button', { name: 'Home' });
    await homeBtn.click({ force: true });
    await expect(page.getByRole('heading', { level: 2, name: 'Keep Piccolo on track' })).toBeVisible();
  });

  test('visual tour (mobile screenshots)', async ({ page }) => {
    await ensureSignedIn(page, ADMIN_PASSWORD);
    const shots: Array<{ url: string; waitFor: string; name: string }> = [
      { url: '/', waitFor: 'h2:text("Keep Piccolo on track")', name: 'm00_home' },
      { url: '/#/apps', waitFor: 'h2:text("Apps")', name: 'm10_apps' },
      { url: '/#/storage', waitFor: 'p:text("Storage status")', name: 'm20_storage' },
      { url: '/#/updates', waitFor: 'h1:text("Updates")', name: 'm30_updates' },
      { url: '/#/remote', waitFor: 'h1:text("Remote access")', name: 'm40_remote' },
    ];
    for (const s of shots) {
      await page.goto(s.url);
      await page.locator(s.waitFor).first().waitFor({ state: 'visible' });
      const out = test.info().outputPath(`${s.name}.png`);
      await page.screenshot({ path: out, fullPage: true });
    }
    await page.evaluate(() => window.dispatchEvent(new CustomEvent('remote-wizard-open')));
    await page.locator('#remote-wizard-title').waitFor({ state: 'visible' });
    const wizardOut = test.info().outputPath('m45_remote_wizard.png');
    await page.screenshot({ path: wizardOut, fullPage: true });
    await page.keyboard.press('Escape');
  });
  test('quick settings control is clearly tappable on mobile', async ({ page }) => {
    await ensureSignedIn(page, ADMIN_PASSWORD);
    await page.goto('/');
    const bottomNav = page.locator('nav.app-shell__bottom-nav');
    await expect(bottomNav).toBeVisible();
    const qsBtn = page.getByRole('button', { name: 'Quick settings' });
    await expect(qsBtn).toBeVisible();
    const box = await qsBtn.boundingBox();
    expect(box).not.toBeNull();
    // Assert touch target height >= 44px per mobile guidelines
    expect((box!.height)).toBeGreaterThanOrEqual(44);
    // Optional: cursor pointer indicates interactivity (may be no-op on mobile but should be set)
    const cursor = await qsBtn.evaluate((el) => getComputedStyle(el as HTMLElement).cursor);
    expect(cursor === 'pointer' || cursor === '').toBeTruthy();
    // Toggle quick settings drawer
    await qsBtn.click();
    const dialog = page.locator('[data-quick-settings-modal]');
    await expect(dialog).toBeVisible();
    await page.keyboard.press('Escape');
    await expect(dialog).toBeHidden({ timeout: 1000 });
  });

  test('logout control is reachable on mobile', async ({ page }) => {
    await ensureSignedIn(page, ADMIN_PASSWORD);
    await page.goto('/');

    const logoutBtn = page.locator('header').getByRole('button', { name: 'Logout' });
    await expect(logoutBtn).toBeVisible({ timeout: 15000 });
    await logoutBtn.click({ force: true });
    await page.waitForURL('**/#/login', { timeout: 15000 });
    await expect(page.getByRole('heading', { name: /Sign in|Piccolo Home is locked/i })).toBeVisible();
  });
});
