import { defineConfig, devices } from '@playwright/test';
import path from 'node:path';

export default defineConfig({
  testDir: './tests',
  use: {
    baseURL: process.env.BASE_URL || 'http://localhost:8080',
    headless: true,
    viewport: { width: 1280, height: 800 },
  },
  webServer: {
    command: 'PORT=8080 PICCOLO_DEMO=1 ./piccolod',
    port: 8080,
    timeout: 120000,
    reuseExistingServer: true,
    cwd: path.resolve(__dirname, '..'),
  },
  projects: [
    { name: 'chromium', use: { ...devices['Desktop Chrome'] } },
  ],
});

