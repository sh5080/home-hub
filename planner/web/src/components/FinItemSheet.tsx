import { useEffect, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { api, type FinDetail, type FinOverview } from '../api'
import { useInvalidating } from '../lib/hooks'
import { fmtWon } from '../lib/date'
import { Button, Field, Input, Sheet, SkeletonList } from './ui'
import { TagChips } from './FinTags'

export type FinItemTarget = { keys: string[]; label: string; cat1: string }

/** 항목 상세: 이름·분류 고치기, 월별 합계, 거래 목록. 같은 이름으로 고치면 수입원에서 하나로 묶인다. */
export function FinItemSheet({ target, owner, d, onClose, onTag }: {
  target: FinItemTarget | null
  owner: number
  d: FinOverview
  onClose: () => void
  onTag: (t: { keys: string[]; label: string }) => void
}) {
  const keys = target?.keys ?? []
  const q = useQuery({
    queryKey: ['finance', 'detail', owner, keys],
    queryFn: () => api.get<FinDetail>(`/api/finance/detail?owner=${owner}&${keys.map((k) => `key=${encodeURIComponent(k)}`).join('&')}`),
    enabled: keys.length > 0,
  })
  const save = useInvalidating((b: { keys: string[]; label: string; cat1: string }) => api.put('/api/finance/labels', b), [['finance']])
  const cur = keys.map((k) => d.labels?.[k]).find(Boolean)
  const [label, setLabel] = useState('')
  const [cat1, setCat1] = useState('')
  useEffect(() => { setLabel(cur?.label ?? ''); setCat1(cur?.cat1 ?? '') }, [target]) // eslint-disable-line react-hooks/exhaustive-deps

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
            await save.mutateAsync({ keys, label, cat1 })
            onClose()
          }}>
            <div className="grid grid-cols-2 gap-2">
              <Field label="이름"><Input value={label} maxLength={40} placeholder={origNames[0] ?? target.label} onChange={(e) => setLabel(e.target.value)} /></Field>
              <Field label="분류">
                <Input value={cat1} maxLength={20} list="fin-cats" placeholder={origCats[0] ?? target.cat1} onChange={(e) => setCat1(e.target.value)} />
                <datalist id="fin-cats">{cats.map((c) => <option key={c} value={c} />)}</datalist>
              </Field>
            </div>
            <p className="text-[11px] text-faint">비우면 파일 값 그대로예요. 수입원은 같은 이름으로 고친 것끼리 하나로 합쳐 세요(예: 회사 이름·'급여'·'10월 급여' → '급여').</p>
            <div className="flex gap-2">
              {cur && <Button type="button" variant="ghost" onClick={async () => { await save.mutateAsync({ keys, label: '', cat1: '' }); onClose() }}>원래대로</Button>}
              <div className="flex-1" />
              <Button type="submit" disabled={save.isPending}>저장</Button>
            </div>
          </form>

          <div>
            <p className="mb-1 text-xs font-semibold text-ink-2">태그</p>
            <TagChips ids={d.tag_links[keys[0]]} tags={d.tags} onEdit={() => onTag({ keys, label: target.label })} />
          </div>

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
