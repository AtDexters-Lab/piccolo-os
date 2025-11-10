<script lang="ts">
  import type { StepDefinition } from '$lib/types/wizard';

  export let steps: StepDefinition[] = [];
  export let activeId: string;
</script>

<ol class="stepper flex flex-col gap-3 md:flex-row md:items-start md:gap-6">
  {#each steps as step, index}
    {@const status = step.id === activeId ? 'active' : steps.findIndex((s) => s.id === activeId) > index ? 'done' : 'pending'}
    <li class={`stepper__item flex flex-1 items-start gap-3 border px-4 py-3 stepper__item--${status}`}>
      <div class={`stepper__bubble mt-1 flex h-6 w-6 items-center justify-center text-xs font-semibold stepper__bubble--${status}`}>
        {index + 1}
      </div>
      <div>
        <p class={`stepper__label text-sm font-semibold stepper__label--${status}`}>{step.label}</p>
        {#if step.description}
          <p class="text-xs text-muted">{step.description}</p>
        {/if}
      </div>
    </li>
  {/each}
</ol>

<style>
  .stepper__item {
    border-radius: var(--radius-xl);
    border-color: rgb(var(--sys-ink) / 0.08);
    background: rgb(var(--sys-surface-variant) / 0.9);
    box-shadow: var(--shadow-soft);
    transition: background var(--motion-dur-fast) var(--motion-ease-standard);
  }

  .stepper__item--pending {
    opacity: 0.85;
    box-shadow: none;
    background: rgb(var(--sys-surface-muted) / 0.65);
  }

  .stepper__item--done {
    border-color: rgb(var(--sys-success) / 0.35);
    background: rgb(var(--sys-surface-variant) / 0.95);
  }

  .stepper__bubble {
    border-radius: var(--radius-pill);
  }

  .stepper__bubble--pending {
    background: rgb(var(--sys-surface-muted));
    color: rgb(var(--sys-ink-muted));
  }

  .stepper__bubble--active {
    background: rgb(var(--sys-accent));
    color: rgb(var(--sys-on-accent));
    box-shadow: 0 10px 20px rgb(var(--sys-accent) / 0.35);
  }

  .stepper__bubble--done {
    background: rgb(var(--sys-success) / 0.2);
    color: rgb(var(--sys-success));
  }

  .stepper__label--pending {
    color: rgb(var(--sys-ink-muted));
  }

  .stepper__label--active,
  .stepper__label--done {
    color: rgb(var(--sys-ink));
  }
</style>
