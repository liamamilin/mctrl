import preact from '@preact/preset-vite';
import { defineConfig, loadEnv } from 'vite';

export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, '.', '');
  const target = env.VITE_DEV_API_TARGET;
  const proxy = target
    ? {
        '/api': {
          target,
          changeOrigin: false,
          ws: true,
        },
      }
    : undefined;

  return {
    plugins: [preact()],
    server: {
      host: true,
      port: 5173,
      proxy,
    },
    preview: {
      host: true,
      port: 4173,
    },
    build: {
      outDir: 'dist',
      emptyOutDir: true,
    },
  };
});
