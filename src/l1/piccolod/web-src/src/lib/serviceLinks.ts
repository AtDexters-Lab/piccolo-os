export type ServiceLinkInput = {
  scheme?: string | null;
  public_port?: number | null;
  remote_host?: string | null;
  remote_ports?: number[] | null;
};

const LOCAL_SUFFIXES = ['.local', '.lan', '.home.arpa'];
const VALID_SCHEMES = ['http', 'https', 'ws', 'wss'] as const;
const DEFAULT_PORT_BY_SCHEME: Record<string, number> = {
  http: 80,
  https: 443,
  ws: 80,
  wss: 443
};
const TLS_UPGRADE: Record<string, string> = { http: 'https', ws: 'wss' };

const LOOPBACK_HOSTS = new Set(['localhost', '127.0.0.1', '::1']);

function isIPv4(host: string): boolean {
  const segments = host.split('.');
  if (segments.length !== 4) return false;
  return segments.every((segment) => {
    if (!segment || segment.length > 3) return false;
    for (let i = 0; i < segment.length; i += 1) {
      const code = segment.charCodeAt(i);
      if (code < 48 || code > 57) {
        return false;
      }
    }
    const value = Number(segment);
    return value >= 0 && value <= 255;
  });
}

function defaultHost(hostOverride?: string): string {
  if (hostOverride && hostOverride.trim() !== '') {
    return hostOverride.trim();
  }
  if (typeof window !== 'undefined' && window.location.hostname) {
    return window.location.hostname;
  }
  return '127.0.0.1';
}

export function isLikelyLocalHost(host?: string): boolean {
  if (!host) return true;
  const normalized = host.toLowerCase();
  if (LOOPBACK_HOSTS.has(normalized)) return true;
  if (isIPv4(normalized)) return true;
  return LOCAL_SUFFIXES.some((suffix) => normalized.endsWith(suffix));
}

export function normalizeScheme(value?: string | null): string {
  const scheme = (value || '').toLowerCase();
  return VALID_SCHEMES.includes(scheme as (typeof VALID_SCHEMES)[number]) ? scheme : 'http';
}

function filterPorts(value?: number[] | null): number[] {
  if (!Array.isArray(value)) return [];
  return value.filter((port) => typeof port === 'number' && Number.isFinite(port) && port > 0);
}

function computeRemoteLink(service: ServiceLinkInput, baseScheme: string): string | null {
  const rawRemoteHost = typeof service.remote_host === 'string' ? service.remote_host.trim() : '';
  if (!rawRemoteHost) {
    return null;
  }
  const remoteHost = rawRemoteHost.replace(/\/+$/, '');
  let remoteScheme = baseScheme;
  if (TLS_UPGRADE[remoteScheme]) {
    remoteScheme = TLS_UPGRADE[remoteScheme];
  }
  const remotePorts = filterPorts(service.remote_ports ?? undefined);
  const defaultPort = DEFAULT_PORT_BY_SCHEME[remoteScheme];
  let chosenPort = 0;
  if (remotePorts.length > 0) {
    if (defaultPort && remotePorts.includes(defaultPort)) {
      chosenPort = defaultPort;
    } else {
      chosenPort = remotePorts[0];
    }
  } else if (defaultPort) {
    chosenPort = defaultPort;
  }
  const portSegment = chosenPort && defaultPort && chosenPort === defaultPort ? '' : chosenPort ? `:${chosenPort}` : '';
  return `${remoteScheme}://${remoteHost}${portSegment}/`;
}

export function buildServiceLink(service: ServiceLinkInput | null | undefined, hostOverride?: string): string | null {
  if (!service) return null;

  const host = defaultHost(hostOverride);
  const scheme = normalizeScheme(service.scheme ?? undefined);
  const publicPort = typeof service.public_port === 'number' ? service.public_port : null;

  if (isLikelyLocalHost(host)) {
    if (publicPort) {
      return `${scheme}://${host}:${publicPort}/`;
    }
    return null;
  }

  const remoteLink = computeRemoteLink(service, scheme);
  if (remoteLink) {
    return remoteLink;
  }

  if (publicPort) {
    return `${scheme}://${host}:${publicPort}/`;
  }

  return null;
}

export function buildRemoteServiceLink(service: ServiceLinkInput | null | undefined): string | null {
  if (!service) {
    return null;
  }
  const scheme = normalizeScheme(service.scheme ?? undefined);
  return computeRemoteLink(service, scheme);
}
