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

<div class="rounded-3xl border border-white/30 bg-white/90 p-6 shadow-xl shadow-slate-200/40">
  <header class="space-y-1">
    <p class="text-xs uppercase tracking-[0.3em] text-muted">Plan</p>
    <h3 class="text-lg font-semibold text-slate-900">{plan ? 'Simulated actions' : 'Awaiting plan'}</h3>
    <p class="text-sm text-muted">{targetSummary()}</p>
  </header>

  {#if plan}
    <ol class="relative mt-6 space-y-4 before:absolute before:left-3 before:top-0 before:h-full before:w-px before:bg-slate-200">
      {#each actions as action, index}
        <li class="relative pl-8">
          <span class={`absolute left-0 top-[2px] flex h-6 w-6 items-center justify-center rounded-full border ${index === 0 ? 'border-accent bg-accent/10 text-accent' : 'border-slate-200 bg-white text-muted'}`}>
            {index + 1}
          </span>
          <p class="text-sm font-semibold text-slate-900">{action}</p>
          {#if index === 0 && latest?.version}
            <p class="text-xs text-muted">Latest image {latest.version} ({latest.verified ? 'verified' : 'pending signature'})</p>
          {/if}
        </li>
      {/each}
    </ol>
    <div class="mt-6 flex flex-wrap gap-3 text-xs text-muted">
      <span class="rounded-full border border-slate-200 px-3 py-1">{plan.simulate ? 'Simulated (dry run)' : 'Ready to write'}</span>
      {#if latest?.sizeBytes}
        <span class="rounded-full border border-slate-200 px-3 py-1">Download size ≈ {Math.round((latest.sizeBytes ?? 0) / (1024 * 1024))} MB</span>
      {/if}
    </div>
  {:else}
    <p class="mt-6 text-sm text-muted">Generate a plan to preview the exact steps Piccolo will perform before writing the disk.</p>
  {/if}
</div>
