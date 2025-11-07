<script lang="ts">
  import { createEventDispatcher, onDestroy, onMount } from 'svelte';
  import { apiProd, type ErrorResponse } from '@api/client';
  import { toast } from '@stores/ui';

  type RemoteState = 'disabled' | 'provisioning' | 'preflight_required' | 'active' | 'warning' | 'error';
  type Solver = 'http-01' | 'dns-01';

  interface RemoteStatusPayload {
    enabled?: boolean;
    state?: RemoteState;
    solver?: Solver;
    endpoint?: string | null;
    hostname?: string | null;
    portal_hostname?: string | null;
    tld?: string | null;
    warnings?: string[];
    listeners?: { name: string; remote_host?: string | null }[];
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

  interface PreflightCheck {
    name: string;
    status: 'pass' | 'fail' | 'warn' | 'pending';
    detail?: string | null;
    next_step?: string | null;
  }

  const stepOrder = ['nexus', 'domain', 'solver'] as const;
  type StepKey = (typeof stepOrder)[number];

  const stepCopy: Record<StepKey, { title: string; description: string }> = {
    nexus: {
      title: 'Connect Nexus helper',
      description: 'Paste the endpoint and JWT secret from the Nexus installer.'
    },
    domain: {
      title: 'Set portal domain',
      description: 'Choose the Piccolo domain that exposes the remote portal.'
    },
    solver: {
      title: 'Select certificate solver',
      description: 'Pick HTTP-01 or DNS-01 and run preflight to validate the setup.'
    }
  };

  const dispatch = createEventDispatcher<{ close: void; updated: void }>();

  let loading = true;
  let saving = false;
  let preflightWorking = false;
  let verifyingHelper = false;
  let error: string | null = null;
  let statusPayload: RemoteStatusPayload | null = null;

  let activeStep: StepKey = 'nexus';
  let solver: Solver = 'http-01';

  let httpForm = { endpoint: '', jwtSecret: '' };
  let httpFormError: string | null = null;
  let domainTld = '';
  let domainError: string | null = null;
  let portalMode: 'root' | 'subdomain' = 'subdomain';
  let portalPrefix = 'portal';

  let dnsProviders: DnsProvider[] = [];
  let dnsProviderFields: DnsProviderField[] = [];
  let dnsForm = { provider: '', credentials: {} as Record<string, string> };

  let preflightChecks: PreflightCheck[] = [];
  let preflightRanAt: string | null = null;

  function resetPreflight() {
    preflightChecks = [];
    preflightRanAt = null;
  }

  async function loadStatus() {
    const data = await apiProd<RemoteStatusPayload>('/remote/status').catch((err: ErrorResponse) => {
      if (err.code === 404) return null;
      throw err;
    });
    statusPayload = data;
    if (data) {
      solver = data.solver ?? solver;
      httpForm.endpoint = data.endpoint ?? '';
      domainTld = data.tld ?? '';
      const portalHost = data.portal_hostname ?? data.hostname ?? '';
      if (portalHost && domainTld) {
        if (portalHost === domainTld) {
          portalMode = 'root';
        } else if (portalHost.endsWith(`.${domainTld}`)) {
          portalMode = 'subdomain';
          portalPrefix = portalHost.replace(`.${domainTld}`, '') || 'portal';
        }
      }
    }
  }

  async function loadProviders() {
    const data = await apiProd<{ providers?: DnsProvider[] }>('/remote/dns/providers').catch((err: ErrorResponse) => {
      if (err.code === 404) return { providers: [] };
      throw err;
    });
    dnsProviders = data.providers ?? [];
    if (!dnsProviders.length) {
      dnsProviders = [{ id: 'generic', name: 'Generic DNS Provider', fields: [] }];
    }
    if (!dnsForm.provider) {
      dnsForm.provider = dnsProviders[0]?.id ?? '';
    }
    dnsProviderFields = dnsProviders.find((p) => p.id === dnsForm.provider)?.fields ?? [];
  }

  async function loadWizard() {
    loading = true;
    error = null;
    try {
      await Promise.all([loadStatus(), loadProviders()]);
      resetPreflight();
      const step = determineStep();
      activeStep = step;
    } catch (err: any) {
      error = err?.message || 'Failed to load remote status';
    } finally {
      loading = false;
    }
  }

  function determineStep(): StepKey {
    if (!statusPayload || !statusPayload.endpoint) return 'nexus';
    if (!statusPayload.tld || !(statusPayload.portal_hostname || statusPayload.hostname)) return 'domain';
    return 'solver';
  }

  function updateCredential(id: string, value: string) {
    dnsForm = { ...dnsForm, credentials: { ...dnsForm.credentials, [id]: value } };
  }

  function portalHostname(): string {
    const trimmed = domainTld.trim();
    if (!trimmed) return '';
    if (portalMode === 'root') return trimmed;
    return `${(portalPrefix || 'portal').trim()}.${trimmed}`;
  }

  function isValidWsEndpoint(value: string): boolean {
    const v = value.trim();
    if (!/^wss?:\/\//i.test(v)) return false;
    try {
      const u = new URL(v);
      return !!u.hostname;
    } catch {
      return false;
    }
  }

  function isValidDomain(value: string): boolean {
    const v = value.trim();
    // simple domain pattern (no protocol, at least one dot, 2+ TLD chars)
    return /^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?(?:\.[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?)+$/i.test(v);
  }

  $: httpFormError = (() => {
    if (!httpForm.endpoint) return null;
    if (!isValidWsEndpoint(httpForm.endpoint)) return 'Endpoint should start with ws:// or wss:// and include a valid host.';
    return null;
  })();

  $: domainError = (() => {
    const tld = domainTld.trim();
    if (!tld) return null;
    if (!isValidDomain(tld)) return 'Enter a valid domain like example.com (no protocol).';
    if (portalMode === 'subdomain' && !/^[a-z0-9]([a-z0-9-]{0,20}[a-z0-9])?$/i.test(portalPrefix.trim() || 'portal')) {
      return 'Subdomain must be alphanumerics with optional dashes (2–22 chars).';
    }
    return null;
  })();

  function handleCredentialInput(id: string, event: Event) {
    const target = event.currentTarget as HTMLInputElement | null;
    updateCredential(id, target?.value ?? '');
  }

  async function verifyHelper() {
    verifyingHelper = true;
    try {
      const payload = {
        endpoint: httpForm.endpoint.trim() || undefined,
        tld: domainTld.trim() || undefined,
        portal_hostname: portalHostname() || undefined,
        jwt_secret: httpForm.jwtSecret.trim() || undefined
      };
      await apiProd('/remote/nexus-guide/verify', { method: 'POST', body: JSON.stringify(payload) });
      toast('Nexus helper verified', 'success');
      await loadStatus();
    } catch (err: any) {
      toast(err?.message || 'Verification failed', 'error');
    } finally {
      verifyingHelper = false;
    }
  }

  async function saveConfiguration() {
    saving = true;
    try {
      const endpoint = httpForm.endpoint.trim();
      const secret = httpForm.jwtSecret.trim();
      const tld = domainTld.trim();
      if (!endpoint || !secret) throw new Error('Enter the Nexus endpoint and JWT secret.');
      if (!tld) throw new Error('Enter the Piccolo domain (TLD).');
      const portalHost = portalHostname();
      if (!portalHost) throw new Error('Portal hostname is required.');
      const basePayload: Record<string, unknown> = {
        solver,
        endpoint,
        device_secret: secret,
        tld,
        portal_hostname: portalHost
      };
      const payload =
        solver === 'dns-01'
          ? { ...basePayload, dns_provider: dnsForm.provider, dns_credentials: dnsForm.credentials }
          : basePayload;
      await apiProd('/remote/configure', { method: 'POST', body: JSON.stringify(payload) });
      toast('Remote configuration saved', 'success');
      await loadStatus();
      dispatch('updated');
      activeStep = 'solver';
    } catch (err: any) {
      toast(err?.message || 'Failed to save configuration', 'error');
    } finally {
      saving = false;
    }
  }

  async function runPreflight() {
    preflightWorking = true;
    try {
      const data = await apiProd<{ checks?: PreflightCheck[]; ran_at?: string }>('/remote/preflight', {
        method: 'POST'
      });
      preflightChecks = data.checks ?? [];
      preflightRanAt = data.ran_at ?? new Date().toISOString();
      toast('Preflight completed', 'success');
      await loadStatus();
      dispatch('updated');
    } catch (err: any) {
      toast(err?.message || 'Preflight failed', 'error');
    } finally {
      preflightWorking = false;
    }
  }

  $: wizardPortalHost = (statusPayload?.portal_hostname ?? statusPayload?.hostname ?? portalHostname()) || '';
  $: wizardComplete = Boolean(statusPayload?.enabled && !(statusPayload?.warnings?.length));
  $: wizardAttention = Boolean(statusPayload?.warnings?.length);

  function close() {
    dispatch('close');
  }

  type StepStatus = 'done' | 'current' | 'pending' | 'attention';

  function stepStatus(step: StepKey): StepStatus {
    if (step === 'solver' && statusPayload?.warnings?.length) {
      return 'attention';
    }
    const stepIndex = stepOrder.indexOf(step);
    const currentIndex = stepOrder.indexOf(activeStep);
    if (stepIndex < currentIndex) return 'done';
    if (stepIndex === currentIndex) return 'current';
    return 'pending';
  }

  function stepStatusLabel(status: StepStatus): string {
    if (status === 'done') return 'Done';
    if (status === 'current') return 'Current';
    if (status === 'attention') return 'Attention';
    return 'Pending';
  }

  function nextStep() {
    const idx = stepOrder.indexOf(activeStep);
    if (idx < stepOrder.length - 1) {
      activeStep = stepOrder[idx + 1];
    }
  }

  function prevStep() {
    const idx = stepOrder.indexOf(activeStep);
    if (idx > 0) {
      activeStep = stepOrder[idx - 1];
    }
  }

  function resetWizard() {
    httpForm = { endpoint: '', jwtSecret: '' };
    domainTld = '';
    portalMode = 'subdomain';
    portalPrefix = 'portal';
    solver = 'http-01';
    dnsForm = { provider: dnsProviders[0]?.id ?? '', credentials: {} };
    resetPreflight();
  }

  onMount(() => {
    loadWizard();
  });

  onDestroy(() => {
    resetWizard();
  });
</script>

<div class="wizard-backdrop">
  <div class="wizard-panel" role="dialog" aria-modal="true" aria-labelledby="remote-wizard-title">
    <header class="wizard-header">
      <div>
        <p class="text-xs uppercase tracking-[0.12em] text-text-muted">Remote setup</p>
        <h1 id="remote-wizard-title" class="text-xl font-semibold text-text-primary">Enable remote access</h1>
      </div>
      <button class="wizard-close" on:click={close} aria-label="Close remote setup">
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round" class="h-5 w-5">
          <path d="M18 6 6 18" />
          <path d="m6 6 12 12" />
        </svg>
      </button>
    </header>

    {#if loading}
      <div class="wizard-body">
        <p class="text-sm text-text-muted">Loading remote status…</p>
      </div>
    {:else if error}
      <div class="wizard-body space-y-4">
        <p class="text-sm text-state-critical">{error}</p>
        <button class="wizard-action" on:click={loadWizard}>Retry</button>
      </div>
    {:else}
      <div class="wizard-body">
        <div class="wizard-progress" role="group" aria-label="Remote setup progress">
          {#each stepOrder as step, index}
            {@const status = stepStatus(step)}
            <button
              class={`wizard-progress__step wizard-progress__step--${status}`}
              on:click={() => (activeStep = step)}
              aria-current={status === 'current' ? 'step' : undefined}
            >
              <span class="wizard-progress__index">{index + 1}</span>
              <div class="wizard-progress__meta">
                <span class="wizard-progress__label">{stepCopy[step].title}</span>
                <span class="wizard-progress__status">{stepStatusLabel(status)}</span>
              </div>
            </button>
          {/each}
        </div>

        {#if wizardComplete}
          <div class="wizard-status wizard-status--success" role="status">
            <span class="wizard-status__icon" aria-hidden="true">
              <svg viewBox="0 0 20 20" fill="none" stroke="currentColor" stroke-width="2" aria-hidden="true">
                <path d="M5 10.5 8.5 14 15 6" stroke-linecap="round" stroke-linejoin="round" />
              </svg>
            </span>
            <div>
              <p class="wizard-status__title">Remote access is live</p>
              <p class="wizard-status__subtitle">
                {wizardPortalHost ? `Portal reachable at ${wizardPortalHost}` : 'Remote tunnel is active.'}
              </p>
            </div>
          </div>
        {:else if wizardAttention}
          <div class="wizard-status wizard-status--warn" role="status">
            <span class="wizard-status__icon" aria-hidden="true">
              <svg viewBox="0 0 20 20" fill="none" stroke="currentColor" stroke-width="2" aria-hidden="true">
                <path d="M10 5v5" stroke-linecap="round" />
                <circle cx="10" cy="14" r="1" fill="currentColor" />
                <path d="M10 2 2 18h16L10 2Z" stroke-linejoin="round" />
              </svg>
            </span>
            <div>
              <p class="wizard-status__title">Remote needs attention</p>
              <ul class="wizard-status__list">
                {#each statusPayload?.warnings ?? [] as warning}
                  <li>{warning}</li>
                {/each}
              </ul>
            </div>
          </div>
        {/if}

        {#if activeStep === 'nexus'}
          <section class="wizard-section" aria-labelledby="wizard-step-nexus">
            <header class="wizard-section__header">
              <p class="wizard-section__eyebrow">Step 1</p>
              <h2 id="wizard-step-nexus">{stepCopy.nexus.title}</h2>
              <p>{stepCopy.nexus.description}</p>
            </header>
            <div class="wizard-section__body">
              <div class="wizard-callout">
                <p class="font-medium text-text-primary">What you need</p>
                <ul class="list-disc ml-5 mt-2 text-xs text-text-muted space-y-1">
                  <li>A public VM with ports 80/443 open.</li>
                  <li>The Nexus installer output: endpoint URL and JWT secret.</li>
                </ul>
              </div>
              <p class="wizard-section__intro">
                Run the Nexus installer on your helper VM, then paste the endpoint and JWT secret below.
              </p>
              <label class="wizard-field">
                <span>Nexus endpoint</span>
                <input
                  class="wizard-input"
                  bind:value={httpForm.endpoint}
                  placeholder="wss://nexus.example.com/connect"
                  autocomplete="off"
                  autocapitalize="none"
                  spellcheck={false}
                />
                {#if httpFormError}
                  <span class="text-xs text-state-warn">{httpFormError}</span>
                {/if}
              </label>
              <label class="wizard-field">
                <span>JWT signing secret</span>
                <input
                  class="wizard-input"
                  type="password"
                  bind:value={httpForm.jwtSecret}
                  placeholder="copy from Nexus helper"
                  autocomplete="new-password"
                  autocapitalize="none"
                  spellcheck={false}
                />
              </label>
              <div class="wizard-section__actions">
                <button class="wizard-action" on:click={verifyHelper} disabled={verifyingHelper}>
                  {verifyingHelper ? 'Verifying…' : 'Verify helper'}
                </button>
                <button class="wizard-secondary" on:click={nextStep} disabled={!httpForm.endpoint.trim() || !!httpFormError}>
                  Continue
                </button>
              </div>
            </div>
          </section>
        {:else if activeStep === 'domain'}
          <section class="wizard-section" aria-labelledby="wizard-step-domain">
            <header class="wizard-section__header">
              <p class="wizard-section__eyebrow">Step 2</p>
              <h2 id="wizard-step-domain">{stepCopy.domain.title}</h2>
              <p>{stepCopy.domain.description}</p>
            </header>
            <div class="wizard-section__body">
              <div class="wizard-callout">
                <p class="font-medium text-text-primary">What happens</p>
                <p class="text-xs text-text-muted mt-1">Piccolo serves the portal at your domain and solves certificates for the portal and wildcard listeners.</p>
              </div>
              <label class="wizard-field">
                <span>Piccolo domain (TLD)</span>
                <input
                  class="wizard-input"
                  bind:value={domainTld}
                  placeholder="myname.com"
                  autocomplete="off"
                  autocapitalize="none"
                  spellcheck={false}
                />
                {#if domainError}
                  <span class="text-xs text-state-warn">{domainError}</span>
                {/if}
              </label>
              <fieldset class="wizard-radio-group">
                <legend class="sr-only">Portal hostname preference</legend>
                <label class="wizard-radio">
                  <input type="radio" value="root" bind:group={portalMode} />
                  <span>Use the TLD for the portal <code>{domainTld || 'your-domain.com'}</code></span>
                </label>
                <label class="wizard-radio">
                  <input type="radio" value="subdomain" bind:group={portalMode} />
                  <span>Use a dedicated subdomain</span>
                </label>
                {#if portalMode === 'subdomain'}
                  <div class="wizard-subdomain">
                    <input
                      class="wizard-input wizard-subdomain__input"
                      bind:value={portalPrefix}
                      placeholder="portal"
                      autocomplete="off"
                      autocapitalize="none"
                      spellcheck={false}
                    />
                    <span class="wizard-subdomain__hint">
                      Full host: <code>{portalHostname() || 'portal.your-domain.com'}</code>
                    </span>
                  </div>
                {/if}
              </fieldset>
              <div class="wizard-callout">
                <p class="font-medium text-text-primary">DNS checklist</p>
                <ul class="mt-2 space-y-1 list-disc ml-4">
                  <li>CNAME <code>*.{domainTld || 'your-domain.com'}</code> → Nexus helper host</li>
                  <li>Portal host <code>{portalHostname() || 'portal.your-domain.com'}</code> points to the same helper.</li>
                </ul>
              </div>
              <div class="wizard-section__actions">
                <button class="wizard-secondary" on:click={prevStep}>Back</button>
                <button class="wizard-action" on:click={nextStep} disabled={!portalHostname() || !!domainError}>
                  Continue
                </button>
              </div>
            </div>
          </section>
        {:else}
          <section class="wizard-section" aria-labelledby="wizard-step-solver">
            <header class="wizard-section__header">
              <p class="wizard-section__eyebrow">Step 3</p>
              <h2 id="wizard-step-solver">{stepCopy.solver.title}</h2>
              <p>{stepCopy.solver.description}</p>
            </header>
            <div class="wizard-section__body">
              <div class="wizard-toggle">
                <button class:active={solver === 'http-01'} on:click={() => (solver = 'http-01')}>HTTP-01</button>
                <button class:active={solver === 'dns-01'} on:click={() => (solver = 'dns-01')}>DNS-01</button>
              </div>
              {#if solver === 'http-01'}
                <div class="wizard-callout">
                  Piccolo solves HTTP-01 challenges through Nexus. Ensure ports 80/443 reach the helper.
                </div>
              {:else}
                <div class="space-y-3">
                  <label class="wizard-field">
                    <span>DNS provider</span>
                    <select
                      class="wizard-input"
                      bind:value={dnsForm.provider}
                      on:change={() => (dnsProviderFields = dnsProviders.find((p) => p.id === dnsForm.provider)?.fields ?? [])}
                    >
                      {#each dnsProviders as provider}
                        <option value={provider.id}>{provider.name}</option>
                      {/each}
                    </select>
                  </label>
                  {#if dnsProviderFields.length === 0}
                    <p class="text-xs text-text-muted">This provider does not require API credentials.</p>
                  {:else}
                    {#each dnsProviderFields as field}
                      <label class="wizard-field">
                        <span>{field.label}</span>
                        <input
                          class="wizard-input"
                          type={field.secret ? 'password' : 'text'}
                          value={dnsForm.credentials[field.id] ?? ''}
                          on:input={(event) => handleCredentialInput(field.id, event)}
                          placeholder={field.placeholder || ''}
                        />
                        {#if field.description}
                          <span class="text-xs text-text-muted">{field.description}</span>
                        {/if}
                      </label>
                    {/each}
                  {/if}
                </div>
              {/if}
              <div class="wizard-callout">
                Requesting certificates for portal host <code>{portalHostname() || 'portal.your-domain.com'}</code> and listener wildcard.
              </div>
              <div class="wizard-section__actions wizard-section__actions--wrap">
                <button class="wizard-secondary" on:click={prevStep}>Back</button>
                <button class="wizard-action" on:click={saveConfiguration} disabled={saving}>
                  {saving ? 'Saving…' : 'Save configuration'}
                </button>
                <button class="wizard-secondary" on:click={runPreflight} disabled={preflightWorking}>
                  {preflightWorking ? 'Running…' : 'Run preflight'}
                </button>
              </div>
              {#if preflightChecks.length}
                <div class="wizard-results">
                  <p class="text-sm font-semibold text-text-primary">Preflight results</p>
                  <p class="text-xs text-text-muted">Last run {preflightRanAt ? new Date(preflightRanAt).toLocaleString() : ''}</p>
                  <ul class="mt-3 space-y-2">
                    {#each preflightChecks as check}
                      <li class="wizard-result">
                        <div class="flex items-center justify-between">
                          <span class="text-sm font-medium text-text-primary">{check.name}</span>
                          <span class={`wizard-status-pill ${check.status === 'pass' ? 'ok' : check.status === 'warn' ? 'warn' : check.status === 'pending' ? 'pending' : 'error'}`}>
                            {check.status.toUpperCase()}
                          </span>
                        </div>
                        {#if check.detail}
                          <p class="text-xs text-text-muted mt-1">{check.detail}</p>
                        {/if}
                        {#if check.next_step}
                          <p class="text-xs text-text-muted mt-1">Next: {check.next_step}</p>
                        {/if}
                      </li>
                    {/each}
                  </ul>
                </div>
              {/if}
            </div>
          </section>
        {/if}
      </div>
    {/if}
  </div>
</div>

<style>
  .wizard-backdrop {
    position: fixed;
    inset: 0;
    z-index: 90;
    display: flex;
    align-items: center;
    justify-content: center;
    background: var(--scrim-strong);
    padding: 24px;
  }
  .wizard-panel {
    background: var(--surface-1);
    border-radius: 24px;
    border: 1px solid var(--border);
    width: min(640px, 100%);
    max-height: min(90vh, 860px);
    display: flex;
    flex-direction: column;
    box-shadow: 0 32px 80px rgba(15, 23, 42, 0.28);
  }
  .wizard-header {
    padding: 24px 32px;
    border-bottom: 1px solid var(--border-subtle);
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 16px;
  }
  .wizard-close {
    border: 1px solid var(--border-subtle);
    background: transparent;
    border-radius: 999px;
    height: 40px;
    width: 40px;
    display: inline-flex;
    align-items: center;
    justify-content: center;
    color: var(--text-muted);
  }
  .wizard-close:hover {
    background: var(--surface-2);
  }
  .wizard-body {
    padding: 20px 24px 28px;
    overflow-y: auto;
    display: flex;
    flex-direction: column;
    gap: 24px;
  }
  .wizard-progress {
    display: grid;
    gap: 12px;
    grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
  }
  .wizard-progress__step {
    display: flex;
    align-items: center;
    justify-content: space-between;
    border-radius: 18px;
    border: 1px solid rgba(var(--border-rgb) / 0.16);
    padding: 12px 16px;
    background: var(--surface-0);
    color: var(--text-muted);
    cursor: pointer;
    transition: border-color var(--transition-duration) var(--transition-easing), background var(--transition-duration) var(--transition-easing), color var(--transition-duration) var(--transition-easing);
  }
  .wizard-progress__step:hover {
    border-color: rgba(var(--border-rgb) / 0.3);
  }
  .wizard-progress__step--current {
    border-color: rgba(var(--accent-rgb) / 0.6);
    background: rgba(var(--accent-rgb) / 0.14);
    color: var(--text-primary);
  }
  .wizard-progress__step--done {
    border-color: rgba(var(--state-ok-rgb) / 0.4);
    background: rgba(var(--state-ok-rgb) / 0.12);
    color: rgb(var(--state-ok-rgb));
  }
  .wizard-progress__step--attention {
    border-color: rgba(var(--state-warn-rgb) / 0.45);
    background: rgba(var(--state-warn-rgb) / 0.12);
    color: rgb(var(--state-warn-rgb));
  }
  .wizard-progress__index {
    height: 28px;
    width: 28px;
    border-radius: 999px;
    border: 1px solid rgba(var(--border-rgb) / 0.2);
    display: inline-flex;
    align-items: center;
    justify-content: center;
    font-size: 0.75rem;
    font-weight: 600;
  }
  .wizard-progress__meta {
    display: flex;
    flex-direction: column;
    gap: 2px;
    text-align: left;
  }
  .wizard-progress__label {
    font-size: 0.9rem;
    font-weight: 600;
    color: inherit;
  }
  .wizard-progress__status {
    font-size: 0.72rem;
    text-transform: uppercase;
    letter-spacing: 0.14em;
    color: inherit;
  }
  .wizard-status {
    display: flex;
    align-items: flex-start;
    gap: 12px;
    border-radius: 20px;
    border: 1px solid rgba(var(--border-rgb) / 0.18);
    padding: 16px 18px;
    background: rgba(var(--border-rgb) / 0.05);
  }
  .wizard-status--success {
    border-color: rgba(var(--state-ok-rgb) / 0.32);
    background: rgba(var(--state-ok-rgb) / 0.12);
    color: rgb(var(--state-ok-rgb));
  }
  .wizard-status--warn {
    border-color: rgba(var(--state-warn-rgb) / 0.35);
    background: rgba(var(--state-warn-rgb) / 0.12);
    color: rgb(var(--state-warn-rgb));
  }
  .wizard-status__icon {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    height: 36px;
    width: 36px;
    border-radius: 12px;
    background: var(--surface-1);
    border: 1px solid rgba(var(--border-rgb) / 0.16);
  }
  .wizard-status__title {
    font-size: 0.95rem;
    font-weight: 600;
    color: var(--text-primary);
  }
  .wizard-status--success .wizard-status__title,
  .wizard-status--success .wizard-status__subtitle {
    color: inherit;
  }
  .wizard-status__subtitle {
    margin-top: 2px;
    font-size: 0.82rem;
    color: var(--text-muted);
  }
  .wizard-status__list {
    margin: 4px 0 0;
    padding-left: 18px;
    font-size: 0.82rem;
    color: inherit;
    list-style: disc;
  }
  .wizard-section {
    border: 1px solid rgba(var(--border-rgb) / 0.16);
    border-radius: 24px;
    background: var(--surface-1);
    padding: 24px 28px 28px;
    box-shadow: 0 18px 40px rgba(15, 23, 42, 0.08);
  }
  .wizard-section + .wizard-section {
    margin-top: 16px;
  }
  .wizard-section__header {
    display: flex;
    flex-direction: column;
    gap: 6px;
    margin-bottom: 20px;
  }
  .wizard-section__eyebrow {
    text-transform: uppercase;
    letter-spacing: 0.18em;
    font-size: 0.7rem;
    font-weight: 600;
    color: var(--text-muted);
  }
  .wizard-section__header h2 {
    font-size: 1.1rem;
    font-weight: 600;
    color: var(--text-primary);
  }
  .wizard-section__header p {
    font-size: 0.9rem;
    color: var(--text-muted);
  }
  .wizard-section__body {
    display: flex;
    flex-direction: column;
    gap: 16px;
  }
  .wizard-section__intro {
    font-size: 0.9rem;
    color: var(--text-muted);
  }
  .wizard-field {
    display: flex;
    flex-direction: column;
    gap: 6px;
    font-size: 13px;
    color: var(--text-primary);
  }
  .wizard-input {
    width: 100%;
    border-radius: 16px;
    border: 1px solid rgba(var(--border-rgb) / 0.18);
    padding: 10px 14px;
    font-size: 14px;
    background: var(--surface-0);
  }
  .wizard-radio-group {
    display: flex;
    flex-direction: column;
    gap: 8px;
    font-size: 13px;
    color: var(--text-primary);
  }
  .wizard-radio {
    display: flex;
    align-items: flex-start;
    gap: 8px;
  }
  .wizard-callout {
    border-radius: 16px;
    border: 1px solid var(--border-subtle);
    background: var(--surface-2);
    padding: 16px;
    font-size: 13px;
    color: var(--text-muted);
  }
  .wizard-toggle {
    display: inline-flex;
    border: 1px solid var(--border);
    border-radius: 16px;
    overflow: hidden;
  }
  .wizard-toggle button {
    padding: 8px 16px;
    font-size: 13px;
    font-weight: 600;
    color: var(--text-muted);
    background: var(--surface-1);
    border: none;
    cursor: pointer;
  }
  .wizard-toggle button.active {
    background: var(--accent-subtle);
    color: var(--accent-emphasis);
  }
  .wizard-section__actions {
    display: flex;
    align-items: center;
    gap: 12px;
  }
  .wizard-section__actions--wrap {
    flex-wrap: wrap;
  }
  .wizard-action {
    background: var(--accent);
    color: var(--text-inverse);
    border-radius: 999px;
    border: 1px solid var(--accent-emphasis);
    padding: 10px 20px;
    font-size: 13px;
    font-weight: 600;
    cursor: pointer;
    min-height: 44px;
  }
  .wizard-action:disabled {
    opacity: 0.6;
    cursor: not-allowed;
  }
  .wizard-secondary {
    border-radius: 999px;
    border: 1px solid var(--border);
    padding: 10px 20px;
    font-size: 13px;
    font-weight: 600;
    color: var(--text-primary);
    background: var(--surface-1);
    cursor: pointer;
    min-height: 44px;
  }
  .wizard-secondary:disabled {
    opacity: 0.6;
    cursor: not-allowed;
  }
  .wizard-results {
    border-top: 1px solid var(--border-subtle);
    padding-top: 16px;
  }
  .wizard-result {
    border: 1px solid var(--border-subtle);
    border-radius: 14px;
    padding: 12px 14px;
    background: var(--surface-0);
  }
  .wizard-status-pill {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    min-width: 80px;
    padding: 4px 10px;
    border-radius: 999px;
    font-size: 11px;
    font-weight: 600;
    letter-spacing: 0.08em;
  }
  .wizard-status-pill.ok {
    background: rgba(var(--state-ok-rgb) / 0.12);
    color: rgb(var(--state-ok-rgb));
  }
  .wizard-status-pill.warn {
    background: rgba(var(--state-warn-rgb) / 0.12);
    color: rgb(var(--state-warn-rgb));
  }
  .wizard-status-pill.pending {
    background: rgba(var(--state-notice-rgb) / 0.12);
    color: rgb(var(--state-notice-rgb));
  }
  .wizard-status-pill.error {
    background: rgba(var(--state-critical-rgb) / 0.12);
    color: rgb(var(--state-critical-rgb));
  }
  .wizard-subdomain {
    display: flex;
    align-items: center;
    gap: 12px;
    padding-left: 32px;
  }
  .wizard-subdomain__input {
    max-width: 140px;
  }
  .wizard-subdomain__hint {
    font-size: 12px;
    color: var(--text-muted);
  }
  @media (max-width: 640px) {
    .wizard-backdrop {
      padding: 16px;
    }
    .wizard-panel {
      width: 100%;
      border-radius: 20px;
    }
    .wizard-header {
      padding: 20px;
    }
    .wizard-body {
      padding: 20px 20px 32px;
      gap: 20px;
    }
    .wizard-progress {
      grid-template-columns: 1fr;
    }
    .wizard-section {
      padding: 20px;
    }
    .wizard-section__actions {
      flex-direction: column;
      align-items: stretch;
    }
    .wizard-section__actions button {
      width: 100%;
    }
    .wizard-status {
      flex-direction: column;
    }
    .wizard-subdomain {
      padding-left: 0;
      flex-direction: column;
      align-items: stretch;
    }
    .wizard-subdomain__hint {
      padding-left: 0;
    }
  }
</style>
