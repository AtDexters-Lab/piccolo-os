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
      const tld = domainTld.trim() || undefined;
      const portalHost = portalHostname.trim() || undefined;

      if (!endpoint || !deviceSecret) {
        toast('Provide the Nexus endpoint and JWT signing secret.', 'error');
        return;
      }
      if (!tld) {
        toast('Enter the Piccolo domain (TLD).', 'error');
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
</script>

<svelte:window on:keydown={(event) => {
  if (event.key === 'Escape' && aliasDialogOpen) {
    aliasDialogOpen = false;
  }
}} />

<div class="space-y-6">
  <div class="flex items-center justify-between">
    <div>
      <h1 class="text-2xl font-semibold text-slate-900">Remote Access</h1>
      <p class="text-sm text-slate-500">Configure Nexus connectivity, DNS solvers, and monitor certificates.</p>
    </div>
    <div class="hidden md:flex gap-2">
      <button class="px-3 py-1.5 text-sm border rounded hover:bg-slate-50" on:click={loadAll} disabled={loading}>Refresh</button>
      <button class="px-3 py-1.5 text-sm border rounded hover:bg-slate-50" on:click={runPreflight} disabled={preflightWorking || loading}>Run preflight</button>
    </div>
  </div>

  {#if loading}
    <div class="p-10 border border-slate-200 rounded bg-white text-center text-slate-600">Loading remote status…</div>
  {:else if statusError && !statusPayload}
    <div class="p-6 border border-rose-200 bg-rose-50 rounded text-rose-900">
      <p class="font-medium">Failed to load remote status</p>
      <p class="text-sm mt-1">{statusError}</p>
      <button class="mt-4 px-3 py-1.5 text-sm border rounded bg-white hover:bg-slate-50" on:click={loadAll}>Retry</button>
    </div>
  {:else}
    {#if statusPayload}
      <section class={`rounded border ${stateStyles[state].border} ${stateStyles[state].bg} ${stateStyles[state].text} p-4 md:p-5`}
        aria-live="polite">
        <div class="flex flex-col md:flex-row md:items-center md:justify-between gap-4">
          <div>
            <p class="uppercase tracking-wide text-xs font-semibold">{stateTitle(state)}</p>
            <p class="text-base md:text-lg font-medium mt-1">{stateDescription(statusPayload, state)}</p>
            {#if statusPayload.warnings && statusPayload.warnings.length > 0}
              <ul class="mt-3 space-y-1 text-sm">
                {#each statusPayload.warnings as warning}
                  <li>• {warning}</li>
                {/each}
              </ul>
            {/if}
            {#if guideVerifiedAt}
              <p class="text-xs mt-3">Nexus helper verified {formatDate(guideVerifiedAt)}.</p>
            {/if}
          </div>
          <div class="flex flex-wrap gap-2">
            {#if state !== 'disabled'}
              <button class="px-3 py-1.5 text-sm border rounded bg-white/80 hover:bg-white" on:click={disableRemote}
                disabled={saving}>Disable remote</button>
            {/if}
          </div>
        </div>
      </section>
    {/if}

    <div class="grid gap-4 lg:grid-cols-2">
      <div class="space-y-4">
        <section class="bg-white border border-slate-200 rounded p-4">
          <div class="flex items-center justify-between">
            <h2 class="text-lg font-medium text-slate-900">Connection overview</h2>
            <button class="text-sm text-blue-600 hover:underline" on:click={rotateRemote} disabled={saving}>Rotate credentials</button>
          </div>
          <dl class="mt-3 grid grid-cols-1 md:grid-cols-2 gap-x-6 gap-y-3 text-sm text-slate-600">
            <div>
              <dt class="text-xs uppercase tracking-wide text-slate-500">Solver</dt>
              <dd class="font-medium text-slate-900">{statusPayload?.solver ? statusPayload.solver.toUpperCase() : 'Not set'}</dd>
            </div>
            <div>
              <dt class="text-xs uppercase tracking-wide text-slate-500">Nexus endpoint</dt>
              <dd class="break-words">{statusPayload?.endpoint || '-'}</dd>
            </div>
            <div>
              <dt class="text-xs uppercase tracking-wide text-slate-500">Piccolo domain</dt>
              <dd>{statusPayload?.tld || '-'}</dd>
            </div>
            <div>
              <dt class="text-xs uppercase tracking-wide text-slate-500">Portal hostname</dt>
              <dd>{statusPayload?.portal_hostname || statusPayload?.hostname || '-'}</dd>
            </div>
            <div>
              <dt class="text-xs uppercase tracking-wide text-slate-500">Last handshake</dt>
              <dd>{formatDate(statusPayload?.last_handshake)}</dd>
            </div>
            <div>
              <dt class="text-xs uppercase tracking-wide text-slate-500">Latency</dt>
              <dd>{typeof statusPayload?.latency_ms === 'number' ? `${statusPayload.latency_ms} ms` : '-'}</dd>
            </div>
            <div>
              <dt class="text-xs uppercase tracking-wide text-slate-500">Next renewal</dt>
              <dd>{formatDate(statusPayload?.next_renewal)}</dd>
            </div>
            <div>
              <dt class="text-xs uppercase tracking-wide text-slate-500">Certificate issuer</dt>
              <dd>{statusPayload?.issuer || '-'}</dd>
            </div>
            <div>
              <dt class="text-xs uppercase tracking-wide text-slate-500">Certificate expires</dt>
              <dd>{formatDate(statusPayload?.expires_at)}</dd>
            </div>
          </dl>
        </section>

        <section class="bg-white border border-slate-200 rounded p-4">
          <h2 class="text-lg font-medium text-slate-900">Certificate inventory</h2>
          {#if !certificates.length}
            <p class="text-sm text-slate-500 mt-3">No certificates issued yet.</p>
          {:else}
            <div class="mt-3 space-y-3">
              {#each certificates as cert}
                <div class="border border-slate-100 rounded-md p-3 space-y-3">
                  <div class="flex flex-col gap-3 md:flex-row md:items-start md:justify-between">
                    <div class="space-y-2">
                      <div class="space-y-1">
                        {#each cert.domains as domain}
                          <div class="text-slate-900 font-medium break-words">{domain}</div>
                        {/each}
                      </div>
                      <dl class="grid grid-cols-2 gap-x-6 gap-y-1 text-xs text-slate-600">
                        <dt>Solver</dt><dd class="uppercase text-slate-800">{cert.solver || '-'}</dd>
                        <dt>Issued</dt><dd>{formatDate(cert.issued_at)}</dd>
                        <dt>Expires</dt><dd>{formatDate(cert.expires_at)}</dd>
                        <dt>Next renewal</dt><dd>{formatDate(cert.next_renewal)}</dd>
                      </dl>
                    </div>
                    <div class="space-y-2 text-sm md:text-right md:w-48">
                      <p class={`text-xs font-semibold ${certificateStatusClass(cert.status)}`}>{cert.status ? cert.status.toUpperCase() : 'UNKNOWN'}</p>
                      {#if cert.failure_reason}
                        <p class="text-xs text-slate-500 whitespace-pre-wrap break-words">{cert.failure_reason}</p>
                      {/if}
                      {#if cert.id}
                        <button class="px-2 py-1 text-xs border rounded hover:bg-slate-50 disabled:opacity-50 disabled:cursor-not-allowed" on:click={() => renewCert(cert)} disabled={renewing[cert.id] || cert.status === 'pending'}>{renewing[cert.id] ? 'Queuing…' : 'Retry issuance'}</button>
                      {/if}
                    </div>
                  </div>
                </div>
              {/each}
            </div>
          {/if}
        </section>
      </div>

      <div class="space-y-4">
        <section class="bg-white border border-slate-200 rounded p-4">
          <div class="flex flex-col md:flex-row md:items-center md:justify-between gap-2">
            <div>
              <h2 class="text-lg font-medium text-slate-900">Configure remote access</h2>
              <p class="text-sm text-slate-500">Update solver, DNS, and Nexus settings.</p>
            </div>
            <button class="text-sm text-blue-600 hover:underline" on:click={() => {
              guideVerifyMessage = '';
              guideOpen = true;
            }}>
              Set up Nexus server
            </button>
          </div>

          <div class="mt-5 space-y-6">
            <div class="space-y-3">
              <h3 class="text-xs font-semibold uppercase tracking-wide text-slate-500">1. Nexus connection</h3>
              <p class="text-sm text-slate-500">Use the endpoint and JWT secret printed by the Nexus installer.</p>
              <label class="block text-sm text-slate-700">
                Nexus endpoint
                <input class="mt-1 w-full border rounded px-2 py-1 text-sm" bind:value={httpForm.endpoint}
                  placeholder="copy from Nexus installer" autocomplete="off" autocapitalize="none" spellcheck={false} />
              </label>
              <label class="block text-sm text-slate-700">
                JWT signing secret
                <input class="mt-1 w-full border rounded px-2 py-1 text-sm" type="password" bind:value={httpForm.jwtSecret}
                  placeholder="copy from Nexus installer" autocomplete="new-password" autocapitalize="none" spellcheck={false} />
              </label>
            </div>

            <div class="space-y-3">
              <h3 class="text-xs font-semibold uppercase tracking-wide text-slate-500">2. Domain setup</h3>
              <p class="text-sm text-slate-500">Pick the Piccolo domain that your portal and app listeners will use.</p>
              <label class="block text-sm text-slate-700">
                Piccolo domain (TLD)
                <input class="mt-1 w-full border rounded px-2 py-1 text-sm" bind:value={domainTld}
                  placeholder="myname.com" autocomplete="off" autocapitalize="none" spellcheck={false} />
              </label>
              <div class="space-y-2 text-sm text-slate-700">
                <label class="flex items-start gap-2">
                  <input type="radio" class="mt-1" value="root" bind:group={portalMode} />
                  <span>Serve the portal at <code>{trimmedTld || 'your-domain.com'}</code></span>
                </label>
                <div class="flex flex-col gap-2">
                  <label class="flex items-start gap-2">
                    <input type="radio" class="mt-1" value="subdomain" bind:group={portalMode} />
                    <span>Use a dedicated portal subdomain</span>
                  </label>
                  <div class="ml-6 flex items-center gap-2">
                    <input class="w-32 border rounded px-2 py-1 text-sm" bind:value={portalPrefix}
                      placeholder="portal" autocomplete="off" autocapitalize="none" spellcheck={false}
                      disabled={portalMode !== 'subdomain'} />
                    <span class="text-xs text-slate-500">Full host: <code>{portalDisplay}</code></span>
                  </div>
                </div>
              </div>
              <div class="text-xs text-slate-600 bg-slate-50 border border-slate-200 rounded p-3">
                <p class="font-medium text-slate-700">DNS checklist</p>
                <ul class="mt-2 space-y-1 list-disc ml-4">
                  <li>CNAME <code>{wildcardDisplay}</code> → <code>{nexusHostHint}</code></li>
                  {#if portalMode === 'subdomain'}
                    <li><code>{portalDisplay}</code> is covered by the wildcard above; no extra record needed.</li>
                  {:else}
                    <li>Point <code>{portalDisplay}</code> to <code>{nexusHostHint}</code> using an ALIAS/ANAME (or A/AAAA to the Nexus IP).</li>
                  {/if}
                  <li>Expected listener: <code>{listenerPreviewUrl}</code></li>
                </ul>
              </div>
            </div>

            <div class="space-y-3">
              <h3 class="text-xs font-semibold uppercase tracking-wide text-slate-500">3. Certificate solver</h3>
              <div class="inline-flex border border-slate-200 rounded overflow-hidden text-sm">
                <button class={`px-3 py-1.5 ${solver === 'http-01' ? 'bg-slate-900 text-white' : 'bg-white text-slate-600'}`}
                  on:click={() => (solver = 'http-01')}>
                  HTTP-01
                </button>
                <button class={`px-3 py-1.5 ${solver === 'dns-01' ? 'bg-slate-900 text-white' : 'bg-white text-slate-600'}`}
                  on:click={() => (solver = 'dns-01')}>
                  DNS-01
                </button>
              </div>
              {#if solver === 'http-01'}
                <div class="text-sm text-slate-600 bg-slate-50 border border-slate-200 rounded p-3">
                  <p>Piccolo solves HTTP-01 challenges through Nexus. Ensure ports 80 and 443 reach {nexusHostHint}.</p>
                </div>
              {:else}
                <div class="space-y-3">
                  <label class="block text-sm text-slate-700">
                    DNS provider
                    <select class="mt-1 w-full border rounded px-2 py-1 text-sm" bind:value={dnsForm.provider}>
                      {#each providers as provider}
                        <option value={provider.id}>{provider.name}</option>
                      {/each}
                    </select>
                  </label>
                  {#if dnsProviderFields.length === 0}
                    <p class="text-xs text-slate-500">This provider does not require API credentials.</p>
                  {:else}
                    {#each dnsProviderFields as field}
                      <label class="block text-sm text-slate-700">
                        {field.label}
                        <input class="mt-1 w-full border rounded px-2 py-1 text-sm"
                          type={field.secret ? 'password' : 'text'}
                          value={dnsForm.credentials[field.id] ?? ''}
                          on:input={(event) => handleCredentialInput(field.id, event)}
                          placeholder={field.placeholder || ''} />
                        {#if field.description}
                          <span class="text-xs text-slate-500">{field.description}</span>
                        {/if}
                      </label>
                    {/each}
                  {/if}
                  <div class="text-xs text-slate-600 bg-slate-50 border border-slate-200 rounded p-3">
                    Requesting wildcard certificate for <code>{wildcardDisplay}</code> and portal host <code>{portalDisplay}</code>.
                  </div>
                </div>
              {/if}
            </div>

            <div class="flex flex-wrap gap-2">
              <button class="px-3 py-1.5 text-sm border rounded bg-slate-900 text-white hover:bg-slate-800"
                on:click={configureRemote} disabled={saving}>
                Save &amp; run preflight
              </button>
              <button class="px-3 py-1.5 text-sm border rounded hover:bg-slate-50" on:click={loadStatus} disabled={saving}>
                Reset form
              </button>
            </div>
          </div>
        </section>

        <section class="bg-white border border-slate-200 rounded p-4">
          <div class="flex items-center justify-between">
            <div>
              <h2 class="text-lg font-medium text-slate-900">Alias domains</h2>
              <p class="text-sm text-slate-500">Map custom hostnames to listener endpoints.</p>
            </div>
            <button class="text-sm text-blue-600 hover:underline" on:click={() => (aliasDialogOpen = true)}>
              Add alias
            </button>
          </div>
          {#if !aliases.length}
            <p class="text-sm text-slate-500 mt-3">No alias domains configured.</p>
          {:else}
            <div class="mt-3 overflow-x-auto">
              <table class="min-w-full text-sm text-left">
                <thead class="text-xs uppercase tracking-wide text-slate-500">
                  <tr>
                    <th class="py-2 pr-4">Alias</th>
                    <th class="py-2 pr-4">Listener</th>
                    <th class="py-2 pr-4">Status</th>
                    <th class="py-2 pr-4">Last check</th>
                    <th class="py-2 pr-4">Actions</th>
                  </tr>
                </thead>
                <tbody class="divide-y divide-slate-100">
                  {#each aliases as alias}
                    <tr>
                      <td class="py-2 pr-4">
                        <div class="font-medium text-slate-900">{alias.hostname}</div>
                        {#if alias.message}
                          <div class="text-xs text-slate-500">{alias.message}</div>
                        {/if}
                      </td>
                      <td class="py-2 pr-4">{alias.listener}</td>
                      <td class="py-2 pr-4">
                        <span class={`px-2 py-0.5 rounded-full text-xs font-semibold ${aliasStatusClass(alias.status)}`}>
                          {alias.status ? alias.status.toUpperCase() : 'PENDING'}
                        </span>
                      </td>
                      <td class="py-2 pr-4">{formatDate(alias.last_checked)}</td>
                      <td class="py-2 pr-4">
                        <button class="text-xs text-blue-600 hover:underline"
                          on:click={() => removeAlias(alias)} disabled={aliasWorking || !alias.id}>
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
            <div class="mt-4 border border-slate-200 bg-slate-50 rounded p-4 space-y-3">
              <div class="flex items-center justify-between">
                <p class="font-medium text-sm text-slate-900">Add alias</p>
                <button class="text-xs text-slate-500 hover:underline" on:click={() => (aliasDialogOpen = false)}>Cancel</button>
              </div>
              <label class="block text-sm text-slate-700">
                Alias hostname
                <input class="mt-1 w-full border rounded px-2 py-1 text-sm" bind:value={aliasForm.hostname}
                  placeholder="forum.mybusiness.com" />
              </label>
              <label class="block text-sm text-slate-700">
                Listener
                <select class="mt-1 w-full border rounded px-2 py-1 text-sm" bind:value={aliasForm.listener}>
                  <option value="">Select listener</option>
                  {#each listenerOptions as option}
                    <option value={option.name}>{option.label}</option>
                  {/each}
                </select>
              </label>
              <label class="block text-sm text-slate-700">
                Notes (optional)
                <input class="mt-1 w-full border rounded px-2 py-1 text-sm" bind:value={aliasForm.note}
                  placeholder="Expect CNAME to portal hostname" />
              </label>
              <div class="flex items-center gap-2 text-xs text-slate-500">
                <span>DNS requirement:</span>
                <code class="bg-slate-200 rounded px-2 py-0.5">CNAME → {portalDisplay}</code>
              </div>
              <div class="flex gap-2">
                <button class="px-3 py-1.5 text-sm border rounded bg-slate-900 text-white hover:bg-slate-800"
                  on:click={addAlias} disabled={aliasWorking || !aliasForm.hostname || !aliasForm.listener}>
                  Save alias
                </button>
                <button class="px-3 py-1.5 text-sm border rounded hover:bg-slate-50"
                  on:click={() => (aliasDialogOpen = false)} disabled={aliasWorking}>
                  Cancel
                </button>
              </div>
            </div>
          {/if}
        </section>
      </div>
    </div>

    {#if preflightChecks.length}
      <section class="bg-white border border-slate-200 rounded p-4">
        <div class="flex items-center justify-between">
          <h2 class="text-lg font-medium text-slate-900">Preflight results</h2>
          <p class="text-xs text-slate-500">Last run {formatDate(preflightTimestamp)}</p>
        </div>
        <ul class="mt-3 space-y-2 text-sm">
          {#each preflightChecks as check}
            <li class="border border-slate-100 rounded p-3">
              <div class="flex items-center justify-between">
                <span class="font-medium text-slate-900">{check.name}</span>
                <span class={`px-2 py-0.5 rounded-full text-xs font-semibold ${checkStatusClass(check.status)}`}>
                  {check.status.toUpperCase()}
                </span>
              </div>
              {#if check.detail}
                <p class="mt-2 text-slate-600">{check.detail}</p>
              {/if}
              {#if check.next_step}
                <p class="mt-1 text-xs text-slate-500">Next: {check.next_step}</p>
              {/if}
            </li>
          {/each}
        </ul>
      </section>
    {/if}

    <section class="bg-white border border-slate-200 rounded p-4">
      <div class="flex items-center justify-between">
        <h2 class="text-lg font-medium text-slate-900">Activity log</h2>
        <button class="text-sm text-blue-600 hover:underline" on:click={loadEvents}>Refresh</button>
      </div>
      {#if !events.length}
        <p class="text-sm text-slate-500 mt-3">No recent remote activity.</p>
      {:else}
        <ul class="mt-3 space-y-3 text-sm">
          {#each events as event}
            <li class="border border-slate-100 rounded p-3">
              <div class="flex items-start justify-between">
                <div>
                  <p class="font-medium text-slate-900">{event.message}</p>
                  {#if event.next_step}
                    <p class="text-xs text-slate-500 mt-1">Next: {event.next_step}</p>
                  {/if}
                  {#if event.source}
                    <p class="text-xs text-slate-500 mt-1">Source: {event.source}</p>
                  {/if}
                </div>
                <div class="text-right text-xs text-slate-500">
                  <p>{formatDate(event.ts)}</p>
                  {#if event.level}
                    <p class="uppercase mt-1">{event.level}</p>
                  {/if}
                </div>
              </div>
            </li>
          {/each}
        </ul>
      {/if}
    </section>
    {#if guideOpen}
      <div class="fixed inset-0 z-50 flex items-center justify-center bg-slate-900/60 px-4 !mt-0" role="dialog"
        aria-modal="true">
        <div class="bg-white rounded-lg shadow-xl max-w-2xl w-full p-6 space-y-4">
          <div class="flex items-start justify-between gap-4">
            <div>
              <h2 class="text-xl font-semibold text-slate-900">Set up Nexus server</h2>
              <p class="text-sm text-slate-500">Provision a public VM, run the installer, then paste the generated secret.</p>
            </div>
            <button class="text-sm text-slate-500 hover:text-slate-700" on:click={() => (guideOpen = false)}>Close</button>
          </div>
          <div>
            <p class="text-xs font-medium text-slate-700">VM requirements</p>
            <ul class="mt-2 list-disc ml-5 space-y-1 text-sm text-slate-600">
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
              <p class="mt-3 text-xs text-slate-500">
                Reference: <a class="text-blue-600 underline" target="_blank" rel="noopener" href={nexusGuide.docs_url}>Installer guide</a>
              </p>
            {/if}
          </div>
          <div>
            <p class="text-xs font-medium text-slate-700">Install command</p>
            <pre class="mt-2 bg-slate-900 text-slate-100 text-xs p-3 rounded overflow-x-auto">{nexusGuide.command}</pre>
          </div>
          <div class="grid gap-3 md:grid-cols-2">
            <label class="text-xs text-slate-600">
              Piccolo domain (TLD)
              <input class="mt-1 w-full border rounded px-2 py-1 text-sm" bind:value={domainTld}
                placeholder="myname.com" autocomplete="off" autocapitalize="none" spellcheck={false} />
            </label>
            <label class="text-xs text-slate-600">
              Nexus endpoint (wss://)
              <input class="mt-1 w-full border rounded px-2 py-1 text-sm" bind:value={httpForm.endpoint}
                placeholder="copy from Nexus installer" autocomplete="off" autocapitalize="none" spellcheck={false} />
            </label>
          </div>
          <p class="text-xs text-slate-500">Portal host preview: <code>{portalDisplay}</code></p>
          <label class="text-xs text-slate-600">
            Paste the generated JWT secret
            <input class="mt-1 w-full border rounded px-2 py-1 text-sm" type="password" bind:value={guideJwtSecret}
              placeholder="copy-from-installer" autocomplete="new-password" autocapitalize="none" spellcheck={false} />
          </label>
          <div class="flex flex-wrap items-center gap-3">
            <button class="px-3 py-1.5 text-sm border rounded bg-slate-900 text-white hover:bg-slate-800"
              on:click={verifyNexusEndpoint} disabled={verifyingNexus}>
              {verifyingNexus ? 'Verifying…' : 'Verify & continue'}
            </button>
            <button class="px-3 py-1.5 text-sm border rounded hover:bg-slate-50" on:click={() => (guideOpen = false)}>
              Cancel
            </button>
            {#if guideVerifyMessage}
              <p class="text-xs text-slate-600">{guideVerifyMessage}</p>
            {/if}
            {#if guideVerifiedAt}
              <p class="text-xs text-slate-500">Verified {formatDate(guideVerifiedAt)}</p>
            {/if}
          </div>
        </div>
      </div>
    {/if}
  {/if}
</div>
