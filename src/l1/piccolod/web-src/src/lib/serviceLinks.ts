export type ServiceLinkInput = {
  scheme?: string | null;
  public_port?: number | null;
};

const LOCAL_SUFFIXES = ['.local', '.lan', '.home.arpa'];
const VALID_SCHEMES = ['http', 'https', 'ws', 'wss'] as const;

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

  if (publicPort) {
    return `${scheme}://${host}:${publicPort}/`;
  }

  return null;
}
