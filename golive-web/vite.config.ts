import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';
import path from 'node:path';

export default defineConfig({
  plugins: [react()],
  resolve: {
    alias: {
      '@': path.resolve(__dirname, './src'),
    },
  },
  server: {
    port: 5173,
    host: true,
    proxy: {
      '/api': {
        target: 'http://localhost:8080',
        changeOrigin: true,
      },
      '/ws': {
        target: 'http://localhost:8081',
        changeOrigin: true,
        ws: true,
      },
      '/live': {
        target: 'http://localhost:8082',
        changeOrigin: true,
        ws: false,
      },
    },
  },
  build: {
    rollupOptions: {
      output: {
        manualChunks(id) {
          const normalizedId = id.replace(/\\/g, '/');
          if (!normalizedId.includes('/node_modules/')) return;

          if (
            normalizedId.includes('/react/') ||
            normalizedId.includes('/react-dom/') ||
            normalizedId.includes('/react-router/') ||
            normalizedId.includes('/react-router-dom/') ||
            normalizedId.includes('/@remix-run/') ||
            normalizedId.includes('/lucide-react/') ||
            normalizedId.includes('/scheduler/') ||
            normalizedId.includes('/use-sync-external-store/')
          ) {
            return 'vendor-react';
          }
          if (normalizedId.includes('/@tanstack/')) return 'vendor-query';
          if (normalizedId.includes('/i18next/') || normalizedId.includes('/react-i18next/')) {
            return 'vendor-i18n';
          }
          if (normalizedId.includes('/@radix-ui/') || normalizedId.includes('/sonner/')) {
            return 'vendor-ui';
          }
          if (normalizedId.includes('/mpegts.js/')) return 'vendor-player';
          if (normalizedId.includes('/axios/')) return 'vendor-http';

          return undefined;
        },
      },
    },
  },
});
