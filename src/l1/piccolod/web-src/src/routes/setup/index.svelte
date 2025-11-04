<script lang="ts">
  import { onMount } from 'svelte';
  import { api, apiProd, demo, type ErrorResponse } from '@api/client';
  import { bootstrapSession } from '@stores/session';
  import { toast } from '@stores/ui';
  let password = '';
  let confirm = '';
  let error = '';
  let working = false;
  onMount(async () => {
    if (demo) return;
    try {
      const init = await apiProd<{ initialized: boolean }>('/auth/initialized');
      if (init?.initialized) {
        toast('Admin already exists. Sign in instead.', 'info');
        window.location.hash = '/login';
      }
    } catch (err: any) {
      const e = err as ErrorResponse | undefined;
      const message = e?.message || '';
      if (e?.code === 423 || /locked/i.test(message)) {
        toast('Encrypted volumes are locked. Unlock to continue.', 'info');
        window.location.hash = '/lock';
      }
    }
  });
  async function onSetup() {
    error = '';
    if (password.length < 8 || password !== confirm) {
      error = 'Passwords must match and be at least 8 characters.';
      return;
    }
    working = true;
    try {
      // Always use real API for admin setup
      await apiProd('/auth/setup', { method: 'POST', body: JSON.stringify({ password }) });
      // Initialize crypto right away with the same password (idempotent)
      try {
        await apiProd('/crypto/setup', { method: 'POST', body: JSON.stringify({ password }) });
      } catch (e: any) {
        // Ignore if already initialized
        if (!/already initialized/i.test(e?.message || '')) throw e;
      }
      await bootstrapSession();
      toast('Admin created', 'success');
      window.location.hash = '/';
    } catch (e: any) {
      const err = e as ErrorResponse | undefined;
      const message = err?.message || 'Setup failed';
      if (err?.code === 423 || /locked/i.test(message)) {
        toast('Encrypted volumes are locked. Unlock to continue.', 'info');
        window.location.hash = '/lock';
        return;
      }
      if (/already initialized/i.test(message)) {
        toast('Admin already exists. Sign in instead.', 'info');
        window.location.hash = '/login';
        return;
      }
      error = message;
    } finally {
      working = false;
    }
  }
</script>

<h2 class="text-xl font-semibold mb-4">Create Admin</h2>
{#if error}
  <p class="text-sm text-red-600 mb-2">{error}</p>
{/if}
<form class="bg-white rounded border p-4 space-y-3" on:submit|preventDefault={onSetup}>
  <label class="block text-sm">Password
    <input class="mt-1 w-full border rounded p-2 text-sm" type="password" bind:value={password} placeholder="New password" autocomplete="new-password" />
  </label>
  <label class="block text-sm">Confirm password
    <input class="mt-1 w-full border rounded p-2 text-sm" type="password" bind:value={confirm} placeholder="Confirm password" autocomplete="new-password" />
  </label>
  <button class="px-3 py-2 text-sm border rounded hover:bg-gray-50" disabled={working} type="submit">Create Admin</button>
</form>
