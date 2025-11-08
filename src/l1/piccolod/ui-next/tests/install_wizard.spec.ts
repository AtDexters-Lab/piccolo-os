import { expect, test } from '@playwright/test';

const installPath = process.env.PICCOLO_INSTALL_PATH ?? '/install';

test('install wizard scaffold renders and advances to disk step', async ({ page }) => {
  await page.goto(installPath, { waitUntil: 'domcontentloaded' });

  await expect(page.getByTestId('install-wizard')).toBeVisible();
  await expect(page.getByRole('heading', { name: /New disk install wizard/i })).toBeVisible();

  await page.getByRole('button', { name: /Begin install/i }).click();

  await expect(page.getByRole('heading', { name: /Choose the installation target/i })).toBeVisible();
});
