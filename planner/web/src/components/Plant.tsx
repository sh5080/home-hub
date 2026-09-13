import { useEffect, useState } from 'react'
import { BeanBuddy } from './BeanBuddy'

// 함께 키우는 강낭콩. 정원(하늘·흐린 집·울타리·덤불·잔디) 아래 땅 단면에서 시작해 콩깍지가 열린다.
// 단계마다 카메라가 바뀐다: 씨앗은 땅속 가득, 새싹은 땅 1/3, 그 뒤로는 정원.
// 입체감: 층마다 그라데이션·하이라이트·그림자, 먼 배경은 흐리게(심도).
// 함께 키우는 식물 = 콩이(머리 위 새싹이 단계마다 자란다).
export function Plant(props: { stage: number; size?: number; happy?: boolean }) {
  return <BeanBuddy {...props} />
}

const POKE_LINES = ['간지러워요 >_<', '고마워요! 💚', '물 더 주세요 💧', '쑥쑥 자랄게요!', '오늘도 같이 해요', '헤헤 😊', '좋아요~']

/** 누를 수 있는 식물: 통통 튀고, 하트·반짝이·물방울이 퍼지고, 한마디 한다. */
export function PokablePlant({ stage, size = 160, happy = false, hint }: { stage: number; size?: number; happy?: boolean; hint?: string }) {
  const [poke, setPoke] = useState(0)
  const [line, setLine] = useState('')
  const [bits, setBits] = useState<{ id: number; ch: string; dx: number; dy: number; r: number }[]>([])
  const tap = () => {
    const n = poke + 1
    setPoke(n)
    // 다섯 번째마다 다음 단계 귀띔
    setLine(n % 5 === 0 && hint ? hint : POKE_LINES[Math.floor(Math.random() * POKE_LINES.length)])
    const chars = ['💚', '✨', '💧', '🌱', '💛']
    setBits(Array.from({ length: 9 }, (_, i) => ({
      id: n * 100 + i,
      ch: chars[(i + n) % chars.length],
      dx: Math.cos((i / 9) * Math.PI * 2 - Math.PI / 2) * (size * 0.38 + Math.random() * 18),
      dy: Math.sin((i / 9) * Math.PI * 2 - Math.PI / 2) * (size * 0.3 + Math.random() * 14) - 20,
      r: Math.random() * 60 - 30,
    })))
    try { navigator.vibrate?.(15) } catch { /* 진동 없는 기기 */ }
  }
  useEffect(() => {
    if (!poke) return
    const t = window.setTimeout(() => { setLine(''); setBits([]) }, 1600)
    return () => window.clearTimeout(t)
  }, [poke])
  return (
    <button type="button" onClick={tap} aria-label="식물 쓰다듬기" className="relative select-none rounded-3xl outline-none [-webkit-tap-highlight-color:transparent] focus-visible:ring-2 focus-visible:ring-emerald-400">
      <div key={poke} className={poke ? 'plant-poke' : ''} style={{ transformOrigin: '50% 90%' }}>
        <Plant stage={stage} size={size} happy={happy || line !== ''} />
      </div>
      {bits.map((b) => (
        <span key={b.id} className="plant-bit pointer-events-none absolute left-1/2 top-1/2 text-lg"
          style={{ '--dx': `${b.dx}px`, '--dy': `${b.dy}px`, '--r': `${b.r}deg` } as React.CSSProperties}>{b.ch}</span>
      ))}
      {line && (
        <span key={`l${poke}`} className="plant-say pointer-events-none absolute left-1/2 top-0 z-10 whitespace-nowrap rounded-2xl bg-white px-3 py-1.5 text-xs font-semibold text-emerald-800 shadow-[0_6px_16px_-6px_rgba(16,94,58,0.5)] ring-1 ring-emerald-100 dark:bg-slate-800 dark:text-emerald-300 dark:ring-slate-700">
          {line}
          <span className="absolute -bottom-1 left-1/2 h-2 w-2 -translate-x-1/2 rotate-45 bg-white ring-0 dark:bg-slate-800" />
        </span>
      )}
    </button>
  )
}

/** 단계 배지: 원형 단계 숫자 + 이름 + 다섯 칸 진행. */
export function StageBadge({ stage, name, total = 5, small = false }: { stage: number; name: string; total?: number; small?: boolean }) {
  return (
    <span className="inline-flex items-center gap-2">
      <span className={`flex shrink-0 items-center justify-center rounded-full bg-gradient-to-b from-emerald-400 to-emerald-600 font-bold text-white shadow-[0_3px_8px_-2px_rgba(5,150,105,0.6),inset_0_1px_0_rgba(255,255,255,0.45)] ${small ? 'h-6 w-6 text-[11px]' : 'h-8 w-8 text-sm'}`}>
        {stage}
      </span>
      <span className="flex flex-col gap-1">
        <span className={`font-bold leading-none ${small ? 'text-sm' : 'text-base'}`}>{name}</span>
        <span className="flex gap-0.5" aria-label={`${total}단계 중 ${stage}단계`}>
          {Array.from({ length: total }, (_, i) => (
            <span key={i} className={`h-1.5 rounded-full ${small ? 'w-3' : 'w-4'} ${i < stage ? 'bg-gradient-to-r from-emerald-400 to-emerald-500 shadow-[0_1px_2px_rgba(5,150,105,0.4)]' : 'bg-line'}`} />
          ))}
        </span>
      </span>
    </span>
  )
}
