import { useEffect, useRef } from 'react'
import { useQueryClient } from '@tanstack/react-query'

// SSE로 "뭔가 바뀌었다"를 받아 목록을 다시 불러온다.
//
// 카드 상세(['card', id])는 **절대 무효화하지 않는다**. 그 화면은 디바운스
// 자동저장 + 비제어 블록 편집기라, 타이핑 중에 다시 불러오면 문서가 리셋되고
// 커서가 튄다. 목록만 갱신하면 충분하다.
const LIVE_KEYS = ['boards', 'board', 'today', 'calendar', 'routines', 'users', 'babyfood', 'babyfood-stock', 'babyfood-foods']

const RETRY_MIN = 2000
const RETRY_MAX = 30000

export function useLiveSync() {
  const qc = useQueryClient()
  // 콜백이 매번 새로 만들어져도 효과가 재실행되지 않도록 ref에 담는다.
  const qcRef = useRef(qc)
  qcRef.current = qc

  useEffect(() => {
    let es: EventSource | null = null
    let timer: number | undefined
    let delay = RETRY_MIN
    let lastVersion = 0
    let stopped = false

    const invalidate = () => {
      for (const k of LIVE_KEYS) qcRef.current.invalidateQueries({ queryKey: [k] })
    }

    const connect = () => {
      if (stopped) return
      es = new EventSource('/api/stream')

      es.addEventListener('open', () => {
        delay = RETRY_MIN
        // 끊겨 있는 동안 놓친 변경이 있다 — 한 번 전부 다시 불러온다.
        invalidate()
      })

      es.addEventListener('changed', (e) => {
        const v = Number((e as MessageEvent).data)
        // 재연결 시 같은 버전이 다시 올 수 있다.
        if (!Number.isFinite(v) || v <= lastVersion) return
        lastVersion = v
        invalidate()
      })

      es.addEventListener('error', () => {
        // EventSource는 네트워크 끊김은 스스로 재연결하지만 서버가 정상
        // 종료하면 그대로 닫힌다(배포할 때마다 그렇다). 그건 직접 다시 연다.
        if (es?.readyState !== EventSource.CLOSED) return
        es.close()
        es = null
        if (stopped) return
        timer = window.setTimeout(connect, delay)
        delay = Math.min(delay * 2, RETRY_MAX)
      })
    }

    connect()
    return () => {
      stopped = true
      window.clearTimeout(timer)
      es?.close()
    }
  }, [])
}
