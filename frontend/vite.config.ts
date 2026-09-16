/// <reference types="vitest/config" />
import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';

export default defineConfig({
  plugins: [react()],
  server: {
    port: 8088,
    host: 'localhost',
    strictPort: true,
  },
  preview: {
    port: 8088,
    host: 'localhost',
    strictPort: true,
  },
  test: {
    environment: 'jsdom',
    setupFiles: ['./src/setupTests.ts'],
    // vitest は既定で CSS の import を空文字列に差し替える。その判定は `app.css?raw` のような
    // raw import にも一致するため、規則のテキストを検証するテスト (src/app.css.test.ts) が
    // 中身を読めない。raw import は文字列でありスタイルを注入しないので、そこだけ通す。
    css: { include: [/\.css\?raw$/] },
  },
});
