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
    // HTML report must not live inside the tests output folder to avoid clashes
    ['html', { open: 'never', outputFolder: 'playwright-report' }],
  ],
  webServer: {
    command: 'PORT=8080 PICCOLO_DEMO=1 ./piccolod',
    port: 8080,
    timeout: 120000,
    reuseExistingServer: true,
    cwd: path.resolve(__dirname, '..'),
  },
  projects: [
    { name: 'chromium', testMatch: ['tests/**/*.spec.ts', '!tests/mobile.spec.ts'], use: { ...devices['Desktop Chrome'] } },
    { name: 'mobile-chromium', testMatch: ['tests/mobile.spec.ts'], use: { ...devices['Pixel 5'] } },
  ],
});
