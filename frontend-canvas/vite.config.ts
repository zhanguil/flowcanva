import { defineConfig, loadEnv } from 'vite'
import vue from '@vitejs/plugin-vue'
import tailwindcss from '@tailwindcss/vite'
import { hostname } from 'node:os'

export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, process.cwd(), '')
  // This address is used by Vite on the host machine, never by the browser.
  // Browser API/upload requests stay relative to the page origin for LAN access.
  const apiTarget = env.VITE_API_TARGET || `http://${hostname()}:6789`
  return {
    base: '/canvas/',
    plugins: [vue(), tailwindcss()],
    server: {
      host: env.VITE_HOST || '0.0.0.0',
      port: Number(env.VITE_PORT || 5173),
      strictPort: true,
      proxy: {
        '/api': apiTarget,
        '/uploads': apiTarget,
      },
    },
    preview: {
      host: env.VITE_HOST || '0.0.0.0',
    },
  }
})
