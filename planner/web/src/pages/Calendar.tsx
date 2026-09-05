import { useEffect, useMemo, useState } from 'react'
import { useNavigate, useSearchParams } from 'react-router'
import { api, type Card } from '../api'
import { useCalendar, useDiary, useInvalidating, useBoards, useUsers } from '../lib/hooks'
import { addDays, addMonths, ampm, fmtDate, fmtTime, hasTime, startOfMonth, startOfWeek, today, weekdayIndex, WEEKDAYS } from '../lib/date'
import { Avatar, Button, Field, Input, PageHeader, Sheet, SkeletonList } from '../components/ui'
import { useDiaryViewer } from '../components/DiaryViewer'
import { Stars } from '../components/SortToggle'

// 캘린더는 날짜가 있는 카드를 기간으로 보는 뷰다.
export default function Calendar() {
  const nav = useNavigate()
  // ?date= 로 특정 날짜를 연다.
  const [params, setParams] = useSearchParams()
  const linked = params.get('date')
  const valid = (d: string | null) => !!d && /^\d{4}-\d{2}-\d{2}$/.test(d)
  const initial = valid(linked) ? linked! : today()

  const [month, setMonth] = useState(() => startOfMonth(initial))
  const [selected, setSelectedState] = useState(initial)
  const [adding, setAdding] = useState(false)

  // 날짜 선택은 주소만 바꾸고 히스토리에 쌓지 않는다.
  const setSelected = (d: string) => {
    setSelectedState(d)
    setParams({ date: d }, { replace: true })
  }

  useEffect(() => {
    if (valid(linked) && linked !== selected) {
      setSelectedState(linked!)
      setMonth(startOfMonth(linked!))
    }
    // linked 만 본다 — selected 를 넣으면 사용자가 고른 날짜를 되돌린다.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [linked])

  // 그리드는 1일이 속한 주의 일요일부터 6주
  const gridStart = startOfWeek(month)
  const gridEnd = addDays(gridStart, 42)
  const cal = useCalendar(gridStart, gridEnd)
  const users = useUsers()
  const boards = useBoards()

  const create = useInvalidating(
    ({ col, ...body }: { col: number } & Record<string, unknown>) => api.post<Card>(`/api/columns/${col}/cards`, body),
    [['calendar'], ['board'], ['boards'], ['today']],
  )

  // 여러 날 항목은 걸치는 날마다 펼친다.
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
  const hasDiary = new Set(cal.data?.diary_dates ?? [])
  const userName = (id: number | null) => users.data?.find((u) => u.id === id)?.name

  return (
    <div className="mx-auto max-w-lg">
      <PageHeader
        title={`${month.slice(0, 4)}년 ${monthNum}월`}
        right={
          <div className="flex items-center gap-1">
            <button onClick={() => setMonth(addMonths(month, -1))} className="rounded-lg px-2 py-1 text-muted active:bg-line">‹</button>
            <button onClick={() => { setMonth(startOfMonth(today())); setSelected(today()) }} className="rounded-lg px-2 py-1 text-xs font-medium text-ink-2 active:bg-line">오늘</button>
            <button onClick={() => setMonth(addMonths(month, 1))} className="rounded-lg px-2 py-1 text-muted active:bg-line">›</button>
          </div>
        }
      />


      <div className="px-2 pt-2">
        <div className="mb-1.5 flex items-center justify-end gap-3 px-1 text-[10px] text-faint">
          <span className="flex items-center gap-1"><i className="h-1.5 w-1.5 rounded-full bg-sky-500" />시간 약속</span>
          <span className="flex items-center gap-1"><i className="h-1.5 w-1.5 rounded-full bg-amber-500" />종일</span>
        </div>
        <div className="grid grid-cols-7 text-center text-[11px] text-faint">
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
                className={`flex h-14 flex-col items-center rounded-xl pt-1 ${isSel ? 'bg-accent text-accent-ink' : inMonth ? 'text-ink' : 'text-ghost'}`}
              >
                <span className={`flex h-6 w-6 items-center justify-center rounded-full text-sm ${isToday && !isSel ? 'bg-line font-bold' : ''} ${!isSel && inMonth ? (wd === 0 ? 'text-rose-500' : wd === 6 ? 'text-sky-600' : '') : ''}`}>
                  {Number(d.slice(8))}
                </span>
                <span className="mt-0.5 flex gap-0.5">
                  {list?.slice(0, 3).map((c) => (
                    <i key={c.id} className={`h-1.5 w-1.5 rounded-full ${isSel ? 'bg-surface' : hasTime(c.due_at!) ? 'bg-sky-500' : 'bg-amber-500'}`} />
                  ))}
                  {/* 일기는 속을 비운 동그라미(일정 점과 구분). */}
                  {hasDiary.has(d) && (
                    <i className={`h-1.5 w-1.5 rounded-full border ${isSel ? 'border-surface' : 'border-emerald-500'}`} />
                  )}
                </span>
              </button>
            )
          })}
        </div>
      </div>

      <section className="px-4 pt-4">
        <div className="flex items-center justify-between">
          <h2 className="text-base font-bold">{fmtDate(selected)}</h2>
          <Button variant="ghost" onClick={() => setAdding(true)}>+ 추가</Button>
        </div>
        <ul className="mt-2 space-y-2">
          {sel.map((c) => (
            <li key={c.id}>
              <button onClick={() => nav(`/cards/${c.id}`)} className="flex w-full items-center gap-3 rounded-2xl bg-surface p-3 text-left shadow-sm active:bg-canvas">
                <span className={`h-8 w-1 shrink-0 rounded-full ${hasTime(c.due_at!) ? 'bg-sky-500' : 'bg-amber-500'}`} />
                <div className="min-w-0 flex-1">
                  <p className="truncate text-sm font-medium">{c.title}</p>
                  <p className="text-xs text-faint">
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
          {!cal.isPending && sel.length === 0 && <li className="py-6 text-center text-sm text-faint">일정이 없어요</li>}
        </ul>

        <DiaryOfDay date={selected} />
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
      {boardName && <p className="text-xs text-faint">보드 “{boardName}”의 첫 컬럼에 추가돼요 — 칸반에서도 보입니다.</p>}
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

/** 그 날의 일기(다이어리 탭과 같은 글). */
function DiaryOfDay({ date }: { date: string }) {
  const nav = useNavigate()
  const viewer = useDiaryViewer()
  const list = useDiary(date, date)
  const entries = list.data ?? []

  return (
    <div className="mt-4">
      <div className="flex items-center justify-between">
        <h3 className="text-sm font-bold text-ink-2">일기</h3>
        <button
          onClick={() => nav(`/diary/new?date=${date}`)}
          className="text-xs font-medium text-muted"
        >
          + 쓰기
        </button>
      </div>
      {entries.length === 0 ? (
        <p className="py-3 text-center text-xs text-faint">이 날 쓴 일기가 없어요</p>
      ) : (
        <ul className="mt-2 space-y-1.5">
          {entries.map((e) => (
            <li key={e.id}>
              <button
                onClick={() => viewer.open(e.id)}
                className="w-full rounded-2xl bg-surface p-3 text-left shadow-sm active:bg-canvas"
              >
                <div className="flex gap-3">
                  <div className="min-w-0 flex-1">
                    {e.title && <p className="truncate text-sm font-semibold">{e.title}</p>}
                    {e.plain && <p className={`line-clamp-2 text-xs text-muted ${e.title ? 'mt-0.5' : ''}`}>{e.plain}</p>}
                    {!e.title && !e.plain && <p className="text-xs text-faint">빈 일기</p>}
                  </div>
                  {e.photos[0] && <img src={e.photos[0]} alt="" loading="lazy" className="h-12 w-12 shrink-0 rounded-lg object-cover" />}
                </div>
              </button>
            </li>
          ))}
        </ul>
      )}
    </div>
  )
}
