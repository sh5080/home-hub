import { useEffect, useRef, useState } from 'react'
import { api, type BFChild } from '../api'
import { useBFChildren, useInvalidating, useUsers } from '../lib/hooks'
import { lifeDay } from '../lib/date'
import { Button, Field, Input, TimeField } from './ui'
import { useConfirm } from './Confirm'

/** 이유식 대상과 계산 기간. 생년월일은 가족 설정에만 있다. */
export default function BabyfoodSettings() {
  const users = useUsers()
  const kids = useBFChildren()
  const confirm = useConfirm()
  const [err, setErr] = useState<string | null>(null)

  const keys = [['babyfood'], ['babyfood-stock'], ['babyfood-foods']]
  const add = useInvalidating(
    (b: { user_id: number; horizon_days: number }) => api.post('/api/babyfood/children', b), keys)
  const setTimes = useInvalidating(
    ({ child, ...b }: { child: number; n: number; times: string[] }) =>
      api.post(`/api/babyfood/mealtimes?child=${child}`, b), keys)
  const remove = useInvalidating((id: number) => api.del(`/api/babyfood/children/${id}`), keys)

  const current = kids.data ?? []
  const isChild = (id: number) => current.some((c) => c.user_id === id)
  const eligible = (users.data ?? []).filter((u) => u.birth_date && !isChild(u.id))

  return (
    <div className="space-y-3">
      {current.length === 0 && (
        <p className="text-xs text-faint">
          이유식을 진행할 가족을 고르세요. 가족 설정에서 생년월일을 먼저 넣어야 보여요.
        </p>
      )}

      <ul className="space-y-2">
        {current.map((c) => (
          <li key={c.user_id} className="rounded-xl bg-canvas p-3">
            <div className="flex items-center gap-2">
              <span className="flex-1 text-sm font-semibold">{c.name}</span>
              <span className="text-[11px] text-faint">{lifeDay(c.today_dday)} · 식단 {c.days}일</span>
              <button
                onClick={async () => {
                  if (await confirm({
                    title: `${c.name}을(를) 이유식에서 뺄까요?`,
                    body: '식단과 재고 기록은 지우지 않아요. 다시 넣으면 그대로 돌아와요.',
                    confirmLabel: '빼기',
                  })) remove.mutate(c.user_id)
                }}
                className="rounded-lg px-2 py-1 text-xs text-muted active:bg-line"
              >
                빼기
              </button>
            </div>
            <MealTimes
              child={c}
              onSave={(n, times) => setTimes.mutateAsync({ child: c.user_id, n, times })}
            />

            <div className="mt-2 flex items-end gap-2">
              <div className="flex-1">
                <Field label="재고를 며칠치로 계산할까요">
                  <Input
                    inputMode="numeric"
                    defaultValue={String(c.horizon_days)}
                    onBlur={(e) => {
                      const n = Number(e.target.value)
                      if (n > 0 && n !== c.horizon_days) add.mutate({ user_id: c.user_id, horizon_days: n })
                    }}
                  />
                </Field>
              </div>
              <p className="pb-2.5 text-[11px] text-faint">
                {c.horizon_days % 7 === 0 ? `${c.horizon_days / 7}주치` : ''}
              </p>
            </div>
          </li>
        ))}
      </ul>

      {eligible.length > 0 && (
        <div className="flex flex-wrap gap-1.5">
          {eligible.map((u) => (
            <Button
              key={u.id}
              variant="ghost"
              onClick={async () => {
                setErr(null)
                try { await add.mutateAsync({ user_id: u.id, horizon_days: 21 }) }
                catch (e) { setErr(e instanceof Error ? e.message : '추가하지 못했어요') }
              }}
            >
              + {u.name}
            </Button>
          ))}
        </div>
      )}
      {err && <p className="text-xs text-rose-500">{err}</p>}
    </div>
  )
}

/** 하루 n끼일 때의 기본 시각(끼니 수마다 따로). */
const SLOT_NAMES: Record<number, string[]> = {
  1: ['점심'],
  2: ['점심', '저녁'],
  3: ['아침', '점심', '저녁'],
}

// 서버가 등록 시 같은 값을 깐다. 이건 값이 오기 전 빈 칸 방지용.
const DEFAULT_TIMES: Record<number, string[]> = {
  1: ['12:00'],
  2: ['12:00', '18:00'],
  3: ['09:00', '12:00', '18:00'],
}

function MealTimes({ child, onSave }: {
  child: BFChild
  onSave: (n: number, times: string[]) => Promise<unknown>
}) {
  const [err, setErr] = useState<string | null>(null)
  // 연달아 누르는 동안 화면만 움직이고 멈추면 한 번 저장한다(요청이 앞지르지 않게).
  const [draft, setDraft] = useState<Record<string, string[]>>(() => child.meal_times ?? {})
  const dirty = useRef(false)
  const timer = useRef<number | undefined>(undefined)

  useEffect(() => {
    // 고치는 중에는 서버 값으로 덮지 않는다.
    if (!dirty.current && child.meal_times) setDraft(child.meal_times)
  }, [child.meal_times])

  useEffect(() => () => window.clearTimeout(timer.current), [])

  const set = (n: number, i: number, v: string) => {
    const times = SLOT_NAMES[n].map((_, k) =>
      k === i ? v : draft[String(n)]?.[k] || DEFAULT_TIMES[n][k])
    dirty.current = true
    setDraft({ ...draft, [String(n)]: times })
    // 한 칸이라도 비면 서버가 받지 않는다.
    if (times.some((x) => !x)) return
    window.clearTimeout(timer.current)
    timer.current = window.setTimeout(async () => {
      setErr(null)
      try { await onSave(n, times) }
      catch (ex) { setErr(ex instanceof Error ? ex.message : '저장하지 못했어요') }
      finally { dirty.current = false }
    }, 500)
  }

  return (
    <div className="mt-2">
      <p className="text-[11px] font-medium text-muted">기본 시각</p>
      <p className="text-[11px] text-faint">끼니 수마다 따로예요. 2끼의 점심을 바꿔도 3끼의 점심은 그대로예요.</p>
      <div className="mt-1.5 space-y-1.5">
        {[1, 2, 3].map((n) => {
          const times = draft[String(n)] ?? []
          return (
            <div key={n} className="rounded-lg bg-surface p-2">
              <p className="text-[11px] font-medium text-muted">{n}끼</p>
              <div className="mt-1 space-y-1.5">
                {SLOT_NAMES[n].map((name, i) => (
                  <div key={name} className="flex items-center gap-2">
                    <span className="w-8 shrink-0 text-[11px] text-faint">{name}</span>
                    <div className="min-w-0 flex-1">
                      <TimeField
                        value={times[i] ?? ''}
                        fallback={DEFAULT_TIMES[n][i]}
                        onChange={(v) => set(n, i, v)}
                      />
                    </div>
                  </div>
                ))}
              </div>
            </div>
          )
        })}
      </div>
      {err && <p className="mt-1 text-xs text-rose-500">{err}</p>}
    </div>
  )
}
