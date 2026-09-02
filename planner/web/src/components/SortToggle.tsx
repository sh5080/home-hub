import type { SortMode } from '../lib/hooks'

// 시간 / 중요도.
const MODES: { key: SortMode; label: string }[] = [
  { key: 'time', label: '시간' },
  { key: 'priority', label: '중요도' },
]

export type SortOrder = 'asc' | 'desc'

// 방향은 칸 헤더 화살표가 칸마다 맡는다.
export default function SortToggle({
  value,
  onChange,
  modes = MODES,
}: {
  value: SortMode
  onChange: (m: SortMode) => void
  modes?: { key: SortMode; label: string }[]
}) {
  return (
    <div className="flex rounded-lg bg-line p-0.5 text-xs font-medium">
      {modes.map((m) => (
        <button
          key={m.key}
          onClick={() => onChange(m.key)}
          className={`rounded-[6px] px-2.5 py-1 transition ${value === m.key ? 'bg-surface text-ink shadow-sm' : 'text-muted'}`}
        >
          {m.label}
        </button>
      ))}
    </div>
  )
}

export function Stars({ n, className = '' }: { n: number; className?: string }) {
  if (!n) return null
  return (
    <span className={`shrink-0 text-[10px] leading-none text-amber-500 ${className}`} title={`중요도 ${n}`}>
      {'★'.repeat(n)}
    </span>
  )
}

/** 별을 눌러 0~3. 같은 별을 다시 누르면 해제. */
export function StarPicker({ value, onChange }: { value: number; onChange: (n: number) => void }) {
  return (
    <div className="flex items-center gap-0.5">
      {[1, 2, 3].map((n) => (
        <button
          key={n}
          type="button"
          onClick={() => onChange(value === n ? 0 : n)}
          aria-label={`중요도 ${n}`}
          className={`px-0.5 text-lg leading-none transition ${n <= value ? 'text-amber-500' : 'text-ghost'}`}
        >
          ★
        </button>
      ))}
      {value > 0 && (
        <button type="button" onClick={() => onChange(0)} className="ml-1 text-xs text-faint">
          지우기
        </button>
      )}
    </div>
  )
}
