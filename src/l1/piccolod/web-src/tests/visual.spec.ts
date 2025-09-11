import { test, expect } from '@playwright/test';

// Visual tour: visit key routes and capture full-page screenshots.
// Produces images under web-src/test-results/<test-name>-<project>/

test.describe('Visual tour (demo)', () => {
  test('capture screenshots of key pages', async ({ page }) => {
    const shots: Array<{ url: string; waitFor: string; name: string; after?: () => Promise<void> }> = [
      { url: '/', waitFor: 'h2:text("Dashboard")', name: '00_dashboard' },
      { url: '/#/apps', waitFor: 'h2:text("Apps")', name: '10_apps' },
      { url: '/#/apps/vaultwarden', waitFor: 'h2:text("App: vaultwarden")', name: '11_app_vaultwarden' },
      { url: '/#/apps/gitea', waitFor: 'h2:text("App: gitea")', name: '12_app_gitea' },
      { url: '/#/apps/catalog', waitFor: 'h2:text("App Catalog")', name: '13_catalog' },
      { url: '/#/storage', waitFor: 'h2:text("Storage")', name: '20_storage' },
      { url: '/#/updates', waitFor: 'h2:text("Updates")', name: '30_updates' },
      { url: '/#/remote', waitFor: 'h2:text("Remote")', name: '40_remote' },
      { url: '/#/backup', waitFor: 'h2:text("Backup")', name: '50_backup' },
      { url: '/#/install', waitFor: 'h2:text("Install")', name: '60_install' },
      { url: '/#/events', waitFor: 'h2:text("Events")', name: '70_events' },
    ];

    for (const s of shots) {
      await page.goto(s.url);
      await page.locator(s.waitFor).first().waitFor({ state: 'visible' });
      // Give panels a brief moment to render async data in demo mode
      await page.waitForTimeout(200);
      const out = test.info().outputPath(`${s.name}.png`);
      await page.screenshot({ path: out, fullPage: true });
    }
  });
});
