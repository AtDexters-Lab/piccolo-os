<script lang="ts">
  export let title = 'Piccolo';
</script>

<div class="min-h-screen bg-[var(--app-background)] text-text">
  <header class="border-b border-white/20 bg-white/40 backdrop-blur-xl text-slate-900">
    <div class="mx-auto flex max-w-5xl items-center justify-between px-6 py-4">
      <div class="flex items-center gap-3">
        <div class="h-10 w-10 rounded-2xl bg-accent text-white flex items-center justify-center font-semibold">
          P
        </div>
        <div>
          <p class="text-xs uppercase tracking-[0.3em] text-muted">Piccolo OS</p>
          <h1 class="text-lg font-semibold">{title}</h1>
        </div>
      </div>
      <slot name="header-actions"></slot>
    </div>
  </header>
  <main class="mx-auto max-w-5xl px-6 py-10">
    <slot />
  </main>
</div>
