import { spawnSync } from 'node:child_process';
import http from 'node:http';
import net from 'node:net';
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);
const root = path.resolve(__dirname, '..');
const stackDir = path.resolve(root, 'tools/remote-stack');
const configDir = path.resolve(stackDir, 'config');
const dataDir = path.resolve(stackDir, 'data');
const certPath = path.resolve(configDir, 'hub.crt');
const keyPath = path.resolve(configDir, 'hub.key');
const combinedCAPath = path.resolve(stackDir, 'combined-ca.pem');

async function waitTcp(port: number, host = '127.0.0.1', ms = 15_000) {
  const deadline = Date.now() + ms;
  return new Promise<void>((resolve, reject) => {
    const tryOnce = () => {
      const socket = new net.Socket();
      socket.setTimeout(2000);
      socket.once('error', () => done(false));
      socket.once('timeout', () => done(false));
      socket.connect(port, host, () => done(true));
      function done(ok: boolean) {
        socket.destroy();
        if (ok) return resolve();
        if (Date.now() >= deadline) return reject(new Error(`timeout waiting tcp ${host}:${port}`));
        setTimeout(tryOnce, 250);
      }
    };
    tryOnce();
  });
}

async function postJSON(url: string, body: any) {
  const { hostname, port, pathname } = new URL(url);
  const data = JSON.stringify(body);
  const opts: http.RequestOptions = { hostname, port: Number(port) || 80, path: pathname, method: 'POST', headers: { 'Content-Type': 'application/json', 'Content-Length': Buffer.byteLength(data) } };
  return new Promise<void>((resolve, reject) => {
    const req = http.request(opts, (res) => {
      res.resume();
      if (res.statusCode && res.statusCode >= 200 && res.statusCode < 300) resolve();
      else reject(new Error(`HTTP ${res.statusCode}`));
    });
    req.on('error', reject);
    req.write(data);
    req.end();
  });
}

function ensureDirectories() {
  fs.mkdirSync(configDir, { recursive: true });
  fs.mkdirSync(dataDir, { recursive: true });
}

function ensureSelfSignedCertificate() {
  if (fs.existsSync(certPath) && fs.existsSync(keyPath)) return;
  const opensslCfg = path.resolve(configDir, 'openssl.cnf');
  const cfg = `\
[req]
default_bits = 2048
prompt = no
default_md = sha256
distinguished_name = dn
x509_extensions = v3_req

[dn]
CN = localhost

[v3_req]
subjectAltName = @alt_names

[alt_names]
DNS.1 = localhost
IP.1 = 127.0.0.1
`;
  fs.writeFileSync(opensslCfg, cfg, 'utf8');
  const result = spawnSync('openssl', [
    'req', '-x509', '-nodes', '-days', '2',
    '-newkey', 'rsa:2048',
    '-keyout', keyPath,
    '-out', certPath,
    '-config', opensslCfg,
  ], { stdio: 'inherit' });
  if (result.status !== 0) {
    throw new Error('failed to generate self-signed certificate');
  }
}

function writeNexusConfig() {
  const secret = process.env.PICCOLO_E2E_NEXUS_SECRET || 'local-secret';
  const cfg = `backendListenAddress: ":8443"
peerListenAddress: ":8444"
relayPorts:
  - 443
  - 80
idleTimeoutSeconds: 300
backendsJWTSecret: "${secret}"
peers: []
peerAuthentication:
  trustedDomainSuffixes: []
hubTlsCertFile: "/config/hub.crt"
hubTlsKeyFile: "/config/hub.key"
acmeCacheDir: "/var/lib/nexus-proxy-server/acme"
`;
  fs.writeFileSync(path.resolve(configDir, 'nexus-config.yaml'), cfg, 'utf8');
}

function ensureCombinedCA() {
  const candidates = [
    '/etc/ssl/certs/ca-certificates.crt',
    '/etc/pki/tls/certs/ca-bundle.crt',
    '/etc/ssl/ca-bundle.pem',
    '/etc/pki/tls/cacert.pem',
  ];
  const system = candidates.find((p) => fs.existsSync(p));
  if (!system) {
    throw new Error('could not locate system CA bundle');
  }
  fs.copyFileSync(system, combinedCAPath);
  fs.appendFileSync(combinedCAPath, fs.readFileSync(certPath));
  process.env.SSL_CERT_FILE = combinedCAPath;
}

function ensurePebbleCA() {
  const caOut = path.resolve(stackDir, 'pebble.minica.pem');
  try {
    spawnSync('docker', ['cp', 'piccolo-e2e-pebble:/test/certs/pebble.minica.pem', caOut], { stdio: 'ignore' });
    if (fs.existsSync(caOut)) {
      process.env.LEGO_CA_CERTIFICATES = caOut;
    }
  } catch {
    console.warn('WARN: could not copy Pebble CA bundle');
  }
  process.env.PICCOLO_ACME_DIR_URL = process.env.PICCOLO_ACME_DIR_URL || 'https://localhost:14000/dir';
}

async function upRemoteStack() {
  ensureDirectories();
  ensureSelfSignedCertificate();
  writeNexusConfig();

  const compose = process.env.DOCKER_COMPOSE || 'docker compose';
  const file = path.resolve(stackDir, 'docker-compose.yml');
  const up = spawnSync(compose, ['-f', file, 'up', '-d'], { stdio: 'inherit', shell: true });
  if (up.status !== 0) throw new Error('remote stack up failed');

  await waitTcp(14000, '127.0.0.1', 20_000);
  await waitTcp(8443, '127.0.0.1', 20_000);

  try {
    await postJSON('http://127.0.0.1:8055/set-default-ipv4', { ip: '172.29.0.10' });
    const tld = process.env.PICCOLO_E2E_TLD || 'example.com';
    const portal = `${process.env.PICCOLO_E2E_PORTAL || 'portal-e2e'}.${tld}`;
    const sample = `app.${tld}`;
    const nexusIP = '172.29.0.10';
    await postJSON('http://127.0.0.1:8055/add-a', { host: portal, addresses: [nexusIP] });
    await postJSON('http://127.0.0.1:8055/add-a', { host: sample, addresses: [nexusIP] });
  } catch (e) {
    console.warn('WARN: challtestsrv set-default-ipv4 failed:', e);
  }

  ensurePebbleCA();
  ensureCombinedCA();
}

async function downRemoteStack() {
  const compose = process.env.DOCKER_COMPOSE || 'docker compose';
  const file = path.resolve(stackDir, 'docker-compose.yml');
  spawnSync(compose, ['-f', file, 'down', '-v'], { stdio: 'inherit', shell: true });
}

async function globalSetup() {
  if (process.env.E2E_REMOTE_STACK === '1') {
    await upRemoteStack();
  }
}

async function globalTeardown() {
  if (process.env.E2E_REMOTE_STACK === '1') {
    await downRemoteStack();
  }
}

export default globalSetup;
export { globalTeardown };
