import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { BrowserRouter } from 'react-router'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import App from './App'
import './index.css'
import { startTheme } from './lib/theme'
import { registerSW } from './lib/push'

// 첫 렌더 전에 적용한다(나중에 켜면 한 번 번쩍인다).
startTheme()
registerSW()

// 주 경로는 SSE. 포커스 재조회는 iOS 가 백그라운드에서 스트림을 끊어서, 폴링은 스트림이 조용히 죽을 때 대비.
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
