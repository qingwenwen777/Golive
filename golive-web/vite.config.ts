import { defineConfig, type Plugin } from 'vite';
import react from '@vitejs/plugin-react';
import fs from 'node:fs';
import path from 'node:path';

const THEME_INIT_SRC = '/src/theme-init.js';

// index.html loads src/theme-init.js as a classic, render-blocking script so the
// theme is applied before first paint (the production CSP rules out an inline
// script). Vite only bundles module scripts, so in builds this emits the file
// as a content-hashed asset (nginx caches .js as immutable) and points the tag
// at it. The dev server serves the source file directly.
function themeInitScript(): Plugin {
  return {
    name: 'golive-theme-init',
    apply: 'build',
    buildStart() {
      this.emitFile({
        type: 'asset',
        name: 'theme-init.js',
        source: fs.readFileSync(path.resolve(__dirname, 'src/theme-init.js'), 'utf8'),
      });
    },
    transformIndexHtml: {
      order: 'post',
      handler(html, ctx) {
        const asset = Object.values(ctx.bundle ?? {}).find(
          (output) =>
            output.type === 'asset' &&
            (output.names?.includes('theme-init.js') || output.name === 'theme-init.js'),
        );
        if (!asset) throw new Error('theme-init.js was not emitted');
        return html.replace(`src="${THEME_INIT_SRC}"`, `src="/${asset.fileName}"`);
      },
    },
  };
}

export default defineConfig({
  plugins: [react(), themeInitScript()],
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
      // Only stream files go to the FLV server; /live/:id is an app route and
      // must fall through to the SPA (nginx does the same in production).
      '^/live/.*\\.(?:flv|m3u8|ts)$': {
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
