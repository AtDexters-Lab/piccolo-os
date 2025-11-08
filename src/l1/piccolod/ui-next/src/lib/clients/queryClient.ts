import { QueryClient } from '@tanstack/svelte-query';

export const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 5_000,
      refetchOnWindowFocus: false,
      retry: 1
    }
  }
});
