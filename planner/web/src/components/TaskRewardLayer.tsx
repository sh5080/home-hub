import { useEffect, useState } from 'react'
import type { TaskRewardEvent } from '../api'

// 할 일을 끝냈을 때 잠깐 뜨는 판정. 처음 마감 안이면 물방울이 튀고, 넘겼으면 분발하자고 한다.
export default function TaskRewardLayer() {
  const [r, setR] = useState<(TaskRewardEvent & { n: number }) | null>(null)
  useEffect(() => {
    let n = 0
    let t: number | undefined
    const on = (e: Event) => {
      const d = (e as CustomEvent<TaskRewardEvent>).detail
      setR({ ...d, n: ++n })
      window.clearTimeout(t)
      t = window.setTimeout(() => setR(null), d.on_time ? 2600 : 2400)
    }
    window.addEventListener('task-reward', on)
    return () => { window.removeEventListener('task-reward', on); window.clearTimeout(t) }
  }, [])
  if (!r) return null
  const due = `${Number(r.first_due.slice(5, 7))}/${Number(r.first_due.slice(8, 10))}`
  return (
    <div key={r.n} className="pointer-events-none fixed inset-x-0 top-[calc(env(safe-area-inset-top,0px)+72px)] z-50 flex justify-center px-4" role="status" aria-live="polite">
      {r.on_time && (
        <div className="absolute inset-x-0 top-0 flex justify-center">
          {Array.from({ length: 10 }, (_, i) => (
            <span key={i} className="reward-drop absolute text-xl" style={{ '--dx': `${(i - 4.5) * 26}px`, '--dy': `${-40 - (i % 3) * 22}px`, animationDelay: `${i * 30}ms` } as React.CSSProperties}>💧</span>
          ))}
        </div>
      )}
      <div className={`reward-pop rounded-2xl px-4 py-3 text-center shadow-[0_12px_30px_-10px_rgba(15,23,42,0.45)] ring-1 ${r.on_time
        ? 'bg-gradient-to-b from-sky-50 to-white ring-sky-200 dark:from-sky-950 dark:to-slate-900 dark:ring-sky-800'
        : 'bg-gradient-to-b from-amber-50 to-white ring-amber-200 dark:from-amber-950 dark:to-slate-900 dark:ring-amber-800'}`}>
        {r.on_time ? (
          <>
            <p className="text-base font-bold">{r.drops > 0 ? `💧 +${r.drops} 받았어요!` : '마감 지켰어요! 👏'}</p>
            <p className="mt-0.5 max-w-[16rem] truncate text-xs text-muted">'{r.title}' — 처음 정한 {due}까지 끝냈어요{r.drops === 0 && ' (물방울은 이미 받았어요)'}</p>
          </>
        ) : (
          <>
            <p className="text-base font-bold">조금만 더 분발해요! 💪</p>
            <p className="mt-0.5 max-w-[16rem] truncate text-xs text-muted">'{r.title}' — 처음 정한 마감 {due}을 넘겼어요. 다음엔 제때!</p>
          </>
        )}
      </div>
    </div>
  )
}
