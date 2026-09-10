import { useState } from 'react'
import { api, type FinGoal } from '../api'
import { useFinance, useInvalidating } from '../lib/hooks'
import { fmtWon, today } from '../lib/date'
import { Button, Field, Input, PageHeader, Sheet, SkeletonList } from '../components/ui'
import { useConfirm } from '../components/Confirm'

// 목표. 현재 금액은 연결한 자산(최신 스냅샷)의 합이다. 월 적립은 '쓸 수 있는 돈'에서 빠진다.
export default function FinanceGoals() {
  const q = useFinance(0)
  const confirm = useConfirm()
  const [editing, setEditing] = useState<FinGoal | null>(null)
  const save = useInvalidating((g: FinGoal) => api.post<FinGoal>('/api/finance/goals', g), [['finance']])
  const del = useInvalidating((id: number) => api.del(`/api/finance/goals/${id}`), [['finance']])
  const goals = q.data?.goals ?? []

  const monthsLeft = (due: string | null) => {
    if (!due) return null
    const [y, m] = due.split('-').map(Number)
    const [ty, tm] = today().split('-').map(Number)
    return Math.max(0, (y - ty) * 12 + (m - tm))
  }

  return (
    <div className="mx-auto max-w-lg pb-6">
      <PageHeader title="목표" right={<Button variant="ghost" onClick={() => setEditing({ id: 0, name: '', target: 0, due: null, monthly: 0, items: [], current: 0 })}>+ 추가</Button>} />
      {q.isPending && <div className="px-4 pt-4"><SkeletonList rows={3} /></div>}
      {q.data && goals.length === 0 && (
        <p className="px-4 pt-10 text-center text-sm text-muted">비상금, 목돈 모으기처럼 모으고 싶은 금액을 정해두면 얼마나 왔는지 보여줘요.</p>
      )}
      <ul className="space-y-2 px-4 pt-3">
        {goals.map((g) => {
          const pct = Math.min(100, (g.current / g.target) * 100)
          const left = monthsLeft(g.due)
          const need = left && left > 0 ? Math.ceil(Math.max(0, g.target - g.current) / left) : null
          return (
            <li key={g.id}>
              <button onClick={() => setEditing(g)} className="w-full rounded-2xl bg-surface p-4 text-left shadow-sm active:bg-canvas">
                <div className="flex items-baseline justify-between">
                  <span className="font-semibold">{g.name}</span>
                  <span className="text-xs text-muted">{pct.toFixed(0)}%</span>
                </div>
                <div className="mt-2 h-2 overflow-hidden rounded-full bg-line">
                  <div className="h-full rounded-full bg-emerald-500" style={{ width: `${pct}%` }} />
                </div>
                <p className="mt-1.5 text-xs text-muted tabular-nums">{fmtWon(g.current)} / {fmtWon(g.target)}</p>
                <p className="text-[11px] text-faint">
                  {g.monthly > 0 && `매달 ${fmtWon(g.monthly)} 적립 계획`}
                  {g.due && ` · ${g.due}까지`}
                  {need !== null && ` · 남은 ${left}개월 동안 매달 ${fmtWon(need)} 필요`}
                  {g.items.length === 0 && ' · 연결한 자산이 없어요'}
                </p>
              </button>
            </li>
          )
        })}
      </ul>

      <Sheet open={!!editing} onClose={() => setEditing(null)} title={editing?.id ? '목표 고치기' : '새 목표'}>
        {editing && (
          <GoalForm
            goal={editing}
            names={[...new Set((q.data?.items ?? []).filter((i) => i.group !== 'debt').map((i) => i.name))]}
            onSave={async (g) => { await save.mutateAsync(g); setEditing(null) }}
            onDelete={editing.id ? async () => {
              if (await confirm({ title: '이 목표를 지울까요?', confirmLabel: '지우기', danger: true })) { del.mutate(editing.id); setEditing(null) }
            } : undefined}
          />
        )}
      </Sheet>
    </div>
  )
}

function GoalForm({ goal, names, onSave, onDelete }: {
  goal: FinGoal
  names: string[]
  onSave: (g: FinGoal) => Promise<void>
  onDelete?: () => void
}) {
  const [g, setG] = useState(goal)
  const [err, setErr] = useState<string | null>(null)
  const num = (s: string) => Number(s.replace(/[^0-9]/g, '')) || 0
  return (
    <form className="max-h-[70dvh] space-y-3 overflow-y-auto" onSubmit={async (e) => {
      e.preventDefault(); setErr(null)
      try { await onSave(g) } catch (ex) { setErr(ex instanceof Error ? ex.message : '저장하지 못했어요') }
    }}>
      <Field label="이름"><Input value={g.name} onChange={(e) => setG({ ...g, name: e.target.value })} placeholder="비상금" /></Field>
      <Field label="목표 금액 (원)"><Input inputMode="numeric" value={g.target ? String(g.target) : ''} onChange={(e) => setG({ ...g, target: num(e.target.value) })} /></Field>
      <Field label="언제까지 (선택)">
        <input type="date" value={g.due ?? ''} onChange={(e) => setG({ ...g, due: e.target.value || null })} className="w-full rounded-xl border border-line bg-surface px-3 py-2 text-sm" />
      </Field>
      <Field label="매달 적립할 돈 (원, 선택) — '쓸 수 있는 돈'에서 빠져요">
        <Input inputMode="numeric" value={g.monthly ? String(g.monthly) : ''} onChange={(e) => setG({ ...g, monthly: num(e.target.value) })} />
      </Field>
      <Field label="어떤 자산으로 채우나요 (고른 것의 합이 현재 금액)">
        <div className="flex max-h-48 flex-wrap gap-1.5 overflow-y-auto">
          {names.map((n) => {
            const on = g.items.includes(n)
            return (
              <button key={n} type="button" onClick={() => setG({ ...g, items: on ? g.items.filter((x) => x !== n) : [...g.items, n] })}
                className={`rounded-lg px-2 py-1 text-xs ${on ? 'bg-accent text-accent-ink' : 'bg-surface-2 text-muted'}`}>{n}</button>
            )
          })}
          {names.length === 0 && <p className="text-xs text-faint">재정에 파일을 올리면 고를 수 있어요.</p>}
        </div>
      </Field>
      {err && <p className="text-xs text-rose-500">{err}</p>}
      <div className="flex items-center justify-between pt-1">
        {onDelete ? <Button type="button" variant="ghost" onClick={onDelete} className="text-rose-500">지우기</Button> : <span />}
        <Button type="submit">저장</Button>
      </div>
    </form>
  )
}
