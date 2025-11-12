import { expect, test } from '@playwright/test';

const setupPath = process.env.PICCOLO_SETUP_PATH ?? '/setup';

test('setup wizard renders hero, stepper, and primary CTA', async ({ page }) => {
  await page.goto(setupPath, { waitUntil: 'domcontentloaded' });

  await expect(page.getByRole('heading', { name: /Create admin credentials/i })).toBeVisible();
  await expect(page.getByText(/First run/i)).toBeVisible();
  await expect(page.getByRole('button', { name: /(Start setup|Sign in)/i })).toBeVisible();
});

test.describe('first-run setup completion', () => {
  test.skip(({ isMobile }) => !!isMobile, 'Run destructive setup flow on desktop chromium only');

  const password = process.env.PICCOLO_E2E_PASSWORD ?? 'Supersafe123!';

  test('setup wizard completes first-run flow', async ({ page }) => {
    await page.goto(setupPath, { waitUntil: 'networkidle' });

    const startButton = page.getByRole('button', { name: /Start setup/i });
    if (await startButton.isVisible()) {
      await startButton.click();
    }

    // If setup already finished (e.g., rerun), just assert completion UI.
    if (await page.getByRole('heading', { name: /Admin ready/i }).isVisible().catch(() => false)) {
      await expect(page.getByRole('button', { name: /Go to dashboard/i })).toBeVisible();
      return;
    }

    await page.getByLabel(/Create password/i).fill(password);
    await page.getByLabel(/Confirm password/i).fill(password);
    await page.getByRole('button', { name: /Create admin/i }).click();

    await expect(page.getByRole('heading', { name: /Admin ready/i })).toBeVisible();
    await expect(page.getByRole('button', { name: /Go to dashboard/i })).toBeVisible();
  });
});
