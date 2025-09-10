import { test, expect } from '@playwright/test';

// Fail tests on any browser console error
test.beforeEach(async ({ page }) => {
  await page.route('**/favicon.ico', async (route) => {
    await route.fulfill({ status: 200, body: '' });
  });
  page.on('console', (msg) => {
    if (msg.type() === 'error') {
      const text = msg.text();
      if (/Failed to load resource: .* 404/.test(text)) return;
      throw new Error(`Console error: ${text}`);
    }
  });
});

test.describe('Install (demo)', () => {
  test('fetch latest ok and verify_failed show toasts', async ({ page }) => {
    await page.goto('/#/install');
    await page.getByRole('button', { name: 'Fetch latest' }).click();
    await expect(page.getByText(/Fetched .* \(verified\)/)).toBeVisible();
    await page.getByRole('button', { name: 'Simulate verify failed' }).click();
    await expect(page.getByText(/verification failed/i)).toBeVisible();
  });

  test('run install requires typing disk id', async ({ page }) => {
    await page.goto('/#/install');
    // Simulate plan for first target
    const firstSim = page.getByRole('button', { name: 'Simulate' }).first();
    await firstSim.click();
    // The plan block should appear and Run install should be disabled until confirm matches
    const confirmInput = page.getByLabel('Type the target id to confirm install');
    // Grab the selected ID from the plan JSON
    const idText = await page.locator('pre').textContent();
    const match = idText?.match(/\"target\":\s*\"([^\"]+)\"/);
    const targetId = match?.[1] || '';
    const runBtn = page.getByRole('button', { name: 'Run install' });
    await expect(runBtn).toBeDisabled();
    await confirmInput.fill('/wrong/id');
    await expect(runBtn).toBeDisabled();
    await confirmInput.fill(targetId);
    await expect(runBtn).toBeEnabled();
    await runBtn.click();
    await expect(page.getByText(/Installation started; device will reboot on completion|Installation started/)).toBeVisible();
  });

  test('post-install banner appears on dashboard after starting install', async ({ page }) => {
    await page.goto('/#/install');
    const firstSim = page.getByRole('button', { name: 'Simulate' }).first();
    await firstSim.click();
    const idText = await page.locator('pre').textContent();
    const match = idText?.match(/\"target\":\s*\"([^\"]+)\"/);
    const targetId = match?.[1] || '';
    const runBtn = page.getByRole('button', { name: 'Run install' });
    await page.getByLabel('Type the target id to confirm install').fill(targetId);
    await runBtn.click();
    await page.goto('/#/');
    await expect(page.locator('h2')).toHaveText('Dashboard');
    await expect(page.getByText('Installation in progress')).toBeVisible();
    // Dismiss and ensure it hides
    await page.getByRole('button', { name: 'Dismiss' }).click();
    await expect(page.getByText('Installation in progress')).toBeHidden();
  });
});
