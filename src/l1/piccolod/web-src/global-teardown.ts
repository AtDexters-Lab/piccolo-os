import { spawnSync } from 'node:child_process';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);
const root = path.resolve(__dirname, '..');
const stackDir = path.resolve(root, 'tools/remote-stack');

export default async function globalTeardown() {
  if (process.env.E2E_REMOTE_STACK !== '1') return;
  const compose = process.env.DOCKER_COMPOSE || 'docker compose';
  const file = path.resolve(stackDir, 'docker-compose.yml');
  spawnSync(compose, ['-f', file, 'down', '-v'], { stdio: 'inherit', shell: true });
}
