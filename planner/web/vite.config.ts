import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'
import { defineConfig } from 'vite'

// https://vite.dev/config/
export default defineConfig({
  plugins: [react(), tailwindcss()],
  server: {
    port: 5190,          // 5173은 이 Mac에서 다른 프로젝트가 쓴다
    strictPort: true,
    host: true,          // 같은 WiFi의 폰에서 http://<Mac IP>:5190 으로 확인
    proxy: {
      '/api': 'http://127.0.0.1:8090',
    },
  },
  build: {
    // Go의 //go:embed는 패키지 밖(..)을 못 봐서 번들을 Go 패키지 안으로 낸다.
    outDir: '../internal/webui/dist',
    emptyOutDir: false,  // dist/.gitkeep 보존 — Makefile `web`이 assets/만 지운다
  },
})
