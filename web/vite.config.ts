import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import { resolve } from 'path'

export default defineConfig({
  plugins: [vue()],
  build: {
    // Keep compiled files separate from the Vue /assets route.  Besides
    // removing that route collision, changing the URL namespace guarantees
    // clients cannot reuse a previously cached Assets-page chunk.
    assetsDir: 'static',
  },
  resolve: {
    alias: { '@': resolve(__dirname, 'src') },
  },
  server: {
    port: Number(process.env.PORT) || 5173,
    proxy: {
      '/api': { target: 'http://localhost:8080', changeOrigin: true },
      '/healthz': { target: 'http://localhost:8080', changeOrigin: true },
      '/ws': { target: 'ws://localhost:8080', changeOrigin: true, ws: true },
      '/images': { target: 'http://localhost:8080', changeOrigin: true },
    },
  },
})
