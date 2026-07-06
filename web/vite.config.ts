/// <reference types="vitest/config" />
import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

// https://vite.dev/config/
export default defineConfig({
  plugins: [react()],
  server: {
    proxy: {
      // Go server listens on :8991 (server/conf/app.toml -> [rpc] http_addr)
      '/api': 'http://localhost:8991',
      // SSR 详情页 /items/{id} full-navigation 转给 Go（镜像生产反代）
      '/items': 'http://localhost:8991',
    },
  },
  test: {
    environment: 'jsdom',
    globals: true,
    setupFiles: ['./vitest.setup.ts'],
  },
})
