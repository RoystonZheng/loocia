import { configDefaults, defineConfig } from 'vitest/config'
import react from '@vitejs/plugin-react'

const apiProxyTarget = process.env.AI_TOOL_WEB_API_PROXY ?? 'http://localhost:8991'

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
      // Go server listens on :8991 by default (server/conf/app.toml -> [rpc] http_addr).
      '/api': apiProxyTarget,
      // SSR 详情页 /items/{id} full-navigation 转给 Go（镜像生产反代）
      '/items': apiProxyTarget,
    },
  },
  test: {
    environment: 'jsdom',
    globals: true,
    setupFiles: ['./vitest.setup.ts'],
    exclude: [...configDefaults.exclude, 'e2e/**'],
  },
})
