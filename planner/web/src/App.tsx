import { Navigate, Outlet, Route, Routes } from 'react-router'
import { useQuery } from '@tanstack/react-query'
import { api, type User } from './api'
import Shell from './components/Shell'
import { ConfirmProvider } from './components/Confirm'
import { useLiveSync } from './lib/useLiveSync'
import { SkeletonList } from './components/ui'
import Login from './pages/Login'
import Dashboard from './pages/Dashboard'
import Board from './pages/Board'
import Calendar from './pages/Calendar'
import Routines from './pages/Routines'
import Babyfood from './pages/Babyfood'
import Care from './pages/Care'
import Notify from './pages/Notify'
import Family from './pages/Family'
import Finance from './pages/Finance'
import FinanceGoals from './pages/FinanceGoals'
import BabyfoodStock from './pages/BabyfoodStock'
import BabyfoodFoods from './pages/BabyfoodFoods'
import CardPage from './pages/CardPage'
import Diary from './pages/Diary'
import DiaryEntry from './pages/DiaryEntry'

export default function App() {
  return (
    <ConfirmProvider>
    <Routes>
      <Route path="/login" element={<Login />} />
      <Route element={<RequireAuth />}>
        <Route element={<Shell />}>
          <Route index element={<Dashboard />} />
          <Route path="boards" element={<Board />} />
          <Route path="boards/:id" element={<Board />} />
          <Route path="cards/:id" element={<CardPage />} />
          <Route path="calendar" element={<Calendar />} />
          <Route path="diary" element={<Diary />} />
          <Route path="diary/:id" element={<DiaryEntry />} />
          <Route path="routines" element={<Routines />} />
          <Route path="care" element={<Care />} />
          <Route path="notify" element={<Notify />} />
          <Route path="family" element={<Family />} />
          <Route path="finance" element={<Finance />} />
          <Route path="finance/goals" element={<FinanceGoals />} />
          <Route path="babyfood" element={<Babyfood />} />
          <Route path="babyfood/stock" element={<BabyfoodStock />} />
          <Route path="babyfood/foods" element={<BabyfoodFoods />} />
        </Route>
      </Route>
      <Route path="*" element={<Navigate to="/" replace />} />
    </Routes>
    </ConfirmProvider>
  )
}

// 로그인한 뒤에만 스트림을 연다(로그인 화면에서 열면 401 재연결만 돈다).
function LiveOutlet() {
  useLiveSync()
  return <Outlet />
}

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
        <div className="h-7 w-1/2 animate-pulse rounded bg-line" />
        <SkeletonList rows={4} />
      </div>
    )
  }
  if (me.isError) return <Navigate to="/login" replace />
  return <LiveOutlet />
}
