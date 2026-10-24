import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import path from 'path'

import { cabinetDevApi } from './mock/devApi'

export default defineConfig({
  plugins: [react(), ...(process.env.CABINET_DEV_MOCK === '1' ? [cabinetDevApi()] : [])],
  resolve: {
    alias: {
      '@': path.resolve(__dirname, './src'),
    },
  },
  // SPA обслуживается на /cabinet/*; все ассеты должны ссылаться на /cabinet/…
  base: '/cabinet/',
  build: {
    outDir: '../../internal/cabinet/web/dist',
    emptyOutDir: true,
    sourcemap: false,
    rollupOptions: {
      output: {
        manualChunks: {
          vendor: ['react', 'react-dom'],
          router: ['react-router-dom'],
          query: ['@tanstack/react-query'],
        },
      },
    },
  },
  server: {
    port: 5173,
    proxy: {
      // В dev-режиме проксируем API-запросы на бот-сервер.
      '/cabinet/api': {
        target: 'http://localhost:8080',
        changeOrigin: true,
      },
    },
  },
})
