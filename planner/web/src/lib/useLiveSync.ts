import { useEffect, useRef } from 'react'
import { useQueryClient } from '@tanstack/react-query'

// SSE 로 '바뀌었다'를 받아 목록을 다시 불러온다.
// 카드 상세(['card', id])는 무효화하지 않는다 — 편집 중 다시 불러오면 문서가 리셋되고 커서가 튄다.
const LIVE_KEYS = ['boards', 'board', 'today', 'calendar', 'routines', 'users', 'babyfood', 'babyfood-stock', 'babyfood-foods', 'family']

const RETRY_MIN = 2000
const RETRY_MAX = 30000

export function useLiveSync() {
  const qc = useQueryClient()
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
        // 끊긴 동안 놓친 변경이 있어 한 번 전부 다시 불러온다.
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
        // EventSource 는 서버가 정상 종료하면(배포) 재연결하지 않는다 — 직접 다시 연다.
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
