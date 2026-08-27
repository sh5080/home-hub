import { useEffect, useState, type ReactNode } from 'react'

/** 페이지 상단: 제목 + 우측 액션 */
export function PageHeader({ title, right, back }: { title: ReactNode; right?: ReactNode; back?: () => void }) {
  return (
    <header className="sticky top-0 z-10 flex items-center gap-2 border-b border-slate-200 bg-slate-50/90 px-4 py-3 backdrop-blur">
      {back && (
        <button onClick={back} aria-label="뒤로" className="-ml-1 rounded-lg p-1 text-slate-500 active:bg-slate-200">
          <svg viewBox="0 0 24 24" className="h-6 w-6" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
            <path d="M15 18l-6-6 6-6" />
          </svg>
        </button>
      )}
      <h1 className="flex-1 truncate text-xl font-bold">{title}</h1>
      {right}
    </header>
  )
}

/** 아래에서 올라오는 시트. 폰에서 모달 대용. */
export function Sheet({ open, onClose, title, children }: { open: boolean; onClose: () => void; title?: ReactNode; children: ReactNode }) {
  useEffect(() => {
    if (!open) return
    const onKey = (e: KeyboardEvent) => e.key === 'Escape' && onClose()
    document.addEventListener('keydown', onKey)
    document.body.style.overflow = 'hidden'
    return () => {
      document.removeEventListener('keydown', onKey)
      document.body.style.overflow = ''
    }
  }, [open, onClose])

  if (!open) return null
  return (
    <div className="fixed inset-0 z-40 flex items-end justify-center sm:items-center" onClick={onClose}>
      <div className="absolute inset-0 bg-black/40" />
      <div
        role="dialog"
        onClick={(e) => e.stopPropagation()}
        className="relative w-full max-w-lg rounded-t-2xl bg-white p-4 shadow-xl sm:rounded-2xl"
        style={{ paddingBottom: 'calc(1rem + env(safe-area-inset-bottom))' }}
      >
        <div className="mx-auto mb-3 h-1 w-10 rounded-full bg-slate-300 sm:hidden" />
        {title && <h2 className="mb-3 text-lg font-bold">{title}</h2>}
        {children}
      </div>
    </div>
  )
}

export function Button({ children, variant = 'primary', className = '', ...rest }: React.ButtonHTMLAttributes<HTMLButtonElement> & { variant?: 'primary' | 'ghost' | 'danger' }) {
  const base = 'rounded-xl px-4 py-2.5 text-sm font-semibold transition active:scale-[0.98] disabled:opacity-40'
  const v = {
    primary: 'bg-slate-900 text-white',
    ghost: 'bg-slate-100 text-slate-700',
    danger: 'bg-rose-50 text-rose-600',
  }[variant]
  return (
    <button className={`${base} ${v} ${className}`} {...rest}>
      {children}
    </button>
  )
}

export function Input(props: React.InputHTMLAttributes<HTMLInputElement>) {
  return (
    <input
      {...props}
      className={`w-full rounded-xl border border-slate-200 bg-white px-3 py-2.5 text-base outline-none focus:border-slate-400 ${props.className ?? ''}`}
    />
  )
}

/**
 * 비밀번호 입력. 기본은 가려두고, 눈 아이콘으로 잠깐 볼 수 있다.
 *
 * 보안상 손해가 아니다 — 값은 이미 DOM과 메모리에 있고, 토글은 화면에
 * 그리는지만 바꾼다. 실제 위험은 어깨너머로 보는 사람뿐이라 기본을 가린
 * 상태로 두고 사용자가 상황을 보고 누르게 한다.
 *
 * 오히려 도움이 된다: 최소 8자를 폰에서 안 보고 치면 오타가 잦고, 오타가
 * 잦으면 사람은 짧은 비밀번호를 고른다. 게다가 5회 실패하면 계정이 잠긴다.
 * NIST SP 800-63B도 같은 이유로 표시 옵션 제공을 권한다.
 */
