#!/usr/bin/env node

import { mkdir, readdir, rm, stat, copyFile } from 'node:fs/promises';
import path from 'node:path';
import process from 'node:process';

async function findLatestDir(baseDir, predicate) {
  const entries = await readdir(baseDir, { withFileTypes: true });
  const candidates = [];
  for (const entry of entries) {
    if (!entry.isDirectory()) continue;
    if (!predicate(entry.name)) continue;
    const fullPath = path.join(baseDir, entry.name);
    const info = await stat(fullPath);
    candidates.push({ fullPath, mtimeMs: info.mtimeMs });
  }
  if (!candidates.length) {
    return null;
  }
  candidates.sort((a, b) => b.mtimeMs - a.mtimeMs);
  return candidates[0].fullPath;
}

async function clearPngs(dir) {
  await mkdir(dir, { recursive: true });
  const entries = await readdir(dir);
  await Promise.all(
    entries
      .filter((file) => file.endsWith('.png'))
      .map((file) => rm(path.join(dir, file), { force: true }))
  );
}

async function copyPngs(srcDir, destDir) {
  const entries = await readdir(srcDir);
  const pngs = entries.filter((file) => file.endsWith('.png'));
  if (!pngs.length) {
    throw new Error(`No PNG files found in ${srcDir}`);
  }
  await mkdir(destDir, { recursive: true });
  for (const file of pngs) {
    const src = path.join(srcDir, file);
    const dest = path.join(destDir, file);
    await copyFile(src, dest);
    console.log(`Copied ${file} -> ${path.relative(process.cwd(), dest)}`);
  }
}

async function main() {
  const webSrcRoot = path.resolve(path.dirname(new URL(import.meta.url).pathname), '..');
  const testResultsDir = path.join(webSrcRoot, 'test-results');
  const reviewsRoot = path.resolve(webSrcRoot, '..', '..', 'reviews', 'screenshots');
  const desktopDest = path.join(reviewsRoot, 'desktop');
  const mobileDest = path.join(reviewsRoot, 'mobile');

  const desktopSrc = await findLatestDir(
    testResultsDir,
    (name) => name.startsWith('visual-') && name.endsWith('-chromium')
  );
  if (!desktopSrc) {
    throw new Error('Could not locate desktop visual tour output in test-results/. Run the Playwright visual tour first.');
  }

  const mobileSrc = await findLatestDir(
    testResultsDir,
    (name) => name.startsWith('mobile-') && name.endsWith('-mobile-chromium')
  );
  if (!mobileSrc) {
    throw new Error('Could not locate mobile visual tour output in test-results/. Run the Playwright mobile tour first.');
  }

  await clearPngs(desktopDest);
  await clearPngs(mobileDest);
  await copyPngs(desktopSrc, desktopDest);
  await copyPngs(mobileSrc, mobileDest);

  console.log('\nReview screenshot folders updated.');
}

main().catch((err) => {
  console.error(err.message || err);
  process.exitCode = 1;
});

