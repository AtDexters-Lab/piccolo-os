<script lang="ts">
  export let title = 'Piccolo';
</script>

<div class="min-h-screen text-ink" style="background: var(--hero-gradient);">
  <header class="border-b border-white/20 bg-white/40 backdrop-blur-xl text-ink">
    <div class="mx-auto flex max-w-5xl items-center justify-between px-6 py-4">
      <div class="flex items-center gap-3" aria-label={title}>
        <div class="logo-full hidden xl:block">
          <img src="/piccolo.svg" alt="Piccolo" class="logo-full__img logo-full__img--light" />
          <img src="/piccolo-white.svg" alt="Piccolo" class="logo-full__img logo-full__img--dark" />
        </div>
        <img src="/piccolo-p.svg" alt="Piccolo mark" class="logo-mark h-10 w-10 xl:hidden" />
      </div>
      <slot name="header-actions"></slot>
    </div>
  </header>
  <main class="mx-auto max-w-5xl px-6 py-10">
    <slot />
  </main>
</div>

<style>
  .logo-full__img {
    display: none;
    height: 32px;
    width: auto;
  }

  .logo-full__img--light {
    display: block;
  }

  :global([data-theme='dark']) .logo-full__img--light {
    display: none;
  }

  :global([data-theme='dark']) .logo-full__img--dark {
    display: block;
  }

  .logo-mark {
    transition: filter 150ms ease;
  }

  :global([data-theme='dark']) .logo-mark {
    filter: brightness(0) invert(1);
  }
</style>
