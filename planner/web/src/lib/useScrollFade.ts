import { useCallback, useEffect, useRef, useState } from 'react'

/**
 * 스크롤 영역의 위/아래에 "더 있다"를 알린다.
 *
 * 스크롤바를 숨기면 더 볼 게 있는지 알 수가 없다. 막대 대신 가장자리를
 * 흐리게 해서 잘린 내용이 있다는 걸 보여준다.
 *
 * overlay 를 덧대지 않고 mask-image 로 내용 자체를 페이드한다 — 컬럼 배경이
 * 드래그 중에 바뀌어도 색을 맞출 필요가 없다.
 */
export function useScrollFade<T extends HTMLElement>() {
  const ref = useRef<T>(null)
  const [fade, setFade] = useState({ top: false, bottom: false })

  const measure = useCallback(() => {
    const el = ref.current
    if (!el) return
    const { scrollTop, scrollHeight, clientHeight } = el
    // 1px 여유 — 소수 픽셀 때문에 끝에서도 계속 흐려 보이는 걸 막는다.
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
    // 카드가 추가·삭제되거나 화면이 회전해도 다시 잰다.
    const ro = new ResizeObserver(measure)
    ro.observe(el)
    for (const child of Array.from(el.children)) ro.observe(child)
    return () => {
      el.removeEventListener('scroll', measure)
      ro.disconnect()
    }
  }, [measure])

  // 잘린 쪽만 흐리게. 양쪽 다면 위아래 모두.
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
