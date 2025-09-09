<script lang="ts">
  import { toasts, removeToast } from '@stores/ui';
  export let timeout = 3000;
  $: if ($toasts.length) {
    const id = $toasts[$toasts.length - 1].id;
    const t = setTimeout(() => removeToast(id), timeout);
    // cleanup automatically when new toast arrives
  }
</script>

<div class="fixed bottom-4 right-4 space-y-2 z-50">
  {#each $toasts as t (t.id)}
    <div class="px-3 py-2 rounded shadow text-white text-sm"
         class:bg-green-600={t.type === 'success'}
         class:bg-red-600={t.type === 'error'}
         class:bg-slate-800={t.type === 'info'}>
      {t.message}
    </div>
  {/each}
  {#if !$toasts.length}
    <!-- no toasts -->
  {/if}
  
</div>

