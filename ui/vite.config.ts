import { defineConfig } from 'vitest/config'
import { svelte } from '@sveltejs/vite-plugin-svelte'

export default defineConfig(({ mode }) => ({
  plugins: [svelte()],
  resolve: {
    conditions: mode === 'test' ? ['browser'] : [],
  },
  test: {
    environment: 'jsdom',
    globals: true,
    include: ['src/**/*.{test,spec}.ts'],
  },
  build: {
    outDir: '../internal/frontendassets/webdist/dist',
    emptyOutDir: true,
  }
}))
