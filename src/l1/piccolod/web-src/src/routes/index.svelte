<script lang="ts">
  import HealthPanel from '@components/panels/HealthPanel.svelte';
  import ServicesPanel from '@components/panels/ServicesPanel.svelte';
  import StoragePanel from '@components/panels/StoragePanel.svelte';
  import UpdatesPanel from '@components/panels/UpdatesPanel.svelte';
  import RemotePanel from '@components/panels/RemotePanel.svelte';
</script>

<h2 class="text-xl font-semibold mb-4">Dashboard</h2>
<div class="grid grid-cols-1 md:grid-cols-2 gap-4">
  <HealthPanel />
  <ServicesPanel />
  <StoragePanel />
  <UpdatesPanel />
  <RemotePanel />
</div>
