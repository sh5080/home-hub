import { useCallback, useEffect, useRef, useState } from 'react'

/** 스크롤 영역의 잘린 쪽 가장자리를 mask-image 로 흐리게(스크롤바 대신). */
export function useScrollFade<T extends HTMLElement>() {
  const ref = useRef<T>(null)
  const [fade, setFade] = useState({ top: false, bottom: false })

  const measure = useCallback(() => {
    const el = ref.current
    if (!el) return
    const { scrollTop, scrollHeight, clientHeight } = el
    // 1px 여유(소수 픽셀).
    setFade({
      top: scrollTop > 1,
      bottom: scrollTop + clientHeight < scrollHeight - 1,
    })
  }, [])

  useEffect(() => {
    const el = ref.current
    if (!el) return
    measure()
    el.addEventListener('scroll', measure, { passive: true })
    const ro = new ResizeObserver(measure)
    ro.observe(el)
    for (const child of Array.from(el.children)) ro.observe(child)
    return () => {
      el.removeEventListener('scroll', measure)
      ro.disconnect()
    }
  }, [measure])

  const FADE = '14px'
  const mask = fade.top && fade.bottom
    ? `linear-gradient(to bottom, transparent, #000 ${FADE}, #000 calc(100% - ${FADE}), transparent)`
    : fade.top
      ? `linear-gradient(to bottom, transparent, #000 ${FADE})`
      : fade.bottom
        ? `linear-gradient(to bottom, #000 calc(100% - ${FADE}), transparent)`
        : undefined

  return {
    ref,
    remeasure: measure,
    style: mask ? ({ maskImage: mask, WebkitMaskImage: mask } as const) : undefined,
  }
}
