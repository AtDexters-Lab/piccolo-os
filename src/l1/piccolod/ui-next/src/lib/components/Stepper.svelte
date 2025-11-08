<script lang="ts">
  import type { StepDefinition } from '$lib/types/wizard';

  export let steps: StepDefinition[] = [];
  export let activeId: string;
</script>

<ol class="flex flex-col gap-3 md:flex-row md:items-start md:gap-6">
  {#each steps as step, index}
    {@const status = step.id === activeId ? 'active' : steps.findIndex((s) => s.id === activeId) > index ? 'done' : 'pending'}
    <li class={`flex flex-1 items-start gap-3 rounded-2xl border px-4 py-3 ${status === 'active' ? 'border-accent bg-white shadow-lg' : 'border-white/40 bg-white/70'}`}>
      <div class={`mt-1 flex h-6 w-6 items-center justify-center rounded-full text-xs font-semibold ${status === 'done' ? 'bg-success/20 text-success' : status === 'active' ? 'bg-accent text-white' : 'bg-slate-200 text-slate-600'}`}>
        {index + 1}
      </div>
      <div>
        <p class={`text-sm font-semibold ${status === 'active' ? 'text-slate-900' : 'text-slate-700'}`}>{step.label}</p>
        {#if step.description}
          <p class="text-xs text-muted">{step.description}</p>
        {/if}
      </div>
    </li>
  {/each}
</ol>
