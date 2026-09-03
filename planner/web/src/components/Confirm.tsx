import { createContext, useCallback, useContext, useRef, useState, type ReactNode } from 'react'
import { Button } from './ui'

/** 확인 팝업. confirm() 은 동기라 UI 를 멈추고 앱 스타일과 따로 논다 — Promise 로 같은 모양을 준다. */
type Ask = {
  title: string
  body?: ReactNode
  confirmLabel?: string
  cancelLabel?: string
  /** 되돌릴 수 없는 일이면 확인 버튼을 빨갛게 */
  danger?: boolean
}

const Ctx = createContext<(a: Ask) => Promise<boolean>>(async () => false)

export function useConfirm() {
  return useContext(Ctx)
}

export function ConfirmProvider({ children }: { children: ReactNode }) {
  const [ask, setAsk] = useState<Ask | null>(null)
  const resolver = useRef<(v: boolean) => void>(null)

  const confirm = useCallback((a: Ask) => {
    setAsk(a)
    return new Promise<boolean>((resolve) => {
      resolver.current = resolve
    })
  }, [])

  const close = (v: boolean) => {
    setAsk(null)
    resolver.current?.(v)
    resolver.current = null
  }

  return (
    <Ctx.Provider value={confirm}>
      {children}
      {ask && (
        <div className="fixed inset-0 z-50 flex items-center justify-center p-6">
          {/* 바깥 누름은 언제나 취소. */}
          <button aria-label="닫기" onClick={() => close(false)} className="absolute inset-0 bg-black/40" />
          <div
            role="alertdialog"
            aria-modal="true"
            className="relative w-full max-w-xs rounded-2xl bg-surface p-5 shadow-xl"
          >
            <p className="text-base font-bold">{ask.title}</p>
            {ask.body && <div className="mt-1.5 text-sm text-muted">{ask.body}</div>}
            <div className="mt-5 flex gap-2">
              <Button variant="ghost" className="flex-1" onClick={() => close(false)}>
                {ask.cancelLabel ?? '취소'}
              </Button>
              <Button
                autoFocus
                variant={ask.danger ? 'danger' : 'primary'}
                className="flex-1"
                onClick={() => close(true)}
              >
                {ask.confirmLabel ?? '확인'}
              </Button>
            </div>
          </div>
        </div>
      )}
    </Ctx.Provider>
  )
}
