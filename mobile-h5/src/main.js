import { createSSRApp } from 'vue'
import App from './App.vue'

const mpBuildMarker = 'IMAGE_AGENT_MP_BUILD no-urlsearchparams-v2'

// #ifdef MP-WEIXIN
console.info(mpBuildMarker)
// #endif

export function createApp() {
  const app = createSSRApp(App)
  return {
    app
  }
}
