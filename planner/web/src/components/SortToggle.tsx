import type { SortMode } from '../lib/hooks'

// 수동 / 시간 / 중요도.
//
// 수동일 때만 드래그로 바꾼 순서가 화면 순서가 된다. 다른 기준을 켜면 서버가
// 매번 정렬하므로 드래그 재정렬이 남지 않는다 — 그래서 UI에서도 막는다.
const MODES: { key: SortMode; label: string }[] = [
  { key: 'time', label: '시간' },
  { key: 'priority', label: '중요도' },
  { key: 'manual', label: '수동' },
]

export type SortOrder = 'asc' | 'desc'

// 방향(오름/내림)은 여기 없다. 칸반에서는 컬럼마다 달라야 해서 각 컬럼
// 헤더의 화살표가 맡는다 — 기준은 하나, 방향은 컬럼별.
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
    <div className="flex rounded-lg bg-slate-200 p-0.5 text-xs font-medium">
      {modes.map((m) => (
        <button
          key={m.key}
          onClick={() => onChange(m.key)}
          className={`rounded-[6px] px-2.5 py-1 transition ${value === m.key ? 'bg-white text-slate-900 shadow-sm' : 'text-slate-500'}`}
        >
          {m.label}
        </button>
      ))}
    </div>
  )
}

/** 중요도 별. 0이면 아무것도 안 그린다. */
export function Stars({ n, className = '' }: { n: number; className?: string }) {
  if (!n) return null
  return (
    <span className={`shrink-0 text-[10px] leading-none text-amber-500 ${className}`} title={`중요도 ${n}`}>
      {'★'.repeat(n)}
    </span>
  )
}

/** 중요도 고르기 — 별을 눌러 0~3. 같은 별을 다시 누르면 해제. */
export function StarPicker({ value, onChange }: { value: number; onChange: (n: number) => void }) {
  return (
    <div className="flex items-center gap-0.5">
      {[1, 2, 3].map((n) => (
        <button
          key={n}
          type="button"
          onClick={() => onChange(value === n ? 0 : n)}
          aria-label={`중요도 ${n}`}
          className={`px-0.5 text-lg leading-none transition ${n <= value ? 'text-amber-500' : 'text-slate-300'}`}
        >
          ★
        </button>
      ))}
      {value > 0 && (
        <button type="button" onClick={() => onChange(0)} className="ml-1 text-xs text-slate-400">
          지우기
        </button>
      )}
    </div>
  )
}
