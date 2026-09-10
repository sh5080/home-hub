import { useEffect, useState, type ReactNode } from 'react'

export function PageHeader({ title, right, back, dropdown }: {
  title: ReactNode
  right?: ReactNode
  back?: () => void
  /** 헤더 아래 매달려 내용을 덮는 것(검색 결과 등). 흐름에서 빠져 본문을 밀지 않는다. */
  dropdown?: ReactNode
}) {
  return (
    <header className="sticky top-0 z-20 flex items-center gap-2 border-b border-line bg-canvas/90 px-4 py-3 backdrop-blur">
      {back && (
        <button onClick={back} aria-label="뒤로" className="-ml-1 rounded-lg p-1 text-muted active:bg-line">
          <svg viewBox="0 0 24 24" className="h-6 w-6" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
            <path d="M15 18l-6-6 6-6" />
          </svg>
        </button>
      )}
      <h1 className="flex-1 truncate text-xl font-bold">{title}</h1>
      {right}
      {dropdown && <div className="absolute inset-x-0 top-full">{dropdown}</div>}
    </header>
  )
}

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
        className="relative w-full max-w-lg rounded-t-3xl bg-surface p-4 shadow-xl sm:rounded-2xl"
        style={{ paddingBottom: 'calc(1rem + env(safe-area-inset-bottom))' }}
      >
        <div className="mx-auto mb-3 h-1 w-10 rounded-full bg-line sm:hidden" />
        {title && <h2 className="mb-3 text-lg font-bold">{title}</h2>}
        {children}
      </div>
    </div>
  )
}

export function Button({ children, variant = 'primary', className = '', ...rest }: React.ButtonHTMLAttributes<HTMLButtonElement> & { variant?: 'primary' | 'ghost' | 'danger' }) {
  const base = 'rounded-xl px-4 py-2.5 text-sm font-semibold transition active:scale-[0.98] disabled:opacity-40'
  const v = {
    primary: 'bg-accent text-accent-ink',
    ghost: 'bg-surface-2 text-ink-2',
    danger: 'bg-rose-50 text-rose-600',
  }[variant]
  return (
    <button className={`${base} ${v} ${className}`} {...rest}>
      {children}
    </button>
  )
}

// 켜짐/꺼짐. 버튼 글자가 '할 일'인지 '상태'인지 헷갈리지 않게 상태는 스위치로 보인다.
export function Switch({ on, onChange, label, size = 'md' }: { on: boolean; onChange: (v: boolean) => void; label?: string; size?: 'sm' | 'md' }) {
  const sm = size === 'sm'
  return (
    <button
      type="button"
      onClick={() => onChange(!on)}
      role="switch"
      aria-checked={on}
      aria-label={label}
      className={`relative shrink-0 rounded-full transition ${sm ? 'h-5 w-9' : 'h-7 w-12'} ${on ? 'bg-emerald-500' : 'bg-line'}`}
    >
      <span className={`absolute top-0.5 rounded-full bg-white shadow transition-all ${sm ? 'h-4 w-4' : 'h-6 w-6'} ${on ? (sm ? 'left-[18px]' : 'left-[22px]') : 'left-0.5'}`} />
    </button>
  )
}

// React 19 는 함수 컴포넌트도 ref 를 prop 으로 받는다(forwardRef 불필요).
export function Input(props: React.InputHTMLAttributes<HTMLInputElement> & { ref?: React.Ref<HTMLInputElement> }) {
  return (
    <input
      {...props}
      className={`w-full rounded-xl border border-line bg-surface px-3 py-2.5 text-base outline-none focus:border-ghost ${props.className ?? ''}`}
    />
  )
}

/** 비밀번호 입력. 기본은 가리고 눈 아이콘으로 잠깐 본다. */
export function PasswordInput({ dark, ...props }: React.InputHTMLAttributes<HTMLInputElement> & { dark?: boolean }) {
  const [shown, setShown] = useState(false)
  const base = dark
    ? 'w-full rounded-xl border border-line bg-surface-2 px-4 py-3 pr-12 text-base text-white outline-none focus:border-ghost'
    : 'w-full rounded-xl border border-line bg-surface px-3 py-2.5 pr-11 text-base outline-none focus:border-ghost'
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
        className={`absolute inset-y-0 right-0 flex w-11 items-center justify-center ${dark ? 'text-faint' : 'text-faint'} active:opacity-60`}
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
      className={`w-full rounded-xl border border-line bg-surface px-3 py-2.5 text-base outline-none focus:border-ghost ${props.className ?? ''}`}
    />
  )
}

