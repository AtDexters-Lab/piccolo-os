<script lang="ts">
  import type { StepDefinition } from '$lib/types/wizard';

  export let steps: StepDefinition[] = [];
  export let activeId: string;
</script>

<ol class="stepper grid gap-3 md:grid-cols-3 md:gap-4 xl:flex xl:flex-row xl:items-start xl:gap-6">
  {#each steps as step, index}
    {@const baseStatus = step.id === activeId ? 'active' : steps.findIndex((s) => s.id === activeId) > index ? 'done' : 'pending'}
    {@const stateOverride = step.state}
    {@const status = (() => {
      if (stateOverride === 'error' || stateOverride === 'blocked') return stateOverride;
      if (stateOverride === 'success' && step.id !== activeId) return 'done';
      return baseStatus;
    })()}
    {@const statusMessage = status === 'error' ? 'Requires attention' : status === 'blocked' ? 'Complete previous step' : ''}
    <li
      class={`stepper__item flex flex-1 items-center gap-3 border px-4 py-4 stepper__item--${status}`}
      aria-current={status === 'active' ? 'step' : undefined}
      data-step-status={status}
      title={statusMessage || undefined}
    >
      <div class={`stepper__bubble flex h-7 w-7 items-center justify-center text-xs font-semibold stepper__bubble--${status}`}>
        {index + 1}
      </div>
      <div class="flex flex-col">
        <p class={`stepper__label text-sm font-semibold stepper__label--${status}`}>{step.label}</p>
        {#if step.description}
          <p class="text-xs text-muted">{step.description}</p>
        {/if}
      </div>
      {#if statusMessage}
        <span class={`stepper__status-icon stepper__status-icon--${status}`} role="img" aria-label={statusMessage} title={statusMessage}>!</span>
      {/if}
    </li>
  {/each}
</ol>

<style>
  .stepper__item {
    border-radius: var(--radius-xl);
    border: 1px solid var(--card-border);
    background: var(--card-gradient);
    box-shadow: var(--shadow-soft);
    min-height: 92px;
    transition:
      background var(--motion-dur-fast) var(--motion-ease-standard),
      box-shadow var(--motion-dur-fast) var(--motion-ease-standard),
      border-color var(--motion-dur-fast) var(--motion-ease-standard);
  }

  .stepper__item--pending {
    opacity: 0.9;
    box-shadow: none;
    background: var(--stepper-pending-bg);
  }

  .stepper__item--done {
    border-color: rgb(var(--sys-success) / 0.35);
    background: var(--card-bg);
  }

  .stepper__bubble {
    border-radius: var(--radius-pill);
    font-variant-numeric: tabular-nums;
  }

  .stepper__bubble--pending {
    background: var(--stepper-pending-bg);
    color: rgb(var(--sys-ink-muted));
  }

  .stepper__bubble--active {
    background: var(--stepper-active-bg);
    color: rgb(var(--sys-on-accent));
    box-shadow: var(--stepper-active-shadow);
  }

  .stepper__bubble--done {
    background: var(--stepper-done-bg);
    color: rgb(var(--sys-success));
  }

  .stepper__bubble--error,
  .stepper__bubble--blocked {
    color: #fff;
  }

  .stepper__bubble--error {
    background: var(--stepper-error-bg);
    box-shadow: 0 10px 20px rgb(var(--sys-critical) / 0.35);
  }

  .stepper__bubble--blocked {
    background: var(--stepper-blocked-bg);
    color: rgb(var(--sys-on-warning));
    box-shadow: 0 8px 18px rgb(var(--sys-warning) / 0.25);
  }
  .stepper__label {
    font-variant-numeric: tabular-nums;
    color: rgb(var(--sys-ink));
  }

  .stepper__label--pending {
    color: rgb(var(--sys-ink-muted));
    font-weight: 500;
  }

  .stepper__label--active,
  .stepper__label--done {
    color: rgb(var(--sys-ink));
  }

  .stepper__status-icon {
    margin-left: auto;
    font-size: 1.1rem;
  }

  .stepper__status-icon--error {
    color: rgb(var(--sys-critical));
  }

  .stepper__status-icon--blocked {
    color: rgb(var(--sys-warning));
  }

  .sr-only {
    position: absolute;
    width: 1px;
    height: 1px;
    padding: 0;
    margin: -1px;
    overflow: hidden;
    clip: rect(0, 0, 0, 0);
    white-space: nowrap;
    border: 0;
  }

  @media (min-width: 768px) {
    .stepper__label {
      max-width: 22ch;
      white-space: nowrap;
      overflow: hidden;
      text-overflow: ellipsis;
    }
  }

  @media (max-width: 767px) {
    .stepper__label {
      white-space: normal;
    }
  }
</style>
