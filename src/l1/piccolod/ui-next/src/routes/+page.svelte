<script lang="ts">
  import Button from '$lib/components/ui/Button.svelte';
</script>

<svelte:head>
  <title>Piccolo UI Next</title>
</svelte:head>

<main class="min-h-screen px-6 py-12 sm:px-10 lg:px-16 flex flex-col gap-8 text-sm">
  <section class="rounded-3xl border border-white/40 bg-white/80 backdrop-blur-xl elev-3 p-6 text-ink">
    <p class="meta-label">Foundation</p>
    <h1 class="text-2xl font-semibold text-ink mt-3">Piccolo UI Next</h1>
    <p class="text-base text-muted mt-2 max-w-2xl">
      This workspace hosts the next-generation Piccolo UI. We are bootstrapping the admin setup wizard
      and the surrounding shell using SvelteKit, Tailwind, and TanStack Query.
    </p>
    <div class="mt-4 flex flex-wrap gap-3">
      <Button variant="primary" href="/setup">
        Open setup wizard
      </Button>
      <Button variant="secondary" href="/docs/foundation.md" target="_blank">
        View foundations doc
      </Button>
    </div>
  </section>

  <div class="grid gap-6 lg:grid-cols-3">
    <article class="rounded-2xl border border-white/30 bg-white/70 backdrop-blur-lg elev-2 p-5 flex flex-col gap-3">
      <p class="meta-label">Stack</p>
      <h2 class="text-lg font-semibold text-ink">Tooling installed</h2>
      <ul class="text-text list-disc list-inside space-y-1 text-sm">
        <li>SvelteKit + TypeScript</li>
        <li>Tailwind CSS / PostCSS / Autoprefixer</li>
        <li>@tanstack/svelte-query (with provider)</li>
      </ul>
    </article>
    <article class="rounded-2xl border border-white/30 bg-white/70 backdrop-blur-lg elev-2 p-5 flex flex-col gap-3">
      <p class="meta-label">Next milestone</p>
      <h2 class="text-lg font-semibold text-ink">Admin setup wizard</h2>
      <p class="text-sm text-muted">
        Implement the guided sheet that creates/validates the initial admin password during first boot.
      </p>
    </article>
    <article class="rounded-2xl border border-white/30 bg-white/70 backdrop-blur-lg elev-2 p-5 flex flex-col gap-3">
      <p class="meta-label">Docs</p>
      <h2 class="text-lg font-semibold text-ink">Foundations</h2>
      <p class="text-sm text-muted">
        Refer to <code>src/l1/piccolod/ui-next/docs/foundation.md</code> for principles, layout rules,
        theming, and testing workflow.
      </p>
    </article>
  </div>
</main>
