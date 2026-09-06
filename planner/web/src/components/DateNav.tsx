import { useRef, type ReactNode } from 'react'

/**
 * ‹ 날짜 › — 기록·식단·루틴 공용. 글자를 누르면 기기 달력, 오늘이 아닐 때만 '오늘' 칩.
 * iOS 는 투명 date 입력의 '값 자리'만 탭을 받아서, 글자 전체를 버튼으로 두고 showPicker() 로 연다
 * (없으면 focus+click).
 */
export default function DateNav({ value, onPick, onPrev, onNext, label, isToday, onToday, nextDisabled }: {
  /** 달력이 처음 가리킬 날짜 (YYYY-MM-DD) */
  value: string
  onPick: (date: string) => void
  onPrev: () => void
  onNext: () => void
  label: ReactNode
  isToday: boolean
  onToday: () => void
  nextDisabled?: boolean
}) {
  const input = useRef<HTMLInputElement>(null)
  return (
    <div className="flex items-center justify-between">
      <button onClick={onPrev} aria-label="이전" className="rounded-lg px-2 py-1 text-muted active:bg-line">‹</button>
      <div className="flex items-center gap-1.5">
        <button
          type="button"
          onClick={() => {
            const el = input.current
            if (!el) return
            try {
              if (typeof el.showPicker === 'function') { el.showPicker(); return }
            } catch { /* 막히면 아래로 */ }
            el.focus()
            el.click()
          }}
          className="relative flex items-center gap-1 rounded-lg px-1.5 py-1 text-sm font-semibold active:bg-line"
        >
          {label}
          <svg viewBox="0 0 24 24" className="h-3.5 w-3.5 text-faint" fill="none" stroke="currentColor" strokeWidth="2.4" strokeLinecap="round" strokeLinejoin="round"><path d="M6 9l6 6 6-6" /></svg>
          {/* 달력이 글자 아래에 붙어 뜨도록 버튼 안에 두되 탭은 버튼이 받는다. */}
          <input
            ref={input}
            type="date"
            tabIndex={-1}
            value={value}
            onChange={(e) => { if (e.target.value) onPick(e.target.value) }}
            aria-hidden
            className="pointer-events-none absolute inset-0 h-full w-full opacity-0"
          />
        </button>
        {!isToday && (
          <button onClick={onToday} className="rounded-full bg-surface-2 px-2 py-0.5 text-[11px] font-medium text-muted active:bg-line">
            오늘
          </button>
        )}
      </div>
      <button onClick={onNext} disabled={nextDisabled} aria-label="다음" className="rounded-lg px-2 py-1 text-muted active:bg-line disabled:opacity-30">›</button>
    </div>
  )
}
