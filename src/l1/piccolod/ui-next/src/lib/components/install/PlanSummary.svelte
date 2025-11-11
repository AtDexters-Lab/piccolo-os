<script lang="ts">
  import type { InstallPlan, InstallTarget, FetchLatestImage } from '$lib/api/install';

  export let plan: InstallPlan | null = null;
  export let target: InstallTarget | null = null;
  export let latest: FetchLatestImage | null = null;

  const baseActions = ['Validate image signature', 'Write disk image', 'Expand root partition', 'Prepare Piccolo data volume'];

  function combinedActions(): string[] {
    if (!plan) return [];
    const custom = plan.actions.filter(Boolean);
    const deduped = [...new Set([...custom, ...baseActions])];
    return deduped;
  }
  $: actions = combinedActions();

  function targetSummary() {
    if (!target) return 'No disk selected';
    return `${target.model} · ${target.id}`;
  }
</script>

<div class="plan-summary elev-2">
  <header class="space-y-1">
    <p class="meta-label">Plan</p>
    <h3 class="text-lg font-semibold text-ink">{plan ? 'Simulated actions' : 'Awaiting plan'}</h3>
    <p class="text-sm text-muted">{targetSummary()}</p>
  </header>

  {#if plan}
    <ol class="plan-summary__timeline mt-6 space-y-4">
      {#each actions as action, index}
        <li class="relative pl-8">
          <span class={`plan-summary__index ${index === 0 ? 'plan-summary__index--accent' : ''}`}>
            {index + 1}
          </span>
          <p class="text-sm font-semibold text-ink">{action}</p>
          {#if index === 0 && latest?.version}
            <p class="text-xs text-muted">Latest image {latest.version} ({latest.verified ? 'verified' : 'pending signature'})</p>
          {/if}
        </li>
      {/each}
    </ol>
    <div class="mt-6 flex flex-wrap gap-3 text-xs text-muted">
      <span class="plan-summary__chip">{plan.simulate ? 'Simulated (dry run)' : 'Ready to write'}</span>
      {#if latest?.sizeBytes}
        <span class="plan-summary__chip">Download size ≈ {Math.round((latest.sizeBytes ?? 0) / (1024 * 1024))} MB</span>
      {/if}
    </div>
  {:else}
    <p class="mt-6 text-sm text-muted">Generate a plan to preview the exact steps Piccolo will perform before writing the disk.</p>
  {/if}
</div>

<style>
  .plan-summary {
    border-radius: var(--radius-xl);
    border: 1px solid var(--card-border);
    background: var(--card-gradient);
    padding: 1.5rem;
    box-shadow: var(--shadow-soft);
  }

  .plan-summary__timeline {
    position: relative;
    padding-left: 0;
  }

  .plan-summary__timeline::before {
    content: '';
    position: absolute;
    left: 0.75rem;
    top: 0;
    width: 1px;
    height: 100%;
    background: var(--outline-strong);
  }

  .plan-summary__index {
    position: absolute;
    left: 0;
    top: 2px;
    display: flex;
    height: 1.5rem;
    width: 1.5rem;
    align-items: center;
    justify-content: center;
    border-radius: var(--radius-pill);
    border: 1px solid var(--outline-strong);
    background: rgb(var(--sys-surface-variant));
    color: rgb(var(--sys-ink-muted));
    font-size: 0.85rem;
    font-variant-numeric: tabular-nums;
  }

  .plan-summary__index--accent {
    border-color: var(--btn-secondary-outline);
    background: var(--btn-secondary-hover-bg);
    color: var(--sys-link);
  }

  .plan-summary__chip {
    border-radius: var(--radius-pill);
    border: 1px solid var(--card-border);
    padding: 0.35rem 0.85rem;
    background: rgba(255, 255, 255, 0.6);
  }

  :global([data-theme='dark']) .plan-summary__chip {
    background: rgba(5, 7, 16, 0.55);
  }
</style>
