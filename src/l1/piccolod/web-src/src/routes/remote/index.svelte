<script lang="ts">
  import { onMount } from 'svelte';
  import { apiProd, type ErrorResponse } from '@api/client';
  import { toast } from '@stores/ui';

  type RemoteState = 'disabled' | 'provisioning' | 'preflight_required' | 'active' | 'warning' | 'error';
  type Solver = 'http-01' | 'dns-01';
  type PortalMode = 'root' | 'subdomain';

  interface AliasRow {
    id?: string;
    listener: string;
    hostname: string;
    status?: 'pending' | 'active' | 'error' | 'warning';
    last_checked?: string | null;
    message?: string | null;
  }

  interface CertificateRow {
    id?: string;
    domains: string[];
    solver?: Solver;
    issued_at?: string | null;
    expires_at?: string | null;
    next_renewal?: string | null;
    status?: 'ok' | 'warning' | 'error' | 'pending';
    failure_reason?: string | null;
  }

  interface TimelineEvent {
    ts?: string;
    level?: 'info' | 'warn' | 'error' | 'success';
    source?: string;
    message?: string;
    next_step?: string | null;
  }

  interface PreflightCheck {
    name: string;
    status: 'pass' | 'fail' | 'warn' | 'pending';
    detail?: string | null;
    next_step?: string | null;
  }

  interface DnsProviderField {
    id: string;
    label: string;
    secret?: boolean;
    placeholder?: string;
    description?: string;
  }

  interface DnsProvider {
    id: string;
    name: string;
    docs_url?: string;
    fields: DnsProviderField[];
  }

  interface NexusGuide {
    command: string;
    requirements: string[];
    verified_at?: string | null;
    docs_url?: string;
    notes?: string[];
  }

  interface RemoteListenerSummary {
    name: string;
    remote_host?: string | null;
  }

  interface RemoteStatusPayload {
    enabled?: boolean;
    state?: RemoteState;
    solver?: Solver;
    endpoint?: string | null;
    hostname?: string | null;
    portal_hostname?: string | null;
    tld?: string | null;
    latency_ms?: number | null;
    last_handshake?: string | null;
    next_renewal?: string | null;
    issuer?: string | null;
    expires_at?: string | null;
    warnings?: string[];
    guide_verified_at?: string | null;
    nexus_verified_at?: string | null;
    device_id?: string | null;
    listeners?: RemoteListenerSummary[];
    aliases?: AliasRow[];
    certificates?: CertificateRow[];
  }

  const fallbackGuide: NexusGuide = {
    command: "sudo bash -c 'curl -fsSL https://raw.githubusercontent.com/AtDexters-Lab/nexus-proxy-server/main/scripts/install.sh | bash'",
    requirements: [
      'Systemd-based Linux VM (Ubuntu 22.04+/Fedora/Flatcar) with sudo access',
      'Static public IPv4 or IPv6 address',
      'Inbound ports 80/TCP and 443/TCP open',
    ],
    notes: [
      'Installer prompts for the Nexus hostname and prints the backend JWT secret on success.',
      'Keep the terminal open until completion and copy the secret before closing the session.'
    ],
    docs_url: 'https://github.com/AtDexters-Lab/nexus-proxy-server/blob/main/readme.md#install'
  };

  const stateStyles: Record<RemoteState, { bg: string; text: string; border: string }> = {
    disabled: { bg: 'bg-slate-50', text: 'text-slate-700', border: 'border-slate-200' },
    provisioning: { bg: 'bg-blue-50', text: 'text-blue-800', border: 'border-blue-200' },
    preflight_required: { bg: 'bg-sky-50', text: 'text-sky-800', border: 'border-sky-200' },
    active: { bg: 'bg-emerald-50', text: 'text-emerald-800', border: 'border-emerald-200' },
    warning: { bg: 'bg-amber-50', text: 'text-amber-900', border: 'border-amber-200' },
    error: { bg: 'bg-rose-50', text: 'text-rose-900', border: 'border-rose-200' }
  };

  let loading = true;
  let statusError = '';
  let saving = false;
  let statusPayload: RemoteStatusPayload | null = null;
  let state: RemoteState = 'disabled';
  let solver: Solver = 'http-01';
  let formInitialized = false;

  let httpForm = {
    endpoint: '',
    jwtSecret: ''
  };

  let dnsForm = {
    provider: '',
    credentials: {} as Record<string, string>
  };

  let providers: DnsProvider[] = [];
  let dnsProviderFields: DnsProviderField[] = [];
  let listenerOptions: { name: string; label: string }[] = [];
  let providerLookup = new Map<string, DnsProvider>();
  let nexusGuide: NexusGuide = fallbackGuide;
  let guideOpen = false;
  let guideJwtSecret = '';
  let guideVerifyMessage = '';
  let verifyingNexus = false;
  let guideVerifiedAt: string | null = null;

  let aliases: AliasRow[] = [];
  let aliasDialogOpen = false;
  let aliasForm = { hostname: '', listener: '', note: '' };
  let aliasWorking = false;

  let certificates: CertificateRow[] = [];
  let renewing: Record<string, boolean> = {};
  let events: TimelineEvent[] = [];
  let preflightChecks: PreflightCheck[] = [];
  let preflightTimestamp: string | null = null;
  let preflightWorking = false;

  let services: { app?: string; name?: string; remote_hosts?: string[] }[] = [];

  let domainTld = '';
  let portalMode: PortalMode = 'subdomain';
  let portalPrefix = 'portal';
  let nexusComplete = false;
  let domainComplete = false;
  let solverComplete = false;
  let solverAttention = false;
  let initialStep: StepKey = 'nexus';

  type StepKey = 'nexus' | 'domain' | 'solver';
  const stepOrder: StepKey[] = ['nexus', 'domain', 'solver'];
  let activeStep: StepKey = 'nexus';
  let userSelectedStep = false;

  const stepCopy: Record<StepKey, { title: string; description: string }> = {
    nexus: { title: 'Connect Nexus', description: 'Use the endpoint and JWT secret produced by the Nexus helper.' },
    domain: { title: 'Domain & portal host', description: 'Choose the Piccolo domain and portal hostname that remote users will visit.' },
    solver: { title: 'Certificate solver & preflight', description: 'Pick HTTP-01 or DNS-01, add credentials, then run preflight.' }
  };

  function updateCredential(key: string, value: string) {
    dnsForm = { ...dnsForm, credentials: { ...dnsForm.credentials, [key]: value } };
  }

  function handleCredentialInput(key: string, event: Event) {
    const target = event.currentTarget as HTMLInputElement | null;
    updateCredential(key, target?.value ?? '');
  }

  function computePortalHostnameValue(): string {
    const tld = domainTld.trim();
    if (!tld) return '';
    if (portalMode === 'root') return tld;
    const prefix = portalPrefix.trim() || 'portal';
    return `${prefix}.${tld}`;
  }

  function selectStep(step: StepKey) {
    activeStep = step;
    userSelectedStep = true;
  }

  function determineInitialStep(payload: RemoteStatusPayload | null): StepKey {
    if (!payload || !payload.endpoint) return 'nexus';
    if (!payload.tld || !(payload.portal_hostname || payload.hostname)) return 'domain';
    return 'solver';
  }

  type StepStatus = 'done' | 'current' | 'pending' | 'attention';

  function stepStatus(step: StepKey): StepStatus {
    if (step === 'nexus') {
      if (nexusComplete) return 'done';
      return activeStep === 'nexus' ? 'current' : 'pending';
    }
    if (step === 'domain') {
      if (domainComplete && nexusComplete) return 'done';
      return activeStep === 'domain' ? 'current' : 'pending';
    }
    if (solverAttention) return 'attention';
    if (solverComplete && domainComplete && nexusComplete) return 'done';
    return activeStep === 'solver' ? 'current' : 'pending';
  }

  function stepStatusLabel(status: StepStatus): string {
    if (status === 'done') return 'Done';
    if (status === 'current') return 'In progress';
    if (status === 'attention') return 'Attention';
    return 'Pending';
  }

  function extractEndpointHost(raw: string): string {
    if (!raw) return '';
    try {
      const url = new URL(raw);
      return url.hostname || raw;
    } catch (err) {
      const stripped = raw.replace(/^wss?:\/\//, '').replace(/\/.*$/, '');
      return stripped.split(':')[0] || stripped || raw;
    }
  }

  $: trimmedTld = domainTld.trim();
  $: portalHostname = computePortalHostnameValue();
  $: wildcardHost = trimmedTld ? `*.${trimmedTld}` : '';
  $: listenerSample = statusPayload?.listeners?.[0]?.name ?? 'service';
  $: remotePreviewHost = trimmedTld ? `${listenerSample}.${trimmedTld}` : `${listenerSample}.your-domain.com`;
  $: nexusHostHint = (() => {
    const raw = httpForm.endpoint.trim() || statusPayload?.endpoint || '';
    return extractEndpointHost(raw) || 'nexus.example.com';
  })();
  $: if (portalMode === 'root' && !portalPrefix.trim()) {
    portalPrefix = 'portal';
  }
  $: domainExample = trimmedTld || 'your-domain.com';
  $: prefixSafe = portalPrefix.trim() || 'portal';
  $: portalDisplay = portalMode === 'root'
    ? domainExample
    : (portalHostname || (trimmedTld ? `${prefixSafe}.${trimmedTld}` : `${prefixSafe}.${domainExample}`));
  $: wildcardDisplay = wildcardHost || `*.${domainExample}`;
  $: listenerPreviewUrl = `https://${remotePreviewHost}`;
  $: initialStep = determineInitialStep(statusPayload);
  $: {
    const existingEndpoint = (statusPayload?.endpoint ?? '').trim();
    const existingTld = (statusPayload?.tld ?? '').trim();
    const existingPortal = (statusPayload?.portal_hostname ?? statusPayload?.hostname ?? '').trim();
    nexusComplete = Boolean(existingEndpoint || httpForm.endpoint.trim());
    domainComplete = Boolean((existingTld && existingPortal) || (trimmedTld && portalHostname));
    solverAttention = state === 'warning' || state === 'error';
    solverComplete = Boolean(statusPayload?.enabled) || state === 'preflight_required' || state === 'active' || solverAttention;
    if (!userSelectedStep) {
      if (!nexusComplete) {
        activeStep = 'nexus';
      } else if (!domainComplete) {
        activeStep = 'domain';
      } else if (!solverComplete) {
        activeStep = 'solver';
      } else {
        activeStep = initialStep;
      }
    }
  }

  function formatDate(ts?: string | null): string {
    if (!ts) return '-';
    const date = new Date(ts);
    if (Number.isNaN(date.getTime())) return '-';
    return date.toLocaleString(undefined, { dateStyle: 'medium', timeStyle: 'short' });
  }

  function daysUntil(ts?: string | null): number | null {
    if (!ts) return null;
    const date = new Date(ts);
    if (Number.isNaN(date.getTime())) return null;
    const diff = date.getTime() - Date.now();
    return Math.round(diff / (1000 * 60 * 60 * 24));
  }

  function stateTitle(value: RemoteState): string {
    switch (value) {
      case 'provisioning':
        return 'Provisioning Nexus';
      case 'preflight_required':
        return 'Preflight Required';
      case 'active':
        return 'Remote Access Active';
      case 'warning':
        return 'Attention Needed';
      case 'error':
        return 'Remote Access Blocked';
      default:
        return 'Remote Access Disabled';
    }
  }

  function stateDescription(payload: RemoteStatusPayload | null, value: RemoteState): string {
    if (!payload) return 'Remote access is currently disabled.';
    if (value === 'disabled') return 'Remote access is currently disabled.';
    if (value === 'provisioning') return 'Complete the Nexus helper to verify the tunnel before configuring certificates.';
    if (value === 'preflight_required') return 'Configuration saved. Run preflight to validate DNS and the Nexus connection.';
    if (value === 'active') return `Remote portal reachable at ${payload.portal_hostname || payload.hostname || 'configured host'}.`;
    if (value === 'warning') return 'Remote access is active but requires attention to maintain availability.';
    return 'Remote access is blocked until the highlighted issues are resolved.';
  }

  function deriveState(payload: RemoteStatusPayload | null): RemoteState {
    if (!payload) return 'disabled';
    if (payload.state) return payload.state;
    if (!payload.enabled) return 'disabled';
    if (payload.warnings && payload.warnings.length > 0) return 'warning';
    if (payload.expires_at) {
      const days = daysUntil(payload.expires_at);
      if (typeof days === 'number' && days <= 0) return 'error';
      if (typeof days === 'number' && days <= 10) return 'warning';
    }
    return 'active';
  }

  function applyStatus(payload: RemoteStatusPayload) {
    statusPayload = payload;
    state = deriveState(payload);
    solver = payload.solver ?? (payload.enabled ? 'http-01' : solver);
    guideVerifiedAt = payload.guide_verified_at ?? payload.nexus_verified_at ?? guideVerifiedAt;
    aliases = payload.aliases ?? aliases;
    certificates = payload.certificates ?? certificates;
    const previousProvider = dnsForm.provider || '';
    if (!formInitialized) {
      let tld = (payload.tld ?? '').trim();
      let portalHost = (payload.portal_hostname ?? payload.hostname ?? '').trim();
      let mode: PortalMode = 'subdomain';
      let prefix = portalPrefix || 'portal';

      if (!tld && portalHost) {
        tld = portalHost;
      }

      if (tld && portalHost) {
        if (portalHost === tld) {
          mode = 'root';
        } else if (portalHost.endsWith(`.${tld}`)) {
          mode = 'subdomain';
          prefix = portalHost.slice(0, portalHost.length - (tld.length + 1)) || 'portal';
        } else {
          mode = 'root';
          tld = portalHost;
        }
      }

      domainTld = tld;
      portalMode = mode;
      portalPrefix = mode === 'subdomain' ? (prefix || 'portal') : 'portal';

      httpForm = {
        endpoint: payload.endpoint ?? '',
        jwtSecret: ''
      };
      dnsForm = {
        provider: previousProvider,
        credentials: {}
      };
      formInitialized = true;
    }
  }

  async function loadStatus() {
    try {
      const data = await apiProd<RemoteStatusPayload>('/remote/status');
      applyStatus(data);
      statusError = '';
    } catch (err: any) {
      statusError = err?.message || 'Failed to load remote status';
    }
  }

  async function loadCertificates() {
    try {
      const data = await apiProd<{ certificates?: CertificateRow[] }>('/remote/certificates');
      certificates = data.certificates ?? [];
    } catch (err: any) {
      if (err?.code === 404) {
        // Endpoint not yet available; fall back to payload snapshot
        certificates = statusPayload?.certificates ?? [];
      }
    }
  }

  async function renewCert(cert: CertificateRow) {
    const id = (cert.id || cert.domains?.[0] || '').trim();
    if (!id) {
      toast('Missing certificate identifier', 'error');
      return;
    }
    if (renewing[id]) return;
    renewing = { ...renewing, [id]: true };
    try {
      await apiProd(`/remote/certificates/${encodeURIComponent(id)}/renew`, { method: 'POST' });
      toast('Issuance queued', 'success');
      await loadCertificates();
      await loadEvents();
    } catch (err: any) {
      toast(err?.message || 'Failed to queue issuance', 'error');
    } finally {
      renewing = { ...renewing, [id]: false };
    }
  }

  async function loadAliases() {
    try {
      const data = await apiProd<{ aliases?: AliasRow[] }>('/remote/aliases');
      aliases = data.aliases ?? [];
    } catch (err: any) {
      if (err?.code === 404) {
        aliases = statusPayload?.aliases ?? [];
      }
    }
  }

  async function loadProviders() {
    try {
      const data = await apiProd<{ providers?: DnsProvider[] }>('/remote/dns/providers');
      providers = data.providers ?? [];
    } catch (err: any) {
      if (err?.code === 404) {
        providers = [];
      }
    }
    if (!providers.length) {
      providers = [{ id: 'generic', name: 'Generic DNS Provider', fields: [] }];
    }
    providerLookup = new Map(providers.map((p) => [p.id, p]));
    if (!dnsForm.provider && providers.length) {
      dnsForm = { ...dnsForm, provider: providers[0].id };
    }
  }

  async function loadGuide() {
    try {
      const data = await apiProd<NexusGuide>('/remote/nexus-guide');
      if (data) {
        nexusGuide = { ...fallbackGuide, ...data };
        guideVerifiedAt = data.verified_at ?? guideVerifiedAt;
      }
    } catch (err: any) {
      // Keep fallback guide
      if (err?.code && err.code !== 404) {
        toast(err.message || 'Failed to load Nexus helper', 'error');
      }
    }
  }

  async function loadEvents() {
    try {
      const data = await apiProd<{ events?: TimelineEvent[] }>('/remote/events');
      events = data.events ?? [];
    } catch (err: any) {
      if (err?.code === 404) {
        events = [];
      }
    }
  }

  async function loadServices() {
    try {
      const data = await apiProd<{ services?: { app?: string; name?: string; remote_hosts?: string[] }[] }>('/services');
      services = data.services ?? [];
    } catch (err: any) {
      services = [];
    }
  }

  async function loadAll() {
    userSelectedStep = false;
    loading = true;
    await Promise.all([loadStatus(), loadCertificates(), loadAliases(), loadProviders(), loadGuide(), loadEvents(), loadServices()]);
    loading = false;
  }

  onMount(loadAll);

  async function runPreflight() {
    preflightWorking = true;
    guideVerifyMessage = '';
    try {
      const data = await apiProd<{ checks?: PreflightCheck[]; ran_at?: string }>('/remote/preflight', { method: 'POST' });
      preflightChecks = data.checks ?? [];
      preflightTimestamp = data.ran_at ?? new Date().toISOString();
      toast('Preflight completed', 'success');
      await loadStatus();
      await loadCertificates();
      await loadAliases();
    } catch (err: any) {
      if (err?.code === 404) {
        toast('Preflight endpoint is not available yet', 'info');
      } else {
        toast(err.message || 'Preflight failed', 'error');
      }
    } finally {
      preflightWorking = false;
    }
  }

  async function configureRemote() {
    saving = true;
    try {
      const endpoint = httpForm.endpoint.trim() || undefined;
      const deviceSecret = httpForm.jwtSecret.trim() || undefined;
      const tldRaw = domainTld.trim();
      const prefixRaw = portalPrefix.trim();

      let tld = tldRaw ? tldRaw.toLowerCase() : '';
      if (portalMode !== 'root' && !prefixRaw) {
        toast('Enter a portal subdomain before saving.', 'error');
        return;
      }
      if (!endpoint || !deviceSecret) {
        toast('Provide the Nexus endpoint and JWT signing secret.', 'error');
        return;
      }
      if (!tld) {
        toast('Enter the Piccolo domain (TLD).', 'error');
        return;
      }

      let portalHost = '';
      if (portalMode === 'root') {
        portalHost = tld;
      } else {
        portalHost = `${prefixRaw.toLowerCase()}.${tld}`;
      }

      if (!portalHost) {
        toast('Portal hostname is required.', 'error');
        return;
      }
      if (solver === 'dns-01' && !dnsForm.provider) {
        toast('Select a DNS provider for DNS-01 issuance.', 'error');
        return;
      }

      const basePayload: Record<string, unknown> = {
        solver,
        endpoint,
        device_secret: deviceSecret,
        tld,
        portal_hostname: portalHost,
      };

      const payload = solver === 'dns-01'
        ? { ...basePayload, dns_provider: dnsForm.provider, dns_credentials: dnsForm.credentials }
        : basePayload;

      await apiProd('/remote/configure', {
        method: 'POST',
        body: JSON.stringify(payload)
      });
      toast('Remote configuration saved', 'success');
      formInitialized = false;
      userSelectedStep = false;
      await loadStatus();
      await runPreflight();
    } catch (err: any) {
      toast(err.message || 'Failed to save configuration', 'error');
    } finally {
      saving = false;
    }
  }

  async function disableRemote() {
    if (!confirm('Disable remote access? Public reachability will stop immediately.')) return;
    saving = true;
    try {
      await apiProd('/remote/disable', { method: 'POST' });
      toast('Remote access disabled', 'success');
      await loadAll();
    } catch (err: any) {
      toast(err.message || 'Disable failed', 'error');
    } finally {
      saving = false;
    }
  }

  async function rotateRemote() {
    saving = true;
    try {
      const data = await apiProd<{ message?: string; device_secret?: string }>('/remote/rotate', { method: 'POST' });
      if (data?.device_secret) {
        httpForm = { ...httpForm, jwtSecret: data.device_secret };
        toast('Remote credentials rotated. Update your Nexus backend client.', 'success');
      } else {
        toast('Remote credentials rotated', 'success');
      }
      await loadStatus();
    } catch (err: any) {
      toast(err.message || 'Rotate failed', 'error');
    } finally {
      saving = false;
    }
  }

  async function verifyNexusEndpoint() {
    verifyingNexus = true;
    guideVerifyMessage = '';
    try {
      const payload = {
        endpoint: httpForm.endpoint.trim() || undefined,
        tld: domainTld.trim() || undefined,
        portal_hostname: portalHostname.trim() || undefined,
        jwt_secret: guideJwtSecret.trim() || undefined
      };
      const data = await apiProd<{ verified_at?: string; message?: string }>('/remote/nexus-guide/verify', {
        method: 'POST',
        body: JSON.stringify(payload)
      });
      guideVerifiedAt = data.verified_at ?? new Date().toISOString();
      guideVerifyMessage = data.message || 'Nexus endpoint verified. Continue with certificate setup.';
      if (guideJwtSecret.trim()) {
        httpForm = { ...httpForm, jwtSecret: guideJwtSecret.trim() };
      }
      toast('Nexus server verified', 'success');
      guideOpen = false;
      await loadStatus();
    } catch (err: any) {
      const error = err as ErrorResponse;
      if (error?.code === 404) {
        guideVerifyMessage = 'Verification API not available. Continue after validating manually.';
        guideVerifiedAt = new Date().toISOString();
        toast('Marked Nexus install as complete (manual)', 'info');
        guideOpen = false;
      } else {
        guideVerifyMessage = error?.message || 'Verification failed. Ensure the installer completed successfully.';
        toast(guideVerifyMessage, 'error');
      }
    } finally {
      verifyingNexus = false;
    }
  }

  async function addAlias() {
    aliasWorking = true;
    try {
      const payload = {
        hostname: aliasForm.hostname.trim(),
        listener: aliasForm.listener.trim(),
        note: aliasForm.note.trim() || undefined
      };
      await apiProd('/remote/aliases', { method: 'POST', body: JSON.stringify(payload) });
      toast('Alias added', 'success');
      aliasDialogOpen = false;
      aliasForm = { hostname: '', listener: '', note: '' };
      await loadAliases();
      await loadCertificates();
    } catch (err: any) {
      if (err?.code === 404) {
        toast('Alias API not available yet', 'info');
      } else {
        toast(err.message || 'Failed to add alias', 'error');
      }
    } finally {
      aliasWorking = false;
    }
  }

  async function removeAlias(alias: AliasRow) {
    if (!alias.id) {
      toast('Cannot remove alias without identifier', 'error');
      return;
    }
    if (!confirm(`Remove alias ${alias.hostname}?`)) return;
    aliasWorking = true;
    try {
      await apiProd(`/remote/aliases/${encodeURIComponent(alias.id)}`, { method: 'DELETE' });
      toast('Alias removed', 'success');
      await loadAliases();
    } catch (err: any) {
      if (err?.code === 404) {
        toast('Alias removal API not available yet', 'info');
      } else {
        toast(err.message || 'Failed to remove alias', 'error');
      }
    } finally {
      aliasWorking = false;
    }
  }

  function aliasStatusClass(value?: AliasRow['status']): string {
    switch (value) {
      case 'active':
        return 'bg-emerald-100 text-emerald-800';
      case 'warning':
        return 'bg-amber-100 text-amber-900';
      case 'error':
        return 'bg-rose-100 text-rose-900';
      default:
        return 'bg-slate-100 text-slate-700';
    }
  }

  function checkStatusClass(value: PreflightCheck['status']): string {
    if (value === 'pass') return 'bg-emerald-100 text-emerald-800';
    if (value === 'warn') return 'bg-amber-100 text-amber-900';
    if (value === 'fail') return 'bg-rose-100 text-rose-900';
    return 'bg-slate-100 text-slate-700';
  }

  function certificateStatusClass(value?: CertificateRow['status']): string {
    switch (value) {
      case 'ok':
        return 'text-emerald-700';
      case 'warning':
        return 'text-amber-800';
      case 'error':
        return 'text-rose-800';
      case 'pending':
        return 'text-sky-800';
      default:
        return 'text-slate-700';
    }
  }

  $: dnsProviderFields = providerLookup.get(dnsForm.provider)?.fields ?? [];

  $: listenerOptions = (() => {
    const map = new Map<string, string>();
    statusPayload?.listeners?.forEach((l) => {
      if (l.name) map.set(l.name, l.remote_host ? `${l.name} (${l.remote_host})` : l.name);
    });
    services.forEach((service) => {
      if (service?.name) {
        const label = service.app ? `${service.app}/${service.name}` : service.name;
        if (!map.has(service.name)) map.set(service.name, label);
      }
    });
    aliases.forEach((alias) => {
      if (alias.listener && !map.has(alias.listener)) {
        map.set(alias.listener, alias.listener);
      }
    });
    return Array.from(map.entries())
      .map(([name, label]) => ({ name, label }))
      .sort((a, b) => a.label.localeCompare(b.label));
  })();

  const openWizard = () => window.dispatchEvent(new CustomEvent('remote-wizard-open'));

  function describeNextStep(value: RemoteState): string {
    if (!statusPayload) return 'Connect the Nexus helper VM to begin setup.';
    if (value === 'disabled') return 'Connect the Nexus helper and provide the JWT secret.';
    if (value === 'provisioning') return 'Wait for the Nexus helper to report healthy, then configure the portal hostname.';
    if (value === 'preflight_required') return 'Run preflight to verify DNS + certificates, then enable remote.';
    if (value === 'warning') return 'Resolve the highlighted warnings to keep the remote tunnel healthy.';
    if (value === 'error') return 'Remote is blocked—open the wizard and review the failing step.';
    return 'Remote is online. Keep certificates fresh and monitor alias domains.';
  }

  $: remoteEnabled = !!statusPayload?.enabled;
  $: remoteComplete = remoteEnabled && !(statusPayload?.warnings?.length);
  $: remoteNextStep = describeNextStep(state);
  $: heroNexusComplete = Boolean(statusPayload?.endpoint);
  $: heroDomainComplete = Boolean(statusPayload?.tld && (statusPayload?.portal_hostname || statusPayload?.hostname));
  $: heroCertComplete = remoteComplete || state === 'preflight_required';
  $: heroChecklist = [
    {
      label: 'Connect Nexus helper',
      detail: heroNexusComplete ? 'Helper is connected.' : 'Expose ports 80/443 and paste the endpoint.',
      done: heroNexusComplete
    },
    {
      label: 'Portal hostname set',
      detail: heroDomainComplete ? (statusPayload?.portal_hostname || statusPayload?.hostname || '') : 'Choose the Piccolo domain + portal host.',
      done: heroDomainComplete
    },
    {
      label: 'Certificates verified',
      detail: heroCertComplete
        ? statusPayload?.next_renewal
          ? `Next renewal ${formatDate(statusPayload?.next_renewal)}`
          : 'Preflight passed.'
        : 'Run preflight to request certificates.',
      done: heroCertComplete
    }
  ];
  $: remoteHeroCopy = remoteComplete
    ? `Remote portal reachable at ${statusPayload?.portal_hostname || statusPayload?.hostname || 'configured host'}.`
    : remoteNextStep;
  $: remotePrimaryCta = remoteComplete ? 'Review remote setup' : 'Continue setup';
</script>

<svelte:window on:keydown={(event) => {
  if (event.key === 'Escape' && aliasDialogOpen) {
    aliasDialogOpen = false;
  }
}} />

<div class="remote-page space-y-6">
  {#if loading}
    <div class="remote-shell remote-shell--loading">
      <p class="remote-shell__headline">Loading remote status…</p>
    </div>
  {:else if statusError && !statusPayload}
    <div class="remote-shell remote-shell--error">
      <h1 class="remote-shell__headline">Remote access unavailable</h1>
      <p class="remote-shell__copy">{statusError}</p>
      <button class="remote-shell__action" on:click={loadAll}>Retry</button>
    </div>
  {:else}
    {#if statusPayload}
      <section class={`remote-hero rounded-3xl border ${stateStyles[state].border} ${stateStyles[state].bg} ${stateStyles[state].text} shadow-lg`} aria-live="polite">
        <div class="remote-hero__body">
          <p class="remote-hero__eyebrow">Remote status</p>
          <h1 class="remote-hero__title">Remote access</h1>
          <p class="remote-hero__copy">{remoteHeroCopy}</p>
          <div class="remote-hero__checklist" role="list">
            {#each heroChecklist as item}
              <div class={`remote-hero__check ${item.done ? 'remote-hero__check--done' : ''}`} role="listitem">
                <span class="remote-hero__check-dot" aria-hidden="true"></span>
                <div>
                  <p class="remote-hero__check-label">{item.label}</p>
                  <p class="remote-hero__check-detail">{item.detail}</p>
                </div>
              </div>
            {/each}
          </div>
          {#if statusPayload.warnings && statusPayload.warnings.length > 0}
            <ul class="remote-hero__warnings">
              {#each statusPayload.warnings as warning}
                <li>{warning}</li>
              {/each}
            </ul>
          {/if}
        </div>
        <div class="remote-hero__actions">
          <button class="remote-hero__cta" on:click={openWizard}>
            {remotePrimaryCta}
          </button>
          <div class="remote-hero__links">
            <p class="remote-hero__meta">
              {remoteComplete
                ? `Portal host ${statusPayload?.portal_hostname || statusPayload?.hostname || 'configured host'}`
                : remoteNextStep}
            </p>
            {#if !remoteComplete}
              <button class="remote-hero__link" type="button" on:click={() => (guideOpen = true)}>
                View Nexus helper guide
              </button>
            {/if}
            {#if state !== 'disabled'}
              <button class="remote-hero__link" on:click={disableRemote} disabled={saving}>
                {saving ? 'Disabling…' : 'Disable remote'}
              </button>
            {/if}
            {#if guideVerifiedAt}
              <p class="remote-hero__meta">Nexus helper verified {formatDate(guideVerifiedAt)}.</p>
            {/if}
          </div>
        </div>
      </section>

      <section class="remote-detail-grid">
        <div class="remote-detail-card">
          <p class="remote-detail-card__label">Portal host</p>
          <p class="remote-detail-card__value">{statusPayload?.portal_hostname || statusPayload?.hostname || 'Not configured'}</p>
        </div>
        <div class="remote-detail-card">
          <p class="remote-detail-card__label">Piccolo domain</p>
          <p class="remote-detail-card__value">{statusPayload?.tld || 'Not set'}</p>
        </div>
        <div class="remote-detail-card">
          <p class="remote-detail-card__label">Nexus endpoint</p>
          <p class="remote-detail-card__value">{statusPayload?.endpoint ? extractEndpointHost(statusPayload.endpoint) : 'Not connected'}</p>
        </div>
        <div class="remote-detail-card">
          <p class="remote-detail-card__label">Solver</p>
          <p class="remote-detail-card__value">{statusPayload?.solver ? statusPayload.solver.toUpperCase() : 'Not selected'}</p>
        </div>
        <div class="remote-detail-card">
          <p class="remote-detail-card__label">Certificate expires</p>
          <p class="remote-detail-card__value">{formatDate(statusPayload?.expires_at)}</p>
        </div>
        <div class="remote-detail-card">
          <p class="remote-detail-card__label">Latency</p>
          <p class="remote-detail-card__value">{typeof statusPayload?.latency_ms === 'number' ? `${statusPayload.latency_ms} ms` : 'Not measured'}</p>
        </div>
      </section>

      <details class="remote-advanced">
        <summary class="remote-advanced__summary">
          Advanced configuration
        </summary>
        <div class="remote-advanced__content space-y-6">
          {#if !remoteComplete}
            <section class="remote-card remote-card--muted">
              <div class="remote-card__header">
                <div>
                  <h2>Finish setup to manage advanced controls</h2>
                  <p>Certificates, alias domains, and preflight history unlock once the wizard passes.</p>
                </div>
              </div>
              <div class="remote-card__body">
                <button class="remote-hero__cta" on:click={openWizard}>Continue setup</button>
              </div>
            </section>
          {:else}
          <section class="remote-card">
            <div class="remote-card__header">
              <div>
                <h2>Remote credentials</h2>
                <p>Rotate secrets when refreshing the Nexus helper or recovering access.</p>
              </div>
              <button class="remote-link" on:click={rotateRemote} disabled={saving}>
                Rotate credentials
              </button>
            </div>
            <p class="remote-card__body">New credentials are stored locally until you apply them to the Nexus helper.</p>
          </section>

          <section class="remote-card">
            <div class="remote-card__header">
              <div>
                <h2>Certificate inventory</h2>
                <p>Issued certificates for the portal and wildcard listeners.</p>
              </div>
              <button class="remote-link" on:click={loadCertificates}>Refresh</button>
            </div>
            {#if !certificates.length}
              <p class="remote-card__body">No certificates issued yet.</p>
            {:else}
              <div class="remote-card__list">
                {#each certificates as cert}
                  <div class="remote-cert">
                    <div class="remote-cert__domains">
                      {#each cert.domains as domain}
                        <p>{domain}</p>
                      {/each}
                    </div>
                    <dl class="remote-cert__meta">
                      <div>
                        <dt>Solver</dt>
                        <dd>{cert.solver || '-'}</dd>
                      </div>
                      <div>
                        <dt>Issued</dt>
                        <dd>{formatDate(cert.issued_at)}</dd>
                      </div>
                      <div>
                        <dt>Expires</dt>
                        <dd>{formatDate(cert.expires_at)}</dd>
                      </div>
                      <div>
                        <dt>Next renewal</dt>
                        <dd>{formatDate(cert.next_renewal)}</dd>
                      </div>
                    </dl>
                    <div class="remote-cert__actions">
                      <span class={`remote-cert__status ${certificateStatusClass(cert.status)}`}>{cert.status ? cert.status.toUpperCase() : 'UNKNOWN'}</span>
                      {#if cert.failure_reason}
                        <p class="remote-cert__note">{cert.failure_reason}</p>
                      {/if}
                      {#if cert.id}
                        <button class="remote-link" on:click={() => renewCert(cert)} disabled={renewing[cert.id] || cert.status === 'pending'}>
                          {renewing[cert.id] ? 'Queuing…' : 'Retry issuance'}
                        </button>
                      {/if}
                    </div>
                  </div>
                {/each}
              </div>
            {/if}
          </section>

          <section class="remote-card">
            <div class="remote-card__header">
              <div>
                <h2>Alias domains</h2>
                <p>Map additional hostnames to remote listeners.</p>
              </div>
              <div class="flex items-center gap-3">
                <button class="remote-link" on:click={() => (aliasDialogOpen = true)}>
                  Add alias
                </button>
                <button class="remote-link" on:click={loadAliases}>Refresh</button>
              </div>
            </div>
            {#if !aliases.length}
              <p class="remote-card__body">No alias domains configured.</p>
            {:else}
              <div class="remote-table">
                <table>
                  <thead>
                    <tr>
                      <th>Alias</th>
                      <th>Listener</th>
                      <th>Status</th>
                      <th>Last check</th>
                      <th></th>
                    </tr>
                  </thead>
                  <tbody>
                    {#each aliases as alias}
                      <tr>
                        <td>
                          <div class="remote-table__primary">{alias.hostname}</div>
                          {#if alias.message}
                            <div class="remote-table__secondary">{alias.message}</div>
                          {/if}
                        </td>
                        <td>{alias.listener}</td>
                        <td>
                          <span class={`remote-badge ${aliasStatusClass(alias.status)}`}>
                            {alias.status ? alias.status.toUpperCase() : 'PENDING'}
                          </span>
                        </td>
                        <td>{formatDate(alias.last_checked)}</td>
                        <td>
                          <button class="remote-link" on:click={() => removeAlias(alias)} disabled={aliasWorking || !alias.id}>
                            Remove
                          </button>
                        </td>
                      </tr>
                    {/each}
                  </tbody>
                </table>
              </div>
            {/if}

            {#if aliasDialogOpen}
              <div class="remote-sheet">
                <div class="remote-sheet__header">
                  <h3>Add alias</h3>
                  <button class="remote-link" on:click={() => (aliasDialogOpen = false)}>Cancel</button>
                </div>
                <div class="remote-sheet__body">
                  <label>
                    <span>Alias hostname</span>
                    <input class="remote-input" bind:value={aliasForm.hostname} placeholder="portal.example.com" />
                  </label>
                  <label>
                    <span>Listener</span>
                    <select class="remote-input" bind:value={aliasForm.listener}>
                      <option value="">Select listener</option>
                      {#each listenerOptions as option}
                        <option value={option.name}>{option.label}</option>
                      {/each}
                    </select>
                  </label>
                  <label>
                    <span>Notes (optional)</span>
                    <input class="remote-input" bind:value={aliasForm.note} placeholder="Expect CNAME to portal host" />
                  </label>
                  <p class="remote-sheet__hint">DNS requirement: <code>CNAME → {portalDisplay}</code></p>
                </div>
                <div class="remote-sheet__actions">
                  <button class="remote-hero__cta" on:click={addAlias} disabled={aliasWorking || !aliasForm.hostname || !aliasForm.listener}>
                    Save alias
                  </button>
                  <button class="remote-hero__action" on:click={() => (aliasDialogOpen = false)} disabled={aliasWorking}>
                    Cancel
                  </button>
                </div>
              </div>
            {/if}
          </section>

          {#if remoteComplete}
            <section class="remote-card">
              <div class="remote-card__header">
                <div>
                  <h2>Verification preflight</h2>
                  <p>Validate DNS and certificates whenever you change domains or credentials.</p>
                </div>
                <button class="remote-link" on:click={runPreflight} disabled={preflightWorking}>
                  {preflightWorking ? 'Running…' : 'Run preflight'}
                </button>
              </div>
              {#if preflightChecks.length}
                <p class="remote-card__meta">Last run {formatDate(preflightTimestamp)}</p>
                <ul class="remote-preflight">
                  {#each preflightChecks as check}
                    <li class="remote-preflight__item">
                      <div>
                        <p class="remote-preflight__title">{check.name}</p>
                        {#if check.detail}
                          <p class="remote-preflight__detail">{check.detail}</p>
                        {/if}
                        {#if check.next_step}
                          <p class="remote-preflight__detail">Next: {check.next_step}</p>
                        {/if}
                      </div>
                      <span class={`remote-badge ${checkStatusClass(check.status)}`}>
                        {check.status.toUpperCase()}
                      </span>
                    </li>
                  {/each}
                </ul>
              {:else}
                <p class="remote-card__body">Preflight hasn’t been run yet.</p>
              {/if}
            </section>
          {/if}
          {/if}
        </div>
      </details>

      <section class="remote-card remote-activity">
        <div class="remote-card__header">
          <div>
            <h2>Activity log</h2>
            <p>Recent remote events from the device and Nexus helper.</p>
          </div>
          <button class="remote-link" on:click={loadEvents}>Refresh</button>
        </div>
        {#if !events.length}
          <p class="remote-card__body">No recent remote activity.</p>
        {:else}
          <ul class="remote-activity__list">
            {#each events as event}
              <li>
                <div>
                  <p class="remote-activity__message">{event.message}</p>
                  {#if event.next_step}
                    <p class="remote-activity__meta">Next: {event.next_step}</p>
                  {/if}
                  {#if event.source}
                    <p class="remote-activity__meta">Source: {event.source}</p>
                  {/if}
                </div>
                <div class="remote-activity__time">
                  <p>{formatDate(event.ts)}</p>
                  {#if event.level}
                    <p class="uppercase">{event.level}</p>
                  {/if}
                </div>
              </li>
            {/each}
          </ul>
        {/if}
      </section>
    {:else}
      <div class="remote-shell remote-shell--empty">
        <p class="remote-shell__copy">Remote status will appear once the service reports in.</p>
      </div>
    {/if}
  {/if}

  {#if guideOpen}
    <div class="remote-guide" role="dialog" aria-modal="true">
      <div class="remote-guide__panel">
        <div class="remote-guide__header">
          <div>
            <h2>Set up Nexus server</h2>
            <p>Provision a public VM, run the installer, then paste the generated secret.</p>
          </div>
          <button class="remote-link" on:click={() => (guideOpen = false)}>Close</button>
        </div>
        <div class="remote-guide__section">
          <p class="remote-guide__label">VM requirements</p>
          <ul>
            {#each nexusGuide.requirements as item}
              <li>{item}</li>
            {/each}
            {#if nexusGuide.notes}
              {#each nexusGuide.notes as note}
                <li>{note}</li>
              {/each}
            {/if}
          </ul>
          {#if nexusGuide.docs_url}
            <p class="remote-guide__hint">
              Reference: <a class="remote-link" target="_blank" rel="noopener" href={nexusGuide.docs_url}>Installer guide</a>
            </p>
          {/if}
        </div>
        <div class="remote-guide__section">
          <p class="remote-guide__label">Install command</p>
          <pre>{nexusGuide.command}</pre>
        </div>
        <div class="remote-guide__form">
          <label>
            <span>Piccolo domain (TLD)</span>
            <input class="remote-input" bind:value={domainTld} placeholder="myname.com" autocomplete="off" autocapitalize="none" spellcheck={false} />
          </label>
          <label>
            <span>Nexus endpoint (wss://)</span>
            <input class="remote-input" bind:value={httpForm.endpoint} placeholder="copy from Nexus installer" autocomplete="off" autocapitalize="none" spellcheck={false} />
          </label>
          <p class="remote-guide__hint">Portal host preview: <code>{portalDisplay}</code></p>
          <label>
            <span>Paste the generated JWT secret</span>
            <input class="remote-input" type="password" bind:value={guideJwtSecret} placeholder="copy-from-installer" autocomplete="new-password" autocapitalize="none" spellcheck={false} />
          </label>
        </div>
        <div class="remote-guide__actions">
          <button class="remote-hero__cta" on:click={verifyNexusEndpoint} disabled={verifyingNexus}>
            {verifyingNexus ? 'Verifying…' : 'Verify & continue'}
          </button>
          <button class="remote-hero__action" on:click={() => (guideOpen = false)}>Cancel</button>
          {#if guideVerifyMessage}
            <p class="remote-guide__hint">{guideVerifyMessage}</p>
          {/if}
          {#if guideVerifiedAt}
            <p class="remote-guide__hint">Verified {formatDate(guideVerifiedAt)}</p>
          {/if}
        </div>
      </div>
    </div>
  {/if}
</div>

<style>
  .remote-shell {
    border: 1px solid rgba(var(--border-rgb) / 0.16);
    border-radius: 24px;
    background: var(--surface-1);
    padding: 32px;
    text-align: center;
    box-shadow: 0 18px 40px rgba(15, 23, 42, 0.08);
    display: flex;
    flex-direction: column;
    gap: 12px;
    align-items: center;
  }
  .remote-shell__headline {
    font-size: 1.25rem;
    font-weight: 600;
    color: var(--text-primary);
  }
  .remote-shell__copy {
    font-size: 0.95rem;
    color: var(--text-muted);
    max-width: 48ch;
  }
  .remote-shell__action {
    border-radius: 999px;
    border: 1px solid var(--border);
    padding: 10px 20px;
    font-size: 0.9rem;
    font-weight: 600;
    background: var(--surface-0);
    color: var(--text-primary);
    cursor: pointer;
  }
  .remote-shell--error {
    border-color: rgba(var(--state-critical-rgb) / 0.36);
    background: rgba(var(--state-critical-rgb) / 0.12);
  }
  .remote-shell--error .remote-shell__headline {
    color: rgb(var(--state-critical-rgb));
  }
  .remote-shell--empty {
    border-style: dashed;
    color: var(--text-muted);
  }
  .remote-hero {
    display: flex;
    flex-direction: column;
    gap: 24px;
    padding: 32px;
  }
  .remote-hero__body {
    display: flex;
    flex-direction: column;
    gap: 12px;
  }
  .remote-hero__eyebrow {
    text-transform: uppercase;
    letter-spacing: 0.18em;
    font-size: 0.7rem;
    font-weight: 600;
    color: var(--text-muted);
  }
  .remote-hero__title {
    font-size: 1.75rem;
    font-weight: 600;
    color: var(--text-primary);
  }
  .remote-hero__copy {
    font-size: 1rem;
    color: var(--text-muted);
    max-width: 52ch;
  }
  .remote-hero__warnings {
    margin: 8px 0 0;
    padding-left: 18px;
    list-style: disc;
    color: inherit;
    font-size: 0.9rem;
  }
  .remote-hero__meta {
    font-size: 0.75rem;
    color: inherit;
    opacity: 0.8;
  }
  .remote-hero__actions {
    display: flex;
    flex-direction: column;
    gap: 12px;
    align-items: stretch;
  }
  .remote-hero__cta,
  .remote-hero__action,
  .remote-hero__danger {
    border-radius: 999px;
    padding: 12px 20px;
    font-size: 0.9rem;
    font-weight: 600;
    cursor: pointer;
    transition: transform var(--transition-duration) var(--transition-easing), box-shadow var(--transition-duration) var(--transition-easing);
  }
  .remote-hero__cta {
    background: var(--accent);
    color: var(--text-inverse);
    border: 1px solid var(--accent-emphasis);
    box-shadow: 0 12px 24px rgba(var(--accent-rgb) / 0.32);
  }
  .remote-hero__cta:hover {
    transform: translateY(-1px);
  }
  .remote-hero__action {
    background: var(--surface-1);
    color: var(--text-primary);
    border: 1px solid rgba(var(--border-rgb) / 0.2);
  }
  .remote-hero__links {
    display: flex;
    flex-direction: column;
    gap: 8px;
  }
  .remote-hero__link {
    border: none;
    background: none;
    padding: 0;
    font-size: 0.85rem;
    font-weight: 600;
    color: var(--accent-emphasis);
    text-decoration: underline;
    cursor: pointer;
    align-self: flex-start;
  }
  .remote-hero__danger {
    background: rgba(var(--state-critical-rgb) / 0.1);
    color: rgb(var(--state-critical-rgb));
    border: 1px solid rgba(var(--state-critical-rgb) / 0.4);
  }
  .remote-hero__checklist {
    margin-top: 12px;
    display: flex;
    flex-direction: column;
    gap: 10px;
  }
  .remote-hero__check {
    display: flex;
    gap: 10px;
    align-items: flex-start;
    padding: 10px 12px;
    border-radius: 16px;
    border: 1px dashed rgba(var(--border-rgb) / 0.2);
    background: rgba(var(--border-rgb) / 0.04);
  }
  .remote-hero__check--done {
    border-style: solid;
    border-color: rgba(var(--state-ok-rgb) / 0.4);
    background: rgba(var(--state-ok-rgb) / 0.1);
  }
  .remote-hero__check-dot {
    height: 12px;
    width: 12px;
    border-radius: 999px;
    margin-top: 6px;
    border: 2px solid rgba(var(--border-rgb) / 0.3);
    display: inline-flex;
  }
  .remote-hero__check--done .remote-hero__check-dot {
    border-color: rgba(var(--state-ok-rgb) / 0.8);
    background: rgba(var(--state-ok-rgb) / 0.8);
  }
  .remote-hero__check-label {
    font-size: 0.9rem;
    font-weight: 600;
    color: var(--text-primary);
  }
  .remote-hero__check-detail {
    font-size: 0.8rem;
    color: var(--text-muted);
  }
  .remote-link {
    border: none;
    background: none;
    color: var(--accent-emphasis);
    font-size: 0.85rem;
    font-weight: 600;
    text-decoration: underline;
    cursor: pointer;
  }
  .remote-card {
    border: 1px solid rgba(var(--border-rgb) / 0.16);
    border-radius: 24px;
    background: var(--surface-1);
    padding: 24px 28px;
    box-shadow: 0 12px 32px rgba(15, 23, 42, 0.08);
    display: flex;
    flex-direction: column;
    gap: 16px;
  }
  .remote-card__header {
    display: flex;
    flex-direction: column;
    gap: 4px;
  }
  @media (min-width: 768px) {
    .remote-card__header {
      flex-direction: row;
      align-items: center;
      justify-content: space-between;
      gap: 16px;
    }
  }
  .remote-card__header h2 {
    font-size: 1.1rem;
    font-weight: 600;
    color: var(--text-primary);
  }
  .remote-card__header p {
    font-size: 0.9rem;
    color: var(--text-muted);
  }
  .remote-card__body {
    font-size: 0.9rem;
    color: var(--text-muted);
  }
  .remote-card__meta {
    font-size: 0.75rem;
    color: var(--text-muted);
  }
  .remote-card--muted {
    background: rgba(var(--border-rgb) / 0.04);
    border-style: dashed;
  }
  .remote-detail-grid {
    display: grid;
    gap: 16px;
  }
  @media (min-width: 768px) {
    .remote-detail-grid {
      grid-template-columns: repeat(2, minmax(0, 1fr));
    }
  }
  @media (min-width: 1024px) {
    .remote-detail-grid {
      grid-template-columns: repeat(3, minmax(0, 1fr));
    }
  }
  .remote-detail-card {
    border: 1px solid rgba(var(--border-rgb) / 0.12);
    border-radius: 18px;
    background: var(--surface-1);
    padding: 16px 18px;
    display: flex;
    flex-direction: column;
    gap: 6px;
  }
  .remote-detail-card__label {
    font-size: 0.75rem;
    text-transform: uppercase;
    letter-spacing: 0.18em;
    color: var(--text-muted);
  }
  .remote-detail-card__value {
    font-size: 0.95rem;
    font-weight: 600;
    color: var(--text-primary);
    word-break: break-word;
  }
  .remote-advanced {
    border: 1px solid rgba(var(--border-rgb) / 0.16);
    border-radius: 24px;
    background: var(--surface-1);
    padding: 0 0 24px;
    overflow: hidden;
  }
  .remote-advanced__summary {
    list-style: none;
    cursor: pointer;
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 12px;
    padding: 20px 28px;
    font-size: 0.95rem;
    font-weight: 600;
    color: var(--text-primary);
    background: rgba(var(--border-rgb) / 0.04);
  }
  .remote-advanced__summary::-webkit-details-marker {
    display: none;
  }
  .remote-advanced[open] .remote-advanced__summary {
    border-bottom: 1px solid rgba(var(--border-rgb) / 0.12);
  }
  .remote-advanced__content {
    padding: 24px 28px 0;
  }
  .remote-card__list {
    display: flex;
    flex-direction: column;
    gap: 14px;
  }
  .remote-cert {
    border: 1px solid rgba(var(--border-rgb) / 0.12);
    border-radius: 16px;
    padding: 16px;
    display: flex;
    flex-direction: column;
    gap: 12px;
    background: var(--surface-0);
  }
  .remote-cert__domains p {
    font-weight: 600;
    color: var(--text-primary);
    word-break: break-word;
  }
  .remote-cert__meta {
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: 8px 16px;
    font-size: 0.8rem;
    color: var(--text-muted);
  }
  .remote-cert__meta dt {
    text-transform: uppercase;
    letter-spacing: 0.12em;
    font-weight: 600;
  }
  .remote-cert__actions {
    display: flex;
    flex-direction: column;
    gap: 6px;
    font-size: 0.8rem;
  }
  .remote-cert__status {
    font-size: 0.75rem;
    font-weight: 600;
    letter-spacing: 0.14em;
  }
  .remote-cert__note {
    color: var(--text-muted);
  }
  .remote-table {
    border: 1px solid rgba(var(--border-rgb) / 0.12);
    border-radius: 16px;
    overflow-x: auto;
  }
  .remote-table table {
    width: 100%;
    border-collapse: collapse;
    min-width: 560px;
  }
  .remote-table th,
  .remote-table td {
    padding: 12px 16px;
    text-align: left;
    font-size: 0.85rem;
  }
  .remote-table thead {
    background: rgba(var(--border-rgb) / 0.06);
    text-transform: uppercase;
    letter-spacing: 0.12em;
    font-size: 0.75rem;
  }
  .remote-table tbody tr + tr {
    border-top: 1px solid rgba(var(--border-rgb) / 0.08);
  }
  .remote-table__primary {
    font-weight: 600;
    color: var(--text-primary);
  }
  .remote-table__secondary {
    font-size: 0.75rem;
    color: var(--text-muted);
  }
  .remote-badge {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    padding: 4px 10px;
    border-radius: 999px;
    font-size: 0.7rem;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.12em;
  }
  .remote-sheet {
    margin-top: 16px;
    border: 1px solid rgba(var(--border-rgb) / 0.16);
    border-radius: 16px;
    background: var(--surface-0);
    padding: 18px;
    display: flex;
    flex-direction: column;
    gap: 14px;
  }
  .remote-sheet__header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    font-size: 0.95rem;
    font-weight: 600;
    color: var(--text-primary);
  }
  .remote-sheet__body {
    display: flex;
    flex-direction: column;
    gap: 12px;
    font-size: 0.85rem;
    color: var(--text-primary);
  }
  .remote-sheet__body label {
    display: flex;
    flex-direction: column;
    gap: 4px;
  }
  .remote-input {
    border: 1px solid rgba(var(--border-rgb) / 0.18);
    border-radius: 14px;
    padding: 10px 14px;
    font-size: 0.9rem;
    background: var(--surface-1);
  }
  .remote-sheet__hint {
    font-size: 0.75rem;
    color: var(--text-muted);
  }
  .remote-sheet__actions {
    display: flex;
    flex-wrap: wrap;
    gap: 12px;
  }
  .remote-preflight {
    display: flex;
    flex-direction: column;
    gap: 12px;
  }
  .remote-preflight__item {
    border: 1px solid rgba(var(--border-rgb) / 0.12);
    border-radius: 14px;
    padding: 14px;
    display: flex;
    justify-content: space-between;
    gap: 16px;
    background: var(--surface-0);
  }
  .remote-preflight__title {
    font-size: 0.95rem;
    font-weight: 600;
    color: var(--text-primary);
  }
  .remote-preflight__detail {
    font-size: 0.8rem;
    color: var(--text-muted);
  }
  .remote-activity__list {
    display: flex;
    flex-direction: column;
    gap: 12px;
  }
  .remote-activity__list li {
    border: 1px solid rgba(var(--border-rgb) / 0.12);
    border-radius: 16px;
    padding: 16px;
    display: flex;
    justify-content: space-between;
    gap: 16px;
    background: var(--surface-0);
  }
  .remote-activity__message {
    font-size: 0.95rem;
    font-weight: 600;
    color: var(--text-primary);
  }
  .remote-activity__meta {
    font-size: 0.75rem;
    color: var(--text-muted);
  }
  .remote-activity__time {
    text-align: right;
    font-size: 0.75rem;
    color: var(--text-muted);
  }
  .remote-guide {
    position: fixed;
    inset: 0;
    background: rgba(15, 23, 42, 0.6);
    display: flex;
    align-items: center;
    justify-content: center;
    padding: 24px;
    z-index: 90;
  }
  .remote-guide__panel {
    background: var(--surface-1);
    border-radius: 24px;
    border: 1px solid rgba(var(--border-rgb) / 0.2);
    width: min(720px, 100%);
    max-height: 90vh;
    overflow-y: auto;
    display: flex;
    flex-direction: column;
    gap: 20px;
    padding: 28px;
    box-shadow: 0 28px 64px rgba(15, 23, 42, 0.28);
  }
  .remote-guide__header {
    display: flex;
    justify-content: space-between;
    gap: 16px;
  }
  .remote-guide__header h2 {
    font-size: 1.4rem;
    font-weight: 600;
    color: var(--text-primary);
  }
  .remote-guide__header p {
    font-size: 0.9rem;
    color: var(--text-muted);
  }
  .remote-guide__section {
    display: flex;
    flex-direction: column;
    gap: 10px;
    font-size: 0.85rem;
    color: var(--text-muted);
  }
  .remote-guide__section ul {
    list-style: disc;
    padding-left: 20px;
    display: flex;
    flex-direction: column;
    gap: 4px;
  }
  .remote-guide__label {
    font-size: 0.78rem;
    text-transform: uppercase;
    letter-spacing: 0.14em;
    font-weight: 600;
    color: var(--text-muted);
  }
  .remote-guide__section pre {
    background: #0f172a;
    color: #f8fafc;
    font-size: 0.75rem;
    padding: 12px;
    border-radius: 12px;
    overflow-x: auto;
  }
  .remote-guide__form {
    display: flex;
    flex-direction: column;
    gap: 12px;
    font-size: 0.85rem;
    color: var(--text-primary);
  }
  .remote-guide__form label {
    display: flex;
    flex-direction: column;
    gap: 4px;
  }
  .remote-guide__actions {
    display: flex;
    flex-wrap: wrap;
    gap: 12px;
    align-items: center;
  }
  .remote-guide__hint {
    font-size: 0.75rem;
    color: var(--text-muted);
  }
  @media (min-width: 1024px) {
    .remote-hero {
      flex-direction: row;
      justify-content: space-between;
      align-items: flex-start;
    }
    .remote-hero__actions {
      max-width: 260px;
    }
  }
  @media (max-width: 640px) {
    .remote-hero {
      padding: 24px;
    }
    .remote-card,
    .remote-advanced__content {
      padding-left: 20px;
      padding-right: 20px;
    }
    .remote-guide__panel {
      padding: 20px;
    }
  }
</style>
