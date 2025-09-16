<script lang="ts">
  import { api, apiProd, demo } from '@api/client';
  import { onMount } from 'svelte';
  import { sessionStore, bootstrapSession } from '@stores/session';
  import { toast } from '@stores/ui';
  let username = '';
  let password = '';
  let error = '';
  let working = false;
  onMount(async () => {
    if (demo) return;
    try {
      const init: any = await apiProd('/auth/initialized');
      if (!init?.initialized) {
        window.location.hash = '/setup';
      }
    } catch {}
  });
  async function signIn(path: string = '/auth/login') {
    error = '';
    working = true;
    try {
      if (demo && path !== '/auth/login') {
        // Demo-simulated errors (401/429) remain on demo endpoints
        await api(path, { method: 'GET' });
      } else {
        // Real login path
        await apiProd('/auth/login', { method: 'POST', body: JSON.stringify({ username, password }) });
      }
      await bootstrapSession();
      toast('Signed in', 'success');
      // Redirect to dashboard
      window.location.hash = '/';
    } catch (e: any) {
      error = e?.message || 'Sign in failed';
    } finally {
      working = false;
    }
  }
</script>

<h2 class="text-xl font-semibold mb-4">Sign in</h2>
{#if error}
  <p class="text-sm text-red-600 mb-2">{error}</p>
{/if}
<form class="bg-white rounded border p-4 space-y-3" on:submit|preventDefault={() => signIn()}>
  <label class="block text-sm">Username
    <input class="mt-1 w-full border rounded p-2 text-sm" bind:value={username} placeholder="admin" autocomplete="username" />
  </label>
  <label class="block text-sm">Password
    <input class="mt-1 w-full border rounded p-2 text-sm" type="password" bind:value={password} placeholder="••••••••" autocomplete="current-password" />
  </label>
  <div class="flex items-center gap-2">
    <button class="px-3 py-2 text-sm border rounded hover:bg-gray-50" disabled={working} type="submit">Sign in</button>
    {#if demo}
      <button class="px-3 py-2 text-sm border rounded hover:bg-gray-50" type="button" on:click={() => signIn('/auth/login_failed')}>Simulate 401</button>
      <button class="px-3 py-2 text-sm border rounded hover:bg-gray-50" type="button" on:click={() => signIn('/auth/login_rate_limited')}>Simulate 429</button>
    {/if}
  </div>
</form>
