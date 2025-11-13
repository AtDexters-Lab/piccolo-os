<script lang="ts">
  import Button from '$lib/components/ui/Button.svelte';
  import { platformState, platformController } from '$lib/stores/platform';
  import { acknowledgeStaleness } from '$lib/api/setup';
  import type { ApiError } from '$lib/api/http';
  import { goto } from '$app/navigation';

  let errorMessage = '';

  $: session = $platformState.session;
  $: passwordStale = session?.passwordStale;
  $: recoveryStale = session?.recoveryStale;
  $: showBanner = Boolean(passwordStale || recoveryStale);

  function handleGoToSetup() {
    goto('/setup?focus=recovery');
  }
</script>

{#if showBanner}
  <div class="staleness-banner">
    <div class="staleness-banner__copy">
      <p class="staleness-banner__title">Security reminder</p>
      <div class="staleness-banner__body">
        {#if passwordStale}<p>Admin password was reset via recovery key—rotate it or acknowledge the risk.</p>{/if}
        {#if recoveryStale}<p>Recovery key was used and is considered stale until you generate a new one.</p>{/if}
      </div>
    </div>
    <div class="staleness-banner__actions">
      {#if errorMessage}
        <p class="staleness-banner__error">{errorMessage}</p>
      {/if}
      <Button variant="primary" size="compact" on:click={handleGoToSetup}>
        Go to setup
      </Button>
      <Button variant="ghost" href="/docs/foundation.md" size="compact" target="_blank">Details</Button>
    </div>
  </div>
{/if}

<style>
  .staleness-banner {
    border-radius: var(--radius-xl);
    border: 1px solid rgb(var(--sys-warning) / 0.35);
    background: rgb(var(--sys-warning) / 0.16);
    padding: 1.5rem;
    display: flex;
    flex-direction: column;
    gap: 1rem;
    margin-bottom: 1.5rem;
  }

  @media (min-width: 768px) {
    .staleness-banner {
      flex-direction: row;
      justify-content: space-between;
      align-items: center;
      gap: 1.5rem;
    }
  }

  .staleness-banner__title {
    font-weight: 600;
    color: rgb(var(--sys-on-warning));
    margin-bottom: 0.35rem;
  }

  .staleness-banner__body p {
    margin: 0;
    color: rgb(var(--sys-on-warning));
    font-size: 0.9rem;
  }

  .staleness-banner__actions {
    display: flex;
    flex-direction: column;
    gap: 0.35rem;
    align-items: flex-start;
  }

  @media (min-width: 768px) {
    .staleness-banner__actions {
      align-items: flex-end;
    }
  }

  .staleness-banner__error {
    color: rgb(var(--sys-critical));
    font-size: 0.85rem;
  }

</style>
