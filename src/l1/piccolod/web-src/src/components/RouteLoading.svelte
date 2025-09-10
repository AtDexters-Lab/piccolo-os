<script lang="ts">
  export let message: string = 'Loading…';
  // Keep minimal to avoid layout shifts
</script>

<div class="p-4 text-sm text-gray-600" aria-busy="true" aria-live="polite">
  {message}
</div>

