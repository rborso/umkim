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
      '/api': {
        target: process.env.DCM_API_PROXY || 'http://198.51.100.10:30908',
        changeOrigin: true,
      },
      '/fleet/api': {
        target: process.env.DCM_API_PROXY || 'http://198.51.100.10:30908',
        changeOrigin: true,
      },
      '/fleet/auth': {
        target: process.env.DCM_API_PROXY || 'http://198.51.100.10:30908',
        changeOrigin: true,
      },
    },
  },
  build: {
    outDir: 'dist',
    emptyOutDir: true,
  },
});
