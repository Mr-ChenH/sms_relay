import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

export default defineConfig({
  plugins: [vue()],
  cacheDir: 'node_modules/.vite',
  optimizeDeps: {
    include: [
      '@schedule-x/vue',
      '@schedule-x/calendar',
      '@schedule-x/events-service',
      'temporal-polyfill',
      'temporal-polyfill/global'
    ]
  },
  server: {
    port: 5173,
    proxy: {
      '/api': 'http://localhost:8080'
    }
  }
})
