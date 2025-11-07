import { test, expect } from '@playwright/test';
import { ADMIN_PASSWORD, ensureSignedIn } from './support/session';

// Visual tour: visit key routes and capture full-page screenshots.
// Produces images under web-src/test-results/<test-name>-<project>/

test.describe('Visual tour (demo)', () => {
  test('capture screenshots of key pages', async ({ page }) => {
    await ensureSignedIn(page, ADMIN_PASSWORD);
    const shots: Array<{ url: string; waitFor: string; name: string }> = [
      { url: '/', waitFor: 'h2:text("Keep Piccolo on track")', name: '00_home' },
      { url: '/#/apps', waitFor: 'h2:text("Apps")', name: '10_apps' },
      { url: '/#/storage', waitFor: 'p:text("Storage status")', name: '20_storage' },
      { url: '/#/updates', waitFor: 'h1:text("Updates")', name: '30_updates' },
      { url: '/#/remote', waitFor: 'h1:text("Remote access")', name: '40_remote' },
    ];

    for (const s of shots) {
      await page.goto(s.url);
      await page.locator(s.waitFor).first().waitFor({ state: 'visible' });
      // Give panels a brief moment to render async data in demo mode
      await page.waitForTimeout(200);
      const out = test.info().outputPath(`${s.name}.png`);
      await page.screenshot({ path: out, fullPage: true });
    }

    // Capture the remote setup wizard overlay for design review
    await page.evaluate(() => window.dispatchEvent(new CustomEvent('remote-wizard-open')));
    await page.locator('#remote-wizard-title').waitFor({ state: 'visible' });
    await page.waitForTimeout(200);
    const wizardOut = test.info().outputPath('45_remote_wizard.png');
    await page.screenshot({ path: wizardOut, fullPage: true });
    await page.keyboard.press('Escape');
  });
});
