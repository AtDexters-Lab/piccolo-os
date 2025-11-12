#!/usr/bin/env node
import { chromium } from '@playwright/test';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { mkdir } from 'node:fs/promises';

const wait = (ms) => new Promise((resolve) => setTimeout(resolve, ms));
const __dirname = path.dirname(fileURLToPath(import.meta.url));
const projectRoot = path.resolve(__dirname, '..');
const screenshotsRoot = path.join(projectRoot, 'screenshots');

const cliArgs = process.argv.slice(2);
const tagArg = cliArgs.find((arg) => arg.startsWith('--tag='));
const themeArg = cliArgs.find((arg) => arg.startsWith('--theme='));
const tag = tagArg ? tagArg.split('=')[1] : new Date().toISOString().replace(/[:.]/g, '-');
const outputDir = path.join(screenshotsRoot, tag);
const baseUrl = process.env.PICCOLO_BASE_URL ?? 'http://localhost:5173';
const themeInput = themeArg ? themeArg.split('=')[1] : process.env.PICCOLO_SCREENSHOTS_THEME;

const validThemes = ['light', 'dark'];
const defaultThemes = ['light'];
const selectedThemes = (() => {
  if (!themeInput) return defaultThemes;
  const parsed = themeInput
    .split(',')
    .map((value) => value.trim().toLowerCase())
    .filter(Boolean);
  const unique = [...new Set(parsed)];
  const invalid = unique.filter((theme) => !validThemes.includes(theme));
  if (invalid.length) {
    console.warn(`Ignoring invalid theme(s): ${invalid.join(', ')}. Allowed themes: light,dark.`);
  }
  const filtered = unique.filter((theme) => validThemes.includes(theme));
  return filtered.length ? filtered : defaultThemes;
})();

const flows = [
  { name: 'home', path: '/' },
  { name: 'setup', path: '/setup' },
  {
    name: 'setup-start',
    path: '/setup',
    action: async (page) => {
      const startButton = page.getByRole('button', { name: /start setup/i });
      if (await startButton.isVisible().catch(() => false)) {
        await startButton.click();
        await page.locator('#admin-password').waitFor({ timeout: 3000 });
      }
    }
  },
  { name: 'install', path: '/install' },
  {
    name: 'install-begin',
    path: '/install',
    action: async (page) => {
      const beginButton = page.getByRole('button', { name: /begin install/i });
      if (await beginButton.isVisible().catch(() => false)) {
        await beginButton.click();
        await page.getByRole('heading', { name: /choose the installation target/i }).waitFor({ timeout: 5000 });
      }
    }
  }
];

async function ensureReachable(url) {
  for (let attempt = 0; attempt < 40; attempt++) {
    try {
      const res = await fetch(url, { method: 'GET' });
      if (res.ok) return;
    } catch {
      // ignore
    }
    await wait(500);
  }
  throw new Error(`UI server at ${url} is not reachable. Ensure piccolod/preview is running or set PICCOLO_BASE_URL.`);
}

async function capture() {
  await mkdir(outputDir, { recursive: true });
  console.log(`Saving screenshots to ${outputDir}`);
  console.log(`Using base URL ${baseUrl}`);

  await ensureReachable(`${baseUrl}/`);

  const browser = await chromium.launch({ headless: true });
  const schemes = selectedThemes;

  for (const scheme of schemes) {
    const context = await browser.newContext({ viewport: { width: 1440, height: 900 }, colorScheme: scheme });
    const page = await context.newPage();
    console.log(`Capturing ${scheme} theme`);

    for (const [index, flow] of flows.entries()) {
      const url = new URL(flow.path, baseUrl).toString();
      console.log(`→ (${index + 1}/${flows.length}) ${flow.name}: ${url}`);
      await page.goto(url, { waitUntil: 'networkidle' });
      await page.waitForTimeout(500);
      if (typeof flow.action === 'function') {
        await flow.action(page);
        await page.waitForTimeout(300);
      }
      const prefix = scheme === 'dark' ? 'dark-' : '';
      const filename = `${prefix}${String(index + 1).padStart(2, '0')}-${flow.name}.png`;
      await page.screenshot({ path: path.join(outputDir, filename), fullPage: true });
    }

    await context.close();
  }

  await browser.close();
  console.log('Screenshot capture complete.');
}

capture().catch((err) => {
  console.error('Screenshot capture failed:', err);
  process.exit(1);
});