export function PasswordInput({ dark, ...props }: React.InputHTMLAttributes<HTMLInputElement> & { dark?: boolean }) {
  const [shown, setShown] = useState(false)
  const base = dark
    ? 'w-full rounded-xl border border-slate-700 bg-slate-800 px-4 py-3 pr-12 text-base text-white outline-none focus:border-slate-400'
    : 'w-full rounded-xl border border-slate-200 bg-white px-3 py-2.5 pr-11 text-base outline-none focus:border-slate-400'
  return (
    <div className="relative">
      <input {...props} type={shown ? 'text' : 'password'} className={`${base} ${props.className ?? ''}`} />
      <button
        type="button"
        onClick={() => setShown(!shown)}
        // 폼 제출·포커스 이동을 건드리지 않는다
        onMouseDown={(e) => e.preventDefault()}
        tabIndex={-1}
        aria-label={shown ? '비밀번호 숨기기' : '비밀번호 보기'}
        className={`absolute inset-y-0 right-0 flex w-11 items-center justify-center ${dark ? 'text-slate-400' : 'text-slate-400'} active:opacity-60`}
      >
        {shown ? (
          <svg viewBox="0 0 24 24" className="h-5 w-5" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round">
            <path d="M3 3l18 18M10.6 10.7a2 2 0 002.8 2.8" />
            <path d="M9.4 5.2A9.5 9.5 0 0112 5c5 0 9 4.5 9 7 0 .9-.5 2-1.4 3.1M6.3 6.4C3.9 7.9 3 10.2 3 12c0 2.5 4 7 9 7 1.4 0 2.7-.4 3.8-.9" />
          </svg>
        ) : (
          <svg viewBox="0 0 24 24" className="h-5 w-5" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round">
            <path d="M3 12s3.5-7 9-7 9 7 9 7-3.5 7-9 7-9-7-9-7z" />
            <circle cx="12" cy="12" r="2.6" />
          </svg>
        )}
      </button>
    </div>
  )
}

export function Textarea(props: React.TextareaHTMLAttributes<HTMLTextAreaElement>) {
  return (
    <textarea
      {...props}
      className={`w-full rounded-xl border border-slate-200 bg-white px-3 py-2.5 text-base outline-none focus:border-slate-400 ${props.className ?? ''}`}
    />
  )
}

export function Field({ label, children }: { label: string; children: ReactNode }) {
  return (
    <label className="block">
      <span className="mb-1 block text-xs font-medium text-slate-500">{label}</span>
      {children}
    </label>
  )
}

export function Empty({ children }: { children: ReactNode }) {
  return <p className="py-10 text-center text-sm text-slate-400">{children}</p>
}


/** 목록이 들어올 자리를 미리 차지하는 뼈대. 화면이 덜컥 밀리지 않게 한다. */
export function SkeletonList({ rows = 3, className = '' }: { rows?: number; className?: string }) {
  return (
    <ul className={`space-y-2 ${className}`} aria-hidden>
      {Array.from({ length: rows }, (_, i) => (
        <li key={i} className="flex items-center gap-3 rounded-xl bg-white p-3 shadow-sm">
          <span className="h-6 w-6 shrink-0 animate-pulse rounded-full bg-slate-200" />
          <span className="h-3 flex-1 animate-pulse rounded bg-slate-200" style={{ maxWidth: `${70 - i * 12}%` }} />
        </li>
      ))}
    </ul>
  )
}

/** 담당자 아바타(이름 첫 글자) */
export function Avatar({ name, size = 'sm' }: { name: string; size?: 'sm' | 'md' }) {
  const s = size === 'sm' ? 'h-6 w-6 text-[11px]' : 'h-8 w-8 text-sm'
  const hue = [...name].reduce((h, c) => (h * 31 + c.charCodeAt(0)) % 360, 0)
  return (
    <span
      className={`inline-flex ${s} shrink-0 items-center justify-center rounded-full font-bold text-white`}
      style={{ backgroundColor: `hsl(${hue} 55% 50%)` }}
      title={name}
    >
      {name[0]}
    </span>
  )
}
