import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

export default defineConfig({
  plugins: [react()],
  server: { port: 5173, strictPort: true },
  // Pinned like the dev server: the API only allows these two origins.
  preview: { host: '127.0.0.1', port: 4173, strictPort: true },
})
