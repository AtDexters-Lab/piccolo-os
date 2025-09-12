<script lang="ts">
  import { api, apiProd, demo } from '@api/client';
  import { sessionStore, bootstrapSession } from '@stores/session';
  import { toast } from '@stores/ui';
  let password = '';
  let confirm = '';
  let error = '';
  let working = false;
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
      await bootstrapSession();
      toast('Admin created', 'success');
      window.location.hash = '/';
    } catch (e: any) {
      error = e?.message || 'Setup failed';
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
