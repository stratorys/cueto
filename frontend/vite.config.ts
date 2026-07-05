import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import tailwindcss from '@tailwindcss/vite'

// https://vite.dev/config/
export default defineConfig({
  plugins: [vue(), tailwindcss()],
  server: {
    // Allow importing the sibling cue/ dir (schema.cue?raw) which lives outside
    // the frontend project root.
    fs: { allow: [".."] },
  },
})
