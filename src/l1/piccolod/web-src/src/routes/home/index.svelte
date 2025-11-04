<script lang="ts">
  import SystemStatus from '@components/home/widgets/SystemStatus.svelte';
  import NetworkStatus from '@components/home/widgets/NetworkStatus.svelte';
  import StorageStatus from '@components/home/widgets/StorageStatus.svelte';
  import UpdatesStatus from '@components/home/widgets/UpdatesStatus.svelte';
  import PinnedDock from '@components/home/PinnedDock.svelte';
  import ActivityFeed from '@components/home/ActivityFeed.svelte';
  import Nudges from '@components/home/Nudges.svelte';
</script>

<div class="space-y-6">
  <header class="flex flex-col gap-2">
    <h2 class="text-2xl font-semibold text-text-primary">What matters now</h2>
    <p class="text-sm text-text-muted">System, network, storage, and updates widgets load independently so one outage never blocks another.</p>
  </header>

  <Nudges />

  <section class="grid grid-cols-1 gap-4 md:grid-cols-2 xl:grid-cols-4">
    <SystemStatus />
    <NetworkStatus />
    <StorageStatus />
    <UpdatesStatus />
  </section>

  <PinnedDock />

  <ActivityFeed />
</div>
