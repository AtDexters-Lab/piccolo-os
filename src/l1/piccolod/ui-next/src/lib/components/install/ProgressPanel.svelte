<script lang="ts">
  export let notes: string[] = [];
  export let pending = false;
  export let complete = false;
</script>

<section class="rounded-3xl border border-white/30 bg-gradient-to-br from-white/95 via-white/80 to-slate-50 p-6 shadow-2xl shadow-slate-200/50">
  <header class="flex items-center justify-between">
    <div>
      <p class="text-xs uppercase tracking-[0.3em] text-muted">Progress</p>
      <h3 class="text-lg font-semibold text-slate-900">{complete ? 'Install queued' : pending ? 'Writing image…' : 'Awaiting confirmation'}</h3>
    </div>
    <div class={`flex h-10 w-10 items-center justify-center rounded-2xl ${complete ? 'bg-success/15 text-success' : pending ? 'bg-accent/15 text-accent' : 'bg-white text-muted'} }`} aria-hidden="true">
      {#if complete}
        ✓
      {:else if pending}
        <span class="animate-spin">⏳</span>
      {:else}
        ·
      {/if}
    </div>
  </header>
  <ul class="mt-4 space-y-2 text-sm text-slate-800">
    {#each notes as note}
      <li class="rounded-2xl border border-slate-100 bg-white/90 px-4 py-3">{note}</li>
    {/each}
    {#if pending}
      <li class="rounded-2xl border border-slate-100 bg-white/90 px-4 py-3">Device is writing the signed image…</li>
    {/if}
  </ul>
  {#if complete}
    <p class="mt-4 text-sm text-muted">Stay nearby—the device will reboot automatically when finished.</p>
  {/if}
</section>
