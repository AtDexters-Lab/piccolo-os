#!/usr/bin/env node
import { chromium } from '@playwright/test';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { mkdir } from 'node:fs/promises';

const wait = (ms) => new Promise((resolve) => setTimeout(resolve, ms));
const __dirname = path.dirname(fileURLToPath(import.meta.url));
const projectRoot = path.resolve(__dirname, '..');
const screenshotsRoot = path.join(projectRoot, 'screenshots');

const tagArg = process.argv.slice(2).find((arg) => arg.startsWith('--tag='));
const tag = tagArg ? tagArg.split('=')[1] : new Date().toISOString().replace(/[:.]/g, '-');
const outputDir = path.join(screenshotsRoot, tag);
const baseUrl = process.env.PICCOLO_BASE_URL ?? 'http://localhost:5173';

const flows = [
  { name: 'home', path: '/' },
  { name: 'setup', path: '/setup' },
  { name: 'install', path: '/install' }
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
  const page = await browser.newPage({ viewport: { width: 1440, height: 900 } });

  for (const [index, flow] of flows.entries()) {
    const url = new URL(flow.path, baseUrl).toString();
    console.log(`→ (${index + 1}/${flows.length}) ${flow.name}: ${url}`);
    await page.goto(url, { waitUntil: 'networkidle' });
    await page.waitForTimeout(500);
    const filename = `${String(index + 1).padStart(2, '0')}-${flow.name}.png`;
    await page.screenshot({ path: path.join(outputDir, filename), fullPage: true });
  }

  await browser.close();
  console.log('Screenshot capture complete.');
}

capture().catch((err) => {
  console.error('Screenshot capture failed:', err);
  process.exit(1);
});
