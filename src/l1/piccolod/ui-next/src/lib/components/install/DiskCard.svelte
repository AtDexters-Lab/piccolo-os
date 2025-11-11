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

<button type="button" class={`disk-card ${selected ? 'disk-card--selected' : ''}`} aria-pressed={selected}>
  <div class="relative flex items-center gap-3">
    <span class={`disk-card__glyph ${selected ? 'disk-card__glyph--accent' : ''}`}>
      <svg width="26" height="26" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round">
        <path d={glyphPath} />
      </svg>
    </span>
    <div class="flex-1">
      <p class="text-sm font-semibold text-ink">{target.model}</p>
      <p class="text-xs text-muted">{target.id}</p>
    </div>
    {#if selected}
      <span class="rounded-full bg-accent/10 px-3 py-1 text-xs font-semibold text-accent">Selected</span>
    {/if}
  </div>
  <div class="relative mt-4 grid gap-2 text-sm text-ink sm:grid-cols-2">
    <div class="rounded-2xl border border-white/50 bg-white/60 px-3 py-2">
      <p class="text-xs uppercase tracking-wide text-muted">Capacity</p>
      <p class="text-base font-semibold">{formatBytes(target.sizeBytes)}</p>
    </div>
    <div class="rounded-2xl border border-white/50 bg-white/60 px-3 py-2">
      <p class="text-xs uppercase tracking-wide text-muted">Contents</p>
      <p class="text-sm text-ink">{target.contents?.length ? target.contents.join(', ') : 'No partitions detected'}</p>
    </div>
  </div>
  {#if target.eraseWarning}
    <p class="disk-card__warning">Installing will erase all existing data.</p>
  {/if}
</button>

<style>
  .disk-card {
    position: relative;
    overflow: hidden;
    border-radius: var(--radius-xl);
    border: 1px solid var(--card-border);
    padding: 1.25rem;
    text-align: left;
    background: var(--card-gradient);
    transition: box-shadow var(--motion-dur-fast) var(--motion-ease-emphasized),
      border-color var(--motion-dur-fast) var(--motion-ease-standard),
      transform var(--motion-dur-fast) var(--motion-ease-standard);
    box-shadow: var(--shadow-soft);
  }

  .disk-card::after {
    content: '';
    position: absolute;
    inset: 0;
    background: radial-gradient(circle at top right, rgba(63, 107, 255, 0.08), transparent 55%);
    opacity: 0;
    transition: opacity var(--motion-dur-fast) var(--motion-ease-standard);
    pointer-events: none;
  }

  .disk-card--selected {
    border-color: var(--btn-secondary-outline);
    box-shadow: var(--shadow-strong);
  }

  @media (hover: hover) and (pointer: fine) {
    .disk-card:hover {
      border-color: var(--btn-secondary-outline);
      box-shadow: var(--shadow-strong);
      transform: translateY(-2px);
    }

    .disk-card:hover::after {
      opacity: 1;
    }
  }

  .disk-card__glyph {
    display: flex;
    height: 3rem;
    width: 3rem;
    align-items: center;
    justify-content: center;
    border-radius: var(--radius-lg);
    border: 1px solid rgba(255, 255, 255, 0.45);
    background: rgba(255, 255, 255, 0.8);
    color: rgb(var(--sys-ink));
  }

  .disk-card__glyph--accent {
    border-color: var(--btn-secondary-outline);
    background: var(--btn-secondary-hover-bg);
    color: var(--sys-link);
  }

  .disk-card__warning {
    margin-top: 1rem;
    border-radius: var(--radius-lg);
    border: 1px solid rgb(var(--sys-warning) / 0.35);
    background: rgb(var(--sys-warning) / 0.12);
    padding: 0.75rem 1rem;
    font-size: 0.75rem;
    font-weight: 600;
    color: rgb(var(--sys-on-warning));
  }
</style>
