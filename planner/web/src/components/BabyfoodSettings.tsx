import { useEffect, useState } from 'react'
import { api, type BFProfile } from '../api'
import { useBFProfile, useInvalidating } from '../lib/hooks'
import { Button, Field, Input } from './ui'

// 아이 생일과 재고 계산 기간.
//
// 생일은 코드나 데이터 파일에 없다 — 여기서 받는다. 식단은 D+n 으로 저장돼
// 있어서, 생일을 고치면 194일치가 통째로 날짜만 다시 붙는다.
export default function BabyfoodSettings() {
  const q = useBFProfile()
  const save = useInvalidating(
    (body: Partial<Pick<BFProfile, 'name' | 'birth_date' | 'horizon_days'>>) =>
      api.patch<BFProfile>('/api/babyfood/profile', body),
    [['babyfood'], ['babyfood-stock']],
  )

  const [name, setName] = useState('')
  const [birth, setBirth] = useState('')
  const [days, setDays] = useState('21')
  const [err, setErr] = useState<string | null>(null)
  const [saved, setSaved] = useState(false)

  // 서버 값이 오면 한 번 채운다.
  useEffect(() => {
    if (!q.data) return
    setName(q.data.name)
    setBirth(q.data.birth_date ?? '')
    setDays(String(q.data.horizon_days))
  }, [q.data])

  if (q.isPending) return null

  const p = q.data!
  const weeks = Number(days) / 7

  return (
    <form
      onSubmit={async (e) => {
        e.preventDefault()
        setErr(null); setSaved(false)
        try {
          await save.mutateAsync({ name, birth_date: birth, horizon_days: Number(days) })
          setSaved(true)
        } catch (ex) {
          setErr(ex instanceof Error ? ex.message : '저장하지 못했어요')
        }
      }}
    >
      <p className="mb-2 text-xs font-medium text-slate-500">이유식</p>
      <div className="space-y-2 rounded-xl bg-slate-50 p-3">
        <Field label="아이 이름">
          <Input value={name} onChange={(e) => setName(e.target.value)} placeholder="선택" />
        </Field>
        <Field label="생일">
          <Input type="date" value={birth} onChange={(e) => setBirth(e.target.value)} />
        </Field>
        <Field label="재고를 며칠치로 계산할까요">
          <Input inputMode="numeric" value={days} onChange={(e) => setDays(e.target.value)} />
        </Field>
        <p className="text-[11px] text-slate-400">
          {Number.isFinite(weeks) && weeks > 0 ? `약 ${weeks % 1 === 0 ? weeks : weeks.toFixed(1)}주치` : '일 수를 적어주세요'}
          {p.today_dday != null && ` · 오늘 D+${p.today_dday}`}
          {p.from_dday != null && ` · 식단 D+${p.from_dday}~D+${p.to_dday}`}
        </p>
        {err && <p className="text-xs text-rose-500">{err}</p>}
        {saved && <p className="text-xs text-emerald-600">저장했어요</p>}
        <div className="flex justify-end">
          <Button type="submit" disabled={save.isPending}>저장</Button>
        </div>
      </div>
    </form>
  )
}
