import { useEffect, useMemo, useState } from 'react'
import { useNavigate, useSearchParams } from 'react-router'
import { api, type Card } from '../api'
import { useCalendar, useInvalidating, useBoards, useUsers } from '../lib/hooks'
import { addDays, addMonths, ampm, fmtDate, fmtTime, hasTime, startOfMonth, startOfWeek, today, weekdayIndex, WEEKDAYS } from '../lib/date'
import { Avatar, Button, Field, Input, PageHeader, Sheet, SkeletonList } from '../components/ui'
import { Stars } from '../components/SortToggle'

// 캘린더는 별도 데이터가 아니라 **날짜가 있는 카드**를 기간으로 보는 뷰다.
// 여기서 만든 항목도 카드이므로 칸반에서 그대로 보이고, 반대도 마찬가지다.
export default function Calendar() {
  const nav = useNavigate()
  // ?date=YYYY-MM-DD 로 특정 날짜를 열 수 있다 — 홈의 "앞으로 7일"에서 넘어온다.
  const [params, setParams] = useSearchParams()
  const linked = params.get('date')
  const valid = (d: string | null) => !!d && /^\d{4}-\d{2}-\d{2}$/.test(d)
  const initial = valid(linked) ? linked! : today()

  const [month, setMonth] = useState(() => startOfMonth(initial))
  const [selected, setSelectedState] = useState(initial)
  const [adding, setAdding] = useState(false)

  // 날짜를 고를 때마다 주소를 갱신하되 히스토리에 쌓지 않는다 — 뒤로가기가
  // 날짜 선택을 하나씩 되짚는 건 원하는 동작이 아니다.
  const setSelected = (d: string) => {
    setSelectedState(d)
    setParams({ date: d }, { replace: true })
  }

  // 홈에서 다른 날짜로 다시 들어오면 그 날짜로 옮긴다.
  useEffect(() => {
    if (valid(linked) && linked !== selected) {
      setSelectedState(linked!)
      setMonth(startOfMonth(linked!))
    }
    // linked 만 본다 — selected 를 넣으면 사용자가 고른 날짜를 되돌린다.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [linked])

  // 그리드는 그 달 1일이 속한 주의 일요일부터 6주
  const gridStart = startOfWeek(month)
  const gridEnd = addDays(gridStart, 42)
  const cal = useCalendar(gridStart, gridEnd)
  const users = useUsers()
  const boards = useBoards()

  const create = useInvalidating(
    ({ col, ...body }: { col: number } & Record<string, unknown>) => api.post<Card>(`/api/columns/${col}/cards`, body),
    [['calendar'], ['board'], ['boards'], ['today']],
  )

  // 날짜 → 그날의 카드. 여러 날 항목은 걸치는 날마다 펼친다.
  const byDate = useMemo(() => {
    const m = new Map<string, Card[]>()
    for (const c of cal.data?.cards ?? []) {
      if (!c.due_at) continue
      const s = c.due_at.slice(0, 10)
      const e = (c.end_at ?? c.due_at).slice(0, 10)
      for (let d = s; d <= e; d = addDays(d, 1)) {
        const list = m.get(d)
        if (list) list.push(c)
        else m.set(d, [c])
      }
    }
    return m
  }, [cal.data])

  const days = Array.from({ length: 42 }, (_, i) => addDays(gridStart, i))
  const monthNum = Number(month.slice(5, 7))
  const sel = byDate.get(selected) ?? []
  const userName = (id: number | null) => users.data?.find((u) => u.id === id)?.name

  return (
    <div className="mx-auto max-w-lg">
      <PageHeader
        title={`${month.slice(0, 4)}년 ${monthNum}월`}
        right={
          <div className="flex items-center gap-1">
            <button onClick={() => setMonth(addMonths(month, -1))} className="rounded-lg px-2 py-1 text-slate-500 active:bg-slate-200">‹</button>
            <button onClick={() => { setMonth(startOfMonth(today())); setSelected(today()) }} className="rounded-lg px-2 py-1 text-xs font-medium text-slate-600 active:bg-slate-200">오늘</button>
            <button onClick={() => setMonth(addMonths(month, 1))} className="rounded-lg px-2 py-1 text-slate-500 active:bg-slate-200">›</button>
          </div>
        }
      />

      <div className="px-2 pt-2">
        {/* 점 색이 뭘 뜻하는지 — 두 색뿐이라 오전/오후로 오해하기 쉽다 */}
        <div className="mb-1.5 flex items-center justify-end gap-3 px-1 text-[10px] text-slate-400">
          <span className="flex items-center gap-1"><i className="h-1.5 w-1.5 rounded-full bg-sky-500" />시간 약속</span>
          <span className="flex items-center gap-1"><i className="h-1.5 w-1.5 rounded-full bg-amber-500" />종일</span>
        </div>
        <div className="grid grid-cols-7 text-center text-[11px] text-slate-400">
          {WEEKDAYS.map((w, i) => (
            <div key={w} className={i === 0 ? 'text-rose-400' : i === 6 ? 'text-sky-400' : ''}>{w}</div>
          ))}
        </div>
        <div className="mt-1 grid grid-cols-7 gap-y-1">
          {days.map((d) => {
            const inMonth = Number(d.slice(5, 7)) === monthNum
            const list = byDate.get(d)
            const isSel = d === selected
            const isToday = d === today()
            const wd = weekdayIndex(d)
            return (
              <button
                key={d}
                onClick={() => setSelected(d)}
                className={`flex h-14 flex-col items-center rounded-xl pt-1 ${isSel ? 'bg-slate-900 text-white' : inMonth ? 'text-slate-800' : 'text-slate-300'}`}
              >
                <span className={`flex h-6 w-6 items-center justify-center rounded-full text-sm ${isToday && !isSel ? 'bg-slate-200 font-bold' : ''} ${!isSel && inMonth ? (wd === 0 ? 'text-rose-500' : wd === 6 ? 'text-sky-600' : '') : ''}`}>
                  {Number(d.slice(8))}
                </span>
                <span className="mt-0.5 flex gap-0.5">
                  {list?.slice(0, 4).map((c) => (
                    <i key={c.id} className={`h-1.5 w-1.5 rounded-full ${isSel ? 'bg-white' : hasTime(c.due_at!) ? 'bg-sky-500' : 'bg-amber-500'}`} />
                  ))}
                </span>
              </button>
            )
          })}
        </div>
      </div>

      {/* 선택한 날 */}
      <section className="px-4 pt-4">
        <div className="flex items-center justify-between">
          <h2 className="text-base font-bold">{fmtDate(selected)}</h2>
          <Button variant="ghost" onClick={() => setAdding(true)}>+ 추가</Button>
        </div>
        <ul className="mt-2 space-y-2">
          {sel.map((c) => (
            <li key={c.id}>
              {/* 카드이므로 상세 페이지가 그대로 열린다 */}
              <button onClick={() => nav(`/cards/${c.id}`)} className="flex w-full items-center gap-3 rounded-xl bg-white p-3 text-left shadow-sm active:bg-slate-50">
                <span className={`h-8 w-1 shrink-0 rounded-full ${hasTime(c.due_at!) ? 'bg-sky-500' : 'bg-amber-500'}`} />
                <div className="min-w-0 flex-1">
                  <p className="truncate text-sm font-medium">{c.title}</p>
                  <p className="text-xs text-slate-400">
                    {hasTime(c.due_at!) ? `${ampm(c.due_at!)} ${fmtTime(c.due_at!)}` : '종일'}
                    {c.end_at && c.end_at.slice(0, 10) !== c.due_at!.slice(0, 10) && ` – ${c.end_at.slice(5, 10).replace('-', '/')}`}
                  </p>
                </div>
                <Stars n={c.priority} />
                {c.assignee_id && userName(c.assignee_id) && <Avatar name={userName(c.assignee_id)!} />}
              </button>
            </li>
          ))}
          {cal.isPending && <li><SkeletonList rows={2} /></li>}
          {!cal.isPending && sel.length === 0 && <li className="py-6 text-center text-sm text-slate-400">일정이 없어요</li>}
        </ul>
      </section>

      <Sheet open={adding} onClose={() => setAdding(false)} title={`${fmtDate(selected)}에 추가`}>
        <AddForm
          date={selected}
          boardName={boards.data?.[0]?.name}
          onSubmit={async (body) => {
            const col = boards.data?.[0]
            if (!col) return
            const detail = await api.get<{ columns: { id: number }[] }>(`/api/boards/${col.id}`)
            const card = await create.mutateAsync({ col: detail.columns[0].id, ...body })
            setAdding(false)
            nav(`/cards/${card.id}`)
          }}
          onClose={() => setAdding(false)}
        />
      </Sheet>
    </div>
  )
}

// 캘린더에서 만드는 것도 카드다. 제목과 날짜만 받고 나머지는 카드 페이지에서.
function AddForm({ date, boardName, onSubmit, onClose }: {
  date: string
  boardName?: string
  onSubmit: (body: Record<string, unknown>) => void
  onClose: () => void
}) {
  const [title, setTitle] = useState('')
  const [timed, setTimed] = useState(false)
  const [due, setDue] = useState(date)
  const [end, setEnd] = useState('')
  const [busy, setBusy] = useState(false)

  const toggleTimed = (withTime: boolean) => {
    setTimed(withTime)
    setDue(withTime ? `${due.slice(0, 10)}T${nextHour()}` : due.slice(0, 10))
    if (end) setEnd(withTime ? `${end.slice(0, 10)}T${nextHour(1)}` : end.slice(0, 10))
  }

  return (
    <form
      onSubmit={(e) => { e.preventDefault(); if (title.trim()) { setBusy(true); onSubmit({ title: title.trim(), due_at: due, end_at: end }) } }}
      className="space-y-3"
    >
      <Input autoFocus placeholder="무엇을 할까요" value={title} onChange={(e) => setTitle(e.target.value)} className="text-lg font-semibold" />
      <label className="flex items-center gap-2 text-sm">
        <input type="checkbox" checked={timed} onChange={(e) => toggleTimed(e.target.checked)} className="h-4 w-4" />
        시간 정하기
      </label>
      <Field label="날짜">
        <Input type={timed ? 'datetime-local' : 'date'} value={due} onChange={(e) => setDue(e.target.value)} />
      </Field>
      <Field label="끝 (여러 날에 걸칠 때만)">
        <Input type={timed ? 'datetime-local' : 'date'} value={end} min={due} onChange={(e) => setEnd(e.target.value)} />
      </Field>
      {boardName && <p className="text-xs text-slate-400">보드 “{boardName}”의 첫 컬럼에 추가돼요 — 칸반에서도 보입니다.</p>}
      <div className="flex justify-end gap-2 pt-1">
        <Button type="button" variant="ghost" onClick={onClose}>취소</Button>
        <Button type="submit" disabled={!title.trim() || !due || busy}>{busy ? '여는 중…' : '추가하고 열기'}</Button>
      </div>
    </form>
  )
}

/** 지금부터 다음 정시 'HH:00' */
function nextHour(plus = 0) {
  const h = (new Date().getHours() + 1 + plus) % 24
  return `${h < 10 ? '0' : ''}${h}:00`
}
