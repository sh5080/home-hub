import { useEffect, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { api, type FinDetail, type FinOverview } from '../api'
import { useInvalidating } from '../lib/hooks'
import { fmtWon } from '../lib/date'
import { Button, Field, Input, Sheet, SkeletonList, Textarea } from './ui'

export type FinItemTarget = { keys: string[]; label: string; cat1: string }

/** 항목 상세: 이름·분류 고치기, 월별 합계, 거래 목록. 같은 이름으로 고치면 수입원에서 하나로 묶인다. */
export function FinItemSheet({ target, owner, d, onClose }: {
  target: FinItemTarget | null
  owner: number
  d: FinOverview
  onClose: () => void
}) {
  const keys = target?.keys ?? []
  const q = useQuery({
    queryKey: ['finance', 'detail', owner, keys],
    queryFn: () => api.get<FinDetail>(`/api/finance/detail?owner=${owner}&${keys.map((k) => `key=${encodeURIComponent(k)}`).join('&')}`),
    enabled: keys.length > 0,
  })
  const save = useInvalidating((b: { keys: string[]; label: string; cat1: string; memo: string }) => api.put('/api/finance/labels', b), [['finance']])
  const cur = keys.map((k) => d.labels?.[k]).find(Boolean)
  const [label, setLabel] = useState('')
  const [cat1, setCat1] = useState('')
  const [memo, setMemo] = useState('')
  const [rename, setRename] = useState(false)
  useEffect(() => {
    setLabel(cur?.label ?? ''); setCat1(cur?.cat1 ?? ''); setMemo(cur?.memo ?? '')
    setRename(!!(cur?.label || cur?.cat1))
  }, [target]) // eslint-disable-line react-hooks/exhaustive-deps

  const tx = q.data?.tx ?? []
  const origNames = [...new Set(tx.map((t) => t.content))]
  const origCats = [...new Set(tx.map((t) => t.cat1).filter(Boolean))]
  const cats = [...new Set([...d.income, ...d.fixed].map((x) => x.cat1).concat(d.unexpected.map((u) => u.cat1)).filter(Boolean))].sort()
  const max = Math.max(1, ...(q.data?.months ?? []).map((m) => Math.abs(m.amount)))

  return (
    <Sheet open={target !== null} onClose={onClose} title={target?.label}>
      {target && (
        <div className="space-y-4">
          <form className="space-y-2" onSubmit={async (e) => {
            e.preventDefault()
            await save.mutateAsync({ keys, label, cat1, memo })
            onClose()
          }}>
            <Field label="메모">
              <Textarea value={memo} maxLength={200} rows={2} placeholder="예: 아기 침대 중고 거래, 8월 가전 할부 3회차" onChange={(e) => setMemo(e.target.value)} />
            </Field>
            <button type="button" onClick={() => setRename(!rename)} className="text-xs text-muted underline">{rename ? '이름·분류 고치기 접기' : '이름·분류 고치기'}</button>
            {rename && (
            <>
            <p className="text-[11px] text-faint">이름은 가급적 고치지 마세요. 원래 이름으로 거래를 찾고 같은 항목끼리 묶어요 — 설명은 메모에 적는 게 좋아요. 이름을 고치는 건 같은 수입(회사 이름·'급여'·'10월 급여')을 하나로 합칠 때처럼 꼭 필요할 때만.</p>
            <div className="grid grid-cols-2 gap-2">
              <Field label="이름"><Input value={label} maxLength={40} placeholder={origNames[0] ?? target.label} onChange={(e) => setLabel(e.target.value)} /></Field>
              <Field label="분류">
                <Input value={cat1} maxLength={20} list="fin-cats" placeholder={origCats[0] ?? target.cat1} onChange={(e) => setCat1(e.target.value)} />
                <datalist id="fin-cats">{cats.map((c) => <option key={c} value={c} />)}</datalist>
              </Field>
            </div>
            <p className="text-[11px] text-faint">비우면 파일 값 그대로예요.</p>
            </>
            )}
            <div className="flex gap-2">
              {(cur?.label || cur?.cat1) && <Button type="button" variant="ghost" onClick={async () => { await save.mutateAsync({ keys, label: '', cat1: '', memo }); onClose() }}>이름·분류 원래대로</Button>}
              <div className="flex-1" />
              <Button type="submit" disabled={save.isPending}>저장</Button>
            </div>
          </form>


          {origNames.length > 1 && (
            <p className="text-xs text-muted">묶인 원래 이름: {origNames.join(' · ')}</p>
          )}

          <div>
            <p className="mb-1 text-xs font-semibold text-ink-2">월별</p>
            {q.isPending ? <SkeletonList rows={2} /> : (
              <ul className="space-y-1">
                {(q.data?.months ?? []).map((m) => (
                  <li key={m.month} className="flex items-center gap-2 text-xs">
                    <span className="w-14 shrink-0 text-muted">{m.month.slice(2).replace('-', '.')}</span>
                    <div className="h-1.5 flex-1 rounded-full bg-surface-2">
                      <div className={`h-1.5 rounded-full ${m.amount >= 0 ? 'bg-emerald-500' : 'bg-rose-400'}`} style={{ width: `${(Math.abs(m.amount) / max) * 100}%` }} />
                    </div>
                    <span className="w-24 shrink-0 text-right tabular-nums">{fmtWon(Math.abs(m.amount))}</span>
                    <span className="w-8 shrink-0 text-right text-faint">{m.count}건</span>
                  </li>
                ))}
              </ul>
            )}
          </div>

          <div>
            <p className="mb-1 text-xs font-semibold text-ink-2">거래 {tx.length}건</p>
            <ul className="divide-y divide-line">
              {tx.map((t, i) => (
                <li key={i} className="flex items-baseline gap-2 py-1.5 text-xs">
                  <span className="w-20 shrink-0 text-muted">{t.at.slice(2, 10).replaceAll('-', '.')}</span>
                  <span className="min-w-0 flex-1 truncate">
                    {t.content}
                    <span className="text-faint"> · {t.type}{t.cat1 && ` · ${t.cat1}`}{t.method && ` · ${t.method}`}</span>
                  </span>
                  <span className={`shrink-0 tabular-nums ${t.amount >= 0 ? 'text-emerald-600' : ''}`}>{t.amount >= 0 ? '+' : ''}{fmtWon(t.amount)}</span>
                </li>
              ))}
            </ul>
          </div>
        </div>
      )}
    </Sheet>
  )
}
