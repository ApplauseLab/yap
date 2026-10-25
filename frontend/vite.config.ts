/// <reference types="vitest" />
import {defineConfig} from 'vite'
import react from '@vitejs/plugin-react'

// https://vitejs.dev/config/
export default defineConfig({
  // plugin-react's Fast Refresh preamble breaks under Vitest; esbuild's
  // automatic JSX runtime is enough for tests.
  plugins: process.env.VITEST ? [] : [react()],
  esbuild: process.env.VITEST ? { jsx: 'automatic' } : undefined,
  test: {
    environment: 'jsdom',
    setupFiles: ['./src/test/setup.ts'],
    css: false,
  },
})
