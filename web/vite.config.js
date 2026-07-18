import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

const apiProxyTarget = process.env.VITE_API_PROXY_TARGET || 'http://localhost:8888'
const apiProxyOrigin = new URL(apiProxyTarget).origin

export default defineConfig({
  plugins: [vue()],
  build: {
    assetsDir: 'app-assets'
  },
  test: {
    environment: 'jsdom',
    testTimeout: 15000,
    setupFiles: ['./src/__tests__/setup.js'],
    exclude: ['node_modules/**', 'dist/**', 'e2e/**']
  },
  server: {
    proxy: {
      '/api': {
        target: apiProxyTarget,
        changeOrigin: true,
        configure(proxy) {
          proxy.on('proxyReq', (proxyReq) => {
            proxyReq.setHeader('Origin', apiProxyOrigin)
          })
        }
      }
    }
  }
})
