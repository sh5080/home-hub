import { useState, type FormEvent } from 'react'
import { useNavigate } from 'react-router'
import { useQueryClient } from '@tanstack/react-query'
import { api, ApiError, type User } from '../api'
import { PasswordInput } from '../components/ui'

export default function Login() {
  const nav = useNavigate()
  const qc = useQueryClient()
  const [name, setName] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)

  async function submit(e: FormEvent) {
    e.preventDefault()
    setBusy(true)
    setError(null)
    try {
      const user = await api.post<User>('/api/login', { name, password })
      qc.setQueryData(['me'], user)
      nav('/', { replace: true })
    } catch (err) {
      setError(err instanceof ApiError ? err.message : '로그인에 실패했어요')
    } finally {
      setBusy(false)
    }
  }

  return (
    // 로그인은 테마와 무관하게 늘 어둡다(토큰을 쓰면 다크에서 글자가 사라진다).
    <div className="flex min-h-full flex-col justify-center bg-slate-900 px-6 py-12 text-white">
      <div className="mx-auto w-full max-w-sm">
        <h1 className="text-3xl font-bold tracking-tight">단아네 플래너</h1>
        <p className="mt-2 text-slate-400">칸반 · 캘린더 · 주간 루틴</p>

        <form onSubmit={submit} className="mt-10 space-y-4">
          <label className="block">
            <span className="text-sm text-slate-300">이름</span>
            <input
              autoFocus
              autoComplete="username"
              value={name}
              onChange={(e) => setName(e.target.value)}
              className="mt-1 w-full rounded-xl border border-slate-700 bg-slate-800 px-4 py-3 text-base text-white outline-none focus:border-slate-400"
            />
          </label>
          <label className="block">
            <span className="text-sm text-slate-300">비밀번호</span>
            <PasswordInput
              dark
              autoComplete="current-password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              className="mt-1"
            />
          </label>

          {error && <p className="text-sm text-rose-400">{error}</p>}

          <button
            type="submit"
            disabled={busy || !name || !password}
            className="w-full rounded-xl bg-white py-3 text-base font-semibold text-slate-900 transition disabled:opacity-40"
          >
            {busy ? '확인 중…' : '들어가기'}
          </button>
        </form>

        <p className="mt-6 text-center text-xs leading-relaxed text-slate-400">
          비밀번호를 잊었다면 다른 가족에게 부탁하세요.<br />
          설정 → 가족 → 비밀번호 재설정
        </p>
      </div>
    </div>
  )
}
