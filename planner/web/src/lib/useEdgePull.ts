import { useEffect, useRef, useState } from 'react'

/**
 * 스크롤 칸(anchor)의 끝에서 한 번 더 끌면 옆 날짜로 넘어간다.
 * 끝에 닿은 상태로 시작한 끌기만 센다(관성으로 닿은 건 무시).
 */
export function useEdgePull(anchor: React.RefObject<HTMLElement | null>, opts: {
  onTop?: () => void
  onBottom?: () => void
  threshold?: number
}) {
  const [pull, setPull] = useState<{ edge: 'top' | 'bottom'; px: number } | null>(null)
  const optsRef = useRef(opts)
  optsRef.current = opts

  useEffect(() => {
    const main = anchor.current
    if (!main) return
    const need = opts.threshold ?? 80
    let startY = 0
    let edge: 'top' | 'bottom' | null = null
    // 내용이 짧으면 위·아래 끝에 동시에 닿아 있다 — 처음 움직이는 방향으로 정한다.
    let canTop = false
    let canBottom = false

    const atTop = () => main.scrollTop <= 0
    const atBottom = () => main.scrollTop + main.clientHeight >= main.scrollHeight - 2

    const onStart = (e: TouchEvent) => {
      startY = e.touches[0].clientY
      canTop = atTop() && !!optsRef.current.onTop
      canBottom = atBottom() && !!optsRef.current.onBottom
      edge = null
    }
    const onMove = (e: TouchEvent) => {
      const dy0 = e.touches[0].clientY - startY
      if (!edge) {
        if (dy0 > 4 && canTop) edge = 'top'
        else if (dy0 < -4 && canBottom) edge = 'bottom'
        else return
      }
      const dy = dy0
      const px = edge === 'top' ? dy : -dy
      if (px <= 0) { setPull(null); return }
      setPull({ edge, px })
    }
    const onEnd = (e: TouchEvent) => {
      if (!edge) return
      const dy = e.changedTouches[0].clientY - startY
      const px = edge === 'top' ? dy : -dy
      const fire = px >= need ? edge : null
      edge = null
      setPull(null)
      if (fire === 'top') optsRef.current.onTop?.()
      if (fire === 'bottom') optsRef.current.onBottom?.()
    }
    main.addEventListener('touchstart', onStart, { passive: true })
    main.addEventListener('touchmove', onMove, { passive: true })
    main.addEventListener('touchend', onEnd, { passive: true })
    return () => {
      main.removeEventListener('touchstart', onStart)
      main.removeEventListener('touchmove', onMove)
      main.removeEventListener('touchend', onEnd)
    }
  }, [anchor, opts.threshold])

  const ready = pull ? pull.px >= (opts.threshold ?? 80) : false
  return { pull, ready }
}
