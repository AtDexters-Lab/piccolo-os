<script lang="ts">
  import type { InstallTarget } from '$lib/api/install';

  export let target: InstallTarget;
  export let selected = false;

  const glyphPath = 'M6 4h12a2 2 0 0 1 2 2v12a2 2 0 0 1-2 2H6a2 2 0 0 1-2-2V6a2 2 0 0 1 2-2zm0 2v12h12V6zM8 17h8';

  function formatBytes(bytes: number): string {
    if (!bytes) return '0 B';
    const units = ['B', 'KB', 'MB', 'GB', 'TB'];
    let value = bytes;
    let idx = 0;
    while (value >= 1024 && idx < units.length - 1) {
      value /= 1024;
      idx++;
    }
    const formatted = value >= 10 || idx === 0 ? value.toFixed(0) : value.toFixed(1);
    return `${formatted} ${units[idx]}`;
  }
</script>

<button
  type="button"
  class={`group relative overflow-hidden rounded-3xl border px-5 py-5 text-left transition-all duration-200 ${selected ? 'border-accent/80 bg-white shadow-2xl shadow-accent/20' : 'border-white/20 bg-white/80 hover:border-accent/40 hover:shadow-lg'} `}
  aria-pressed={selected}
>
  <div class="absolute inset-0 opacity-0 transition-opacity duration-200 group-hover:opacity-60" aria-hidden="true">
    <div class="absolute inset-0 bg-gradient-to-br from-accent/10 via-transparent to-slate-100"></div>
  </div>
  <div class="relative flex items-center gap-3">
    <span class={`flex h-12 w-12 items-center justify-center rounded-2xl border ${selected ? 'border-accent/60 bg-accent/10 text-accent' : 'border-white/40 bg-white text-muted'}`}>
      <svg width="26" height="26" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round">
        <path d={glyphPath} />
      </svg>
    </span>
    <div class="flex-1">
      <p class="text-sm font-semibold text-slate-900">{target.model}</p>
      <p class="text-xs text-muted">{target.id}</p>
    </div>
    {#if selected}
      <span class="rounded-full bg-accent/10 px-3 py-1 text-xs font-semibold text-accent">Selected</span>
    {/if}
  </div>
  <div class="relative mt-4 grid gap-2 text-sm text-slate-900 sm:grid-cols-2">
    <div class="rounded-2xl border border-white/50 bg-white/60 px-3 py-2">
      <p class="text-xs uppercase tracking-wide text-muted">Capacity</p>
      <p class="text-base font-semibold">{formatBytes(target.sizeBytes)}</p>
    </div>
    <div class="rounded-2xl border border-white/50 bg-white/60 px-3 py-2">
      <p class="text-xs uppercase tracking-wide text-muted">Contents</p>
      <p class="text-sm text-slate-800">{target.contents?.length ? target.contents.join(', ') : 'No partitions detected'}</p>
    </div>
  </div>
  {#if target.eraseWarning}
    <p class="relative mt-4 rounded-2xl border border-amber-200 bg-amber-50 px-4 py-3 text-xs font-medium text-amber-900">
      Installing will erase all existing data.
    </p>
  {/if}
</button>
