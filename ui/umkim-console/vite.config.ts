import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';

export default defineConfig({
  plugins: [react()],
  base: '/fleet/',
  define: {
    'import.meta.env.VITE_BUILD_ID': JSON.stringify(
      process.env.VITE_BUILD_ID || new Date().toISOString().slice(0, 16),
    ),
  },
  server: {
    port: 5173,
    proxy: {
      '/fleet/api': {
        target: process.env.FLEET_API_PROXY || 'http://127.0.0.1:30912',
        changeOrigin: true,
        ws: true,
      },
      '/fleet/auth': {
        target: process.env.FLEET_API_PROXY || 'http://127.0.0.1:30912',
        changeOrigin: true,
        ws: true,
      },
    },
  },
  build: {
    outDir: 'dist',
    emptyOutDir: true,
  },
});
