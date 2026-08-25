import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { BrowserRouter } from 'react-router'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import App from './App'
import './index.css'

// 주 경로는 SSE(useLiveSync)다. 아래 둘은 안전망:
//   - 창 포커스: iOS Safari가 백그라운드에서 EventSource를 끊으므로 돌아올 때 필요
//   - 주기 폴링: 스트림이 조용히 죽은 경우를 대비. SSE가 있으니 길게 잡는다
const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      refetchOnWindowFocus: true,
      refetchInterval: 120_000,
      staleTime: 10_000,
      retry: 1,
    },
  },
})

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <QueryClientProvider client={queryClient}>
      <BrowserRouter>
        <App />
      </BrowserRouter>
    </QueryClientProvider>
  </StrictMode>,
)
