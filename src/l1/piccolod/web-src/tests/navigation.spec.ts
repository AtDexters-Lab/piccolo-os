import { test, expect } from '@playwright/test';

// Fail tests on any browser console error
test.beforeEach(async ({ page }) => {
  // Avoid favicon 404 noise
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

test.describe('Top-level navigation and deep links', () => {
  test('home loads without redirects and assets are reachable', async ({ page, request }) => {
    const resp = await page.goto('/');
    expect(resp?.status()).toBe(200);
    await expect(page.locator('header img[alt="Piccolo"]')).toBeVisible();

    // Ensure main asset JS is reachable
    const scriptHref = await page.locator('script[type="module"][src^="/assets/"]').first().getAttribute('src');
    expect(scriptHref).toBeTruthy();
    const js = await request.get(scriptHref!);
    expect(js.ok()).toBeTruthy();

    // Branding logo is reachable
    const logo = await request.get('/branding/piccolo.svg');
    expect(logo.ok()).toBeTruthy();
  });

  test('navigate via nav: Apps/Storage/Updates/Remote', async ({ page }) => {
    await page.goto('/');
    await page.getByRole('link', { name: 'Apps' }).click();
    await expect(page.locator('h2')).toHaveText('Apps');

    await page.getByRole('link', { name: 'Storage' }).click();
    await expect(page.locator('h2')).toHaveText('Storage');

    await page.getByRole('link', { name: 'Updates' }).click();
    await expect(page.locator('h2')).toHaveText('Updates');

    await page.getByRole('link', { name: 'Remote' }).click();
    await expect(page.locator('h2')).toHaveText('Remote');

    // Additional BFS routes validated separately
  });

  test('deep-link directly to /#/apps', async ({ page }) => {
    const resp = await page.goto('/#/apps');
    expect(resp?.status()).toBe(200);
    await expect(page.locator('h2')).toHaveText('Apps');
  });

  test('deep-link to app details then navigate via nav and back', async ({ page }) => {
    await page.goto('/#/apps/vaultwarden');
    await expect(page.locator('h2')).toHaveText(/App: vaultwarden/);
    // Navigate to Apps via nav
    await page.getByRole('link', { name: 'Apps' }).click();
    await expect(page.locator('h2')).toHaveText('Apps');
    // Go back to details via browser back
    await page.goBack();
    await expect(page.locator('h2')).toHaveText(/App: vaultwarden/);
  });

  test('deep-link to second app (gitea) then navigate via nav and back', async ({ page }) => {
    await page.goto('/#/apps/gitea');
    await expect(page.locator('h2')).toHaveText(/App: gitea/);
    await page.getByRole('link', { name: 'Apps' }).click();
    await expect(page.locator('h2')).toHaveText('Apps');
    await page.goBack();
    await expect(page.locator('h2')).toHaveText(/App: gitea/);
  });
});

test('apps actions show toasts (demo)', async ({ page }) => {
  if (test.info().project.name === 'mobile-chromium') test.skip();
  await page.goto('/#/apps');
  const startBtn = page.getByRole('button', { name: 'Start' }).first();
  await startBtn.click();
  await expect(page.getByText('Started', { exact: false }).last()).toBeVisible();

  // Navigate to details and perform update (toast check)
  await page.getByRole('link', { name: /vaultwarden/i }).click();
  await expect(page.locator('h2')).toHaveText(/App: vaultwarden/);
  await page.getByRole('button', { name: 'Update' }).click();
  await expect(page.getByText('Updated', { exact: false }).last()).toBeVisible();

  // Logs bundle link visible
  await expect(page.getByRole('link', { name: 'Download logs bundle' })).toBeVisible();

  // Uninstall flow with purge option
  await page.getByRole('button', { name: 'Uninstall' }).click();
  await expect(page.getByText('Confirm uninstall')).toBeVisible();
  const purge = page.getByRole('checkbox', { name: /Delete data too/i });
  await purge.check();
  await page.getByRole('button', { name: 'Confirm uninstall' }).click();
  await expect(page.getByText(/Uninstalled/i)).toBeVisible();
});

test('updates OS apply shows toast (demo)', async ({ page }) => {
  await page.goto('/#/updates');
  const applyBtn = page.getByRole('button', { name: 'Apply' });
  await applyBtn.click();
  await expect(page.getByText('OS update applied')).toBeVisible();
});

test('remote simulate DNS error shows error toast (demo)', async ({ page }) => {
  await page.goto('/#/remote');
  const btn = page.getByRole('button', { name: 'Simulate DNS error' });
  await btn.click();
  await expect(page.getByText(/Configure failed|DNS/i)).toBeVisible();
});

test('remote enable and disable show toasts (demo)', async ({ page }) => {
  await page.goto('/#/remote');
  // Enable
  await page.getByRole('button', { name: 'Enable Remote' }).click();
  await expect(page.getByText('Remote configured')).toBeVisible();
  // Disable
  await page.getByRole('button', { name: 'Disable' }).click();
  await expect(page.getByText('Remote disabled')).toBeVisible();
});

test('storage action shows toast (demo)', async ({ page }) => {
  await page.goto('/#/storage');
  // Prefer non-destructive action: Set default if available, else Use as-is
  const setDefault = page.getByRole('button', { name: 'Set default' }).first();
  if (await setDefault.count() > 0) {
    await setDefault.click();
    await expect(page.getByText('Default data root updated')).toBeVisible();
  } else {
    const useBtn = page.getByRole('button', { name: 'Use as-is' }).first();
    await useBtn.click();
    await expect(page.getByText('Using', { exact: false })).toBeVisible();
  }
});

test('storage unlock and recovery key flows (demo)', async ({ page }) => {
  await page.goto('/#/storage');
  // Unlock success
  const unlockBtn = page.getByRole('button', { name: 'Unlock volumes' }).first();
  await unlockBtn.click();
  await expect(page.getByText('Volumes unlocked')).toBeVisible();

  // Simulate unlock failure (demo-only)
  const failBtn = page.getByRole('button', { name: 'Simulate unlock failure' }).first();
  if (await failBtn.count() > 0) {
    await failBtn.click();
    await expect(page.getByText(/Unlock failed|failed/i)).toBeVisible();
  }

  // Generate recovery key (if not present) or re-generate in demo
  const genBtn = page.getByRole('button', { name: 'Generate Recovery Key' }).first();
  if (await genBtn.count() > 0) {
    await genBtn.click();
    await expect(page.getByText('Recovery key generated')).toBeVisible();
  }
});

test('storage encrypt-in-place dry-run, confirm, and failure (demo)', async ({ page }) => {
  await page.goto('/#/storage');
  const input = page.getByPlaceholder('/var/piccolo/storage/app/data');
  await input.fill('/tmp/piccolo-demo');
  await page.getByRole('button', { name: 'Dry-run' }).click();
  await expect(page.getByText('Dry run plan generated')).toBeVisible();
  await page.getByRole('button', { name: 'Confirm' }).click();
  await expect(page.getByText('Encryption completed')).toBeVisible();
  const failBtn = page.getByRole('button', { name: 'Simulate failure' });
  if (await failBtn.count() > 0) {
    await failBtn.click();
    await expect(page.getByText(/Encryption failed|failed/i)).toBeVisible();
  }
});
