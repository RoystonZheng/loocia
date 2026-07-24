/// <reference types="vitest/config" />
import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

// https://vite.dev/config/
export default defineConfig({
  plugins: [react()],
  build: {
    // 老浏览器(微信内置/老 iOS Safari)不认 `@media (width<=600px)` range 语法,
    // 会整段忽略手机断点导致不自适应。锁到老 target,强制输出老式 `max-width:` 语法。
    cssTarget: ['chrome87', 'safari13.1', 'firefox78', 'edge88'],
  },
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
