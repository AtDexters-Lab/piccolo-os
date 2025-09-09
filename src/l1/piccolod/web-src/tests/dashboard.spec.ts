import { test, expect } from '@playwright/test';

test('dashboard loads and shows header', async ({ page }) => {
  await page.goto('/');
  await expect(page).toHaveTitle(/Piccolo OS/);
  await expect(page.locator('header h1')).toHaveText('Piccolo OS');
  await expect(page.locator('h2')).toContainText('Dashboard');
});

test('services API responds (demo)', async ({ request }) => {
  const res = await request.get('/api/v1/demo/services');
  expect(res.ok()).toBeTruthy();
  const json = await res.json();
  expect(json).toHaveProperty('services');
});