export function Field({ label, children }: { label: string; children: ReactNode }) {
  return (
    <label className="block">
      <span className="mb-1 block text-xs font-medium text-muted">{label}</span>
      {children}
    </label>
  )
}

export function Empty({ children }: { children: ReactNode }) {
  return <p className="py-10 text-center text-sm text-faint">{children}</p>
}


export function SkeletonList({ rows = 3, className = '' }: { rows?: number; className?: string }) {
  return (
    <ul className={`space-y-2 ${className}`} aria-hidden>
      {Array.from({ length: rows }, (_, i) => (
        <li key={i} className="flex items-center gap-3 rounded-xl bg-surface p-3 shadow-sm">
          <span className="h-6 w-6 shrink-0 animate-pulse rounded-full bg-line" />
          <span className="h-3 flex-1 animate-pulse rounded bg-line" style={{ maxWidth: `${70 - i * 12}%` }} />
        </li>
      ))}
    </ul>
  )
}

/** 접었다 펴는 구획. 기본은 접힘. */
export function Collapsible({ title, defaultOpen = false, children }: {
  title: string
  defaultOpen?: boolean
  children: ReactNode
}) {
  const [open, setOpen] = useState(defaultOpen)
  return (
    <div className="rounded-xl border border-line">
      <button
        onClick={() => setOpen(!open)}
        aria-expanded={open}
        className="flex w-full items-center justify-between px-3 py-2.5 text-left active:bg-canvas"
      >
        <span className="text-xs font-medium text-muted">{title}</span>
        <svg
          viewBox="0 0 24 24"
          className={`h-4 w-4 text-faint transition-transform ${open ? 'rotate-90' : ''}`}
          fill="none" stroke="currentColor" strokeWidth="2.4" strokeLinecap="round" strokeLinejoin="round"
        >
          <path d="M9 6l6 6-6 6" />
        </svg>
      </button>
      <div className="px-3 pb-3" hidden={!open}>{children}</div>
    </div>
  )
}

// 이름 두 글자 뱃지. 높이는 그대로 두고 옆으로만 늘린다(줄 높이를 밀지 않게).
export function Avatar({ name, size = 'sm' }: { name: string; size?: 'sm' | 'md' }) {
  const s = size === 'sm' ? 'h-5 px-1.5 text-[10px]' : 'h-7 px-2 text-xs'
  const hue = [...name].reduce((h, c) => (h * 31 + c.charCodeAt(0)) % 360, 0)
  return (
    <span
      className={`inline-flex ${s} shrink-0 items-center justify-center rounded-full font-bold text-white`}
      style={{ backgroundColor: `hsl(${hue} 55% 50%)` }}
      title={name}
    >
      {[...name].slice(0, 2).join('')}
    </span>
  )
}

/** 시각 칸('HH:MM'). ±10/±30 버튼 — 기기 휠은 조금 옮기기가 번거롭다. */
export function TimeField({ value, fallback = '12:00', onChange }: {
  value: string
  /** 값이 비었을 때 대신 보여주고, 버튼이 기준으로 삼을 시각 */
  fallback?: string
  onChange: (v: string) => void
}) {
  // 빈 칸('--:--') 대신 따르는 기본값을 보여준다.
  const shown = value || fallback
  const shift = (min: number) => {
    const [h, m] = shown.split(':').map(Number)
    const t = ((h * 60 + m + min) % 1440 + 1440) % 1440
    const two = (n: number) => String(n).padStart(2, '0')
    onChange(`${two(Math.floor(t / 60))}:${two(t % 60)}`)
  }
  const step = (min: number, label: string) => (
    <button
      type="button"
      onClick={() => shift(min)}
      className="h-10 shrink-0 rounded-lg bg-surface-2 px-2 text-xs font-semibold text-ink-2 active:bg-line"
    >
      {label}
    </button>
  )
  return (
    <div className="flex items-center gap-1.5">
      {step(-30, '−30')}
      {step(-10, '−10')}
      <input
        type="time"
        value={shown}
        onChange={(e) => onChange(e.target.value || shown)}
        className="h-10 min-w-0 flex-1 rounded-xl border border-line bg-surface px-2 text-center text-base outline-none focus:border-ghost"
      />
      {step(10, '+10')}
      {step(30, '+30')}
    </div>
  )
}
