import { test, expect } from '@playwright/test';

// Fail tests on any browser console error
test.beforeEach(async ({ page }) => {
  page.on('console', (msg) => {
    if (msg.type() === 'error') {
      const text = msg.text();
      // Ignore benign 404s like favicon or demo endpoints not material to this test
      if (/Failed to load resource: .* 404/.test(text)) return;
      throw new Error(`Console error: ${text}`);
    }
  });
});

test('dashboard loads and shows header logo', async ({ page }) => {
  await page.goto('/');
  await expect(page).toHaveTitle(/Piccolo/);
  // Logo is present and loads successfully
  const logo = page.locator('header img[alt="Piccolo"]');
  await expect(logo).toBeVisible();
  const natural = await logo.evaluate((img: HTMLImageElement) => ({ w: img.naturalWidth, h: img.naturalHeight }));
  expect(natural.w).toBeGreaterThan(0);
  await expect(page.locator('h2')).toContainText('Dashboard');
});

test('services API responds (demo)', async ({ request }) => {
  const res = await request.get('/api/v1/demo/services');
  expect(res.ok()).toBeTruthy();
  const json = await res.json();
  expect(json).toHaveProperty('services');
});
