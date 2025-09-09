#!/usr/bin/env node
/*
 Summarize Playwright JSON report to a concise issues.md with failing tests,
 including pointers to screenshots/videos/traces for quick triage.
*/
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);
const reportPath = path.resolve(__dirname, '../test-results/report.json');
const outPath = path.resolve(__dirname, '../test-results/issues.md');

function loadReport() {
  if (!fs.existsSync(reportPath)) {
    console.error(`No report found at ${reportPath}`);
    process.exit(0);
  }
  const json = JSON.parse(fs.readFileSync(reportPath, 'utf8'));
  return json;
}

function* walkSuites(node, parents = []) {
  if (!node) return;
  const p = node.title ? [...parents, node.title] : parents;
  if (node.specs) {
    for (const spec of node.specs) {
      const titleParts = [...p, spec.title];
      for (const test of spec.tests || []) {
        yield { titleParts, test };
      }
    }
  }
  if (node.suites) {
    for (const s of node.suites) yield* walkSuites(s, p);
  }
}

function summarize() {
  const data = loadReport();
  const failures = [];
  for (const root of data.suites || []) {
    for (const { titleParts, test } of walkSuites(root)) {
      const outcome = test.outcome || test.status;
      if (outcome && (outcome === 'unexpected' || outcome === 'flaky')) {
        const project = test.projectName || (test.project && test.project.name) || 'unknown';
        const result = (test.results && test.results[0]) || {};
        const err = result.error || {};
        const msg = (err.message || '').split('\n')[0] || outcome;
        const attachments = (result.attachments || []).filter(a => a.path).map(a => ({ name: a.name, path: a.path }));
        failures.push({ title: titleParts.join(' › '), project, msg, attachments });
      }
    }
  }

  let md = '# E2E Issues (Playwright failures)\n\n';
  if (!failures.length) {
    md += 'No failures. ✅\n';
  } else {
    for (const f of failures) {
      md += `- [${f.project}] ${f.title} — ${f.msg}\n`;
      for (const a of f.attachments) {
        md += `  - ${a.name}: ${a.path}\n`;
      }
    }
  }
  fs.mkdirSync(path.dirname(outPath), { recursive: true });
  fs.writeFileSync(outPath, md);
  console.log(`Wrote ${outPath}`);
}

summarize();
