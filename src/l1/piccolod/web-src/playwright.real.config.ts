import { defineConfig, devices } from '@playwright/test';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);

export default defineConfig({
  testDir: './tests',
  use: {
    baseURL: process.env.BASE_URL || 'http://localhost:8080',
    headless: true,
    viewport: { width: 1280, height: 800 },
    screenshot: 'only-on-failure',
    video: 'retain-on-failure',
    trace: 'retain-on-failure',
  },
  reporter: [
    ['list'],
    ['json', { outputFile: 'test-results/report.json' }],
    ['html', { open: 'never', outputFolder: 'playwright-report' }],
  ],
  webServer: {
    // Use a throwaway state dir for real e2e to avoid polluting host
    command: 'PORT=8080 PICCOLO_STATE_DIR=.e2e-state ./piccolod',
    port: 8080,
    timeout: 120000,
    reuseExistingServer: true,
    cwd: path.resolve(__dirname, '..'),
  },
  projects: [
    { name: 'chromium', testMatch: ['tests/**/*.spec.ts', '!tests/mobile.spec.ts'], use: { ...devices['Desktop Chrome'] } },
  ],
});

