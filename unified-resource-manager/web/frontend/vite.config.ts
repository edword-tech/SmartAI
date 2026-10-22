import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

// 开发时 API 代理目标；未启动后端会导致 ECONNREFUSED，请先运行: make -C ../.. run 或 go run ./cmd/main.go -port 8080
const proxyTarget = process.env.VITE_PROXY_TARGET || 'http://127.0.0.1:8080'

export default defineConfig({
  plugins: [react()],
  server: {
    port: 3000,
    proxy: {
      '/api': {
        target: proxyTarget,
        changeOrigin: true,
        configure: (proxy) => {
          proxy.on('error', (err, _req, _res) => {
            console.warn('[vite] API proxy error (is backend running?):', err.message)
          })
        },
      },
    },
  },
  preview: {
    port: 3001,
    proxy: {
      '/api': {
        target: proxyTarget,
        changeOrigin: true,
      },
    },
  },
  build: {
    outDir: 'dist',
    assetsDir: 'assets',
  },
})
