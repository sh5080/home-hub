import { Navigate, Outlet, Route, Routes } from 'react-router'
import { useQuery } from '@tanstack/react-query'
import { api, type User } from './api'
import Shell from './components/Shell'
import { useLiveSync } from './lib/useLiveSync'
import { SkeletonList } from './components/ui'
import Login from './pages/Login'
import Dashboard from './pages/Dashboard'
import Board from './pages/Board'
import Calendar from './pages/Calendar'
import Routines from './pages/Routines'
import CardPage from './pages/CardPage'

export default function App() {
  return (
    <Routes>
      <Route path="/login" element={<Login />} />
      <Route element={<RequireAuth />}>
        <Route element={<Shell />}>
          <Route index element={<Dashboard />} />
          <Route path="boards" element={<Board />} />
          <Route path="boards/:id" element={<Board />} />
          <Route path="cards/:id" element={<CardPage />} />
          <Route path="calendar" element={<Calendar />} />
          <Route path="routines" element={<Routines />} />
        </Route>
      </Route>
      <Route path="*" element={<Navigate to="/" replace />} />
    </Routes>
  )
}

// 로그인한 뒤에만 스트림을 연다 — 로그인 화면에서 열면 401로 재연결만 돈다.
function LiveOutlet() {
  useLiveSync()
  return <Outlet />
}

// 로그인 게이트. /api/me 가 401이면 api.ts 가 /login 으로 보낸다.
function RequireAuth() {
  const me = useQuery({
    queryKey: ['me'],
    queryFn: () => api.get<User>('/api/me'),
    retry: false,
    refetchInterval: false,
  })

  if (me.isPending) {
    return (
      <div className="mx-auto max-w-lg space-y-4 p-4">
        <div className="h-7 w-1/2 animate-pulse rounded bg-slate-200" />
        <SkeletonList rows={4} />
      </div>
    )
  }
  if (me.isError) return <Navigate to="/login" replace />
  return <LiveOutlet />
}
