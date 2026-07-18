import { defineConfig } from 'vite'
import uniModule from '@dcloudio/vite-plugin-uni'

const uni =
  typeof uniModule === 'function'
    ? uniModule
    : typeof uniModule.default === 'function'
      ? uniModule.default
      : uniModule.default.default

const apiProxyTarget = process.env.VITE_API_PROXY_TARGET || 'http://localhost:8888'
const apiProxyOrigin = new URL(apiProxyTarget).origin

export default defineConfig({
  plugins: [uni()],
  css: {
    preprocessorOptions: {
      scss: {
        silenceDeprecations: ['legacy-js-api']
      }
    }
  },
  server: {
    host: '0.0.0.0',
    port: 5190,
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
