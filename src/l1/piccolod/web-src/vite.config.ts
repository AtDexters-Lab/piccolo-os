import { defineConfig } from 'vite';
import { svelte } from '@sveltejs/vite-plugin-svelte';
import sveltePreprocess from 'svelte-preprocess';
import path from 'node:path';

// https://vitejs.dev/config/
export default defineConfig(({ mode }) => {
  const demo = process.env.VITE_API_DEMO === '1' || mode === 'demo';
  return {
    plugins: [svelte({ preprocess: sveltePreprocess() })],
    build: {
      outDir: path.resolve(__dirname, '../web'),
      emptyOutDir: true,
    },
    resolve: {
      alias: {
        '@api': path.resolve(__dirname, 'src/api'),
        '@components': path.resolve(__dirname, 'src/components'),
        '@routes': path.resolve(__dirname, 'src/routes'),
        '@stores': path.resolve(__dirname, 'src/stores'),
        '@lib': path.resolve(__dirname, 'src/lib'),
      },
    },
    define: {
      __DEMO__: JSON.stringify(demo),
    },
    server: {
      port: 5173,
    },
  };
});
