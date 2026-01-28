import path from "path"
import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'

export default defineConfig({
  plugins: [react(), tailwindcss()],
  resolve: {
    alias: {
      "@": path.resolve(__dirname, "./src"),
      "@i18n-manifest": path.resolve(__dirname, "../internal/i18n/languages.json"),
    },
  },
  server: {
    proxy: {
      '/api': 'http://localhost:8080',
      '/packages.json': 'http://localhost:8080',
      '/p2': 'http://localhost:8080',
      '/dist': 'http://localhost:8080',
    },
  },
})
