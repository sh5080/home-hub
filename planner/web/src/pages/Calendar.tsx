import { useMemo, useState } from 'react'
import { Link } from 'react-router'
import { api, type Event } from '../api'
import { useCalendar, useInvalidating } from '../lib/hooks'
import { addDays, addMonths, fmtDate, fmtTime, startOfMonth, startOfWeek, today, weekdayIndex, WEEKDAYS } from '../lib/date'
import { Button, Field, Input, PageHeader, Sheet } from '../components/ui'

export default function Calendar() {
  const [month, setMonth] = useState(() => startOfMonth(today()))
  const [selected, setSelected] = useState(today())
  const [editing, setEditing] = useState<Event | 'new' | null>(null)

  // 그리드는 그 달 1일이 속한 주의 월요일부터 6주
  const gridStart = startOfWeek(month)
  const gridEnd = addDays(gridStart, 42)
  const cal = useCalendar(gridStart, gridEnd)

  const keys = [['calendar']]
  const save = useInvalidating(({ id, ...body }: { id?: number } & Record<string, unknown>) =>
    id ? api.patch(`/api/events/${id}`, body) : api.post('/api/events', body), keys)
  const del = useInvalidating((id: number) => api.del(`/api/events/${id}`), keys)

  // 날짜 → 그날의 항목. 여러 날짜 이벤트는 각 날에 펼친다.
  const byDate = useMemo(() => {
    const m = new Map<string, { events: Event[]; due: { id: number; title: string }[] }>()
    const get = (d: string) => { let v = m.get(d); if (!v) { v = { events: [], due: [] }; m.set(d, v) } return v }
    for (const e of cal.data?.events ?? []) {
      const s = e.start_at.slice(0, 10)
      const end = (e.end_at ?? e.start_at).slice(0, 10)
      for (let d = s; d <= end; d = addDays(d, 1)) get(d).events.push(e)
    }
    for (const c of cal.data?.due_cards ?? []) if (c.due_at) get(c.due_at).due.push(c)
    return m
  }, [cal.data])

  const days = Array.from({ length: 42 }, (_, i) => addDays(gridStart, i))
  const monthNum = Number(month.slice(5, 7))
  const sel = byDate.get(selected)

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
        <div className="grid grid-cols-7 text-center text-[11px] text-slate-400">
          {WEEKDAYS.map((w, i) => <div key={w} className={i === 0 ? 'text-rose-400' : i === 6 ? 'text-sky-400' : ''}>{w}</div>)}
        </div>
        <div className="mt-1 grid grid-cols-7 gap-y-1">
          {days.map((d) => {
            const inMonth = Number(d.slice(5, 7)) === monthNum
            const v = byDate.get(d)
            const isSel = d === selected
            const isToday = d === today()
            return (
              <button
                key={d}
                onClick={() => setSelected(d)}
                className={`flex h-14 flex-col items-center rounded-xl pt-1 ${isSel ? 'bg-slate-900 text-white' : inMonth ? 'text-slate-800' : 'text-slate-300'}`}
              >
                <span className={`flex h-6 w-6 items-center justify-center rounded-full text-sm ${isToday && !isSel ? 'bg-slate-200 font-bold' : ''} ${weekdayIndex(d) === 0 && !isSel && inMonth ? 'text-rose-500' : weekdayIndex(d) === 6 && !isSel && inMonth ? 'text-sky-600' : ''}`}>
                  {Number(d.slice(8))}
                </span>
                <span className="mt-0.5 flex gap-0.5">
                  {v?.events.slice(0, 3).map((e) => <i key={e.id} className={`h-1.5 w-1.5 rounded-full ${isSel ? 'bg-white' : 'bg-sky-500'}`} />)}
                  {v?.due.slice(0, 2).map((c) => <i key={c.id} className={`h-1.5 w-1.5 rounded-full ${isSel ? 'bg-white/70' : 'bg-amber-500'}`} />)}
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
          <Button variant="ghost" onClick={() => setEditing('new')}>+ 일정</Button>
        </div>
        <ul className="mt-2 space-y-2">
          {sel?.events.map((e) => (
            <li key={e.id}>
              <button onClick={() => setEditing(e)} className="flex w-full items-center gap-3 rounded-xl bg-white p-3 text-left shadow-sm">
                <span className="h-8 w-1 rounded-full bg-sky-500" />
                <div className="min-w-0 flex-1">
                  <p className="truncate text-sm font-medium">{e.title}</p>
                  <p className="text-xs text-slate-400">{e.all_day ? '종일' : fmtTime(e.start_at) + (e.end_at ? ` – ${fmtTime(e.end_at)}` : '')}</p>
                </div>
              </button>
            </li>
          ))}
          {sel?.due.map((c) => (
            <li key={`c${c.id}`}>
              <Link to={`/boards`} className="flex w-full items-center gap-3 rounded-xl bg-white p-3 shadow-sm">
                <span className="h-8 w-1 rounded-full bg-amber-500" />
                <div className="min-w-0 flex-1">
                  <p className="truncate text-sm font-medium">{c.title}</p>
                  <p className="text-xs text-slate-400">마감</p>
                </div>
              </Link>
            </li>
          ))}
          {!sel?.events.length && !sel?.due.length && <li className="py-6 text-center text-sm text-slate-400">일정이 없어요</li>}
        </ul>
      </section>

      <Sheet open={editing !== null} onClose={() => setEditing(null)} title={editing === 'new' ? '새 일정' : '일정'}>
        {editing !== null && (
          <EventForm
            key={editing === 'new' ? `new-${selected}` : editing.id}
            event={editing === 'new' ? null : editing}
            defaultDate={selected}
            onSave={(body) => { save.mutate(editing === 'new' ? body : { id: editing.id, ...body }); setEditing(null) }}
            onDelete={editing === 'new' ? undefined : () => { if (confirm('일정을 삭제할까요?')) { del.mutate(editing.id); setEditing(null) } }}
          />
        )}
      </Sheet>
    </div>
  )
}

function EventForm({ event, defaultDate, onSave, onDelete }: { event: Event | null; defaultDate: string; onSave: (b: Record<string, unknown>) => void; onDelete?: () => void }) {
  const [title, setTitle] = useState(event?.title ?? '')
  const [allDay, setAllDay] = useState(event?.all_day ?? false)
  // 종일이면 date, 아니면 datetime-local. 전환 시 날짜 부분은 유지.
  const initStart = event?.start_at ?? `${defaultDate}T${nextHour()}`
  const [start, setStart] = useState(initStart)
  const [end, setEnd] = useState(event?.end_at ?? '')

  const dateOf = (s: string) => s.slice(0, 10)
  const switchAllDay = (v: boolean) => {
    setAllDay(v)
    setStart(v ? dateOf(start) : `${dateOf(start)}T${nextHour()}`)
    setEnd(end ? (v ? dateOf(end) : `${dateOf(end)}T${nextHour(1)}`) : '')
  }

  return (
    <div className="space-y-3">
      <Input autoFocus placeholder="제목" value={title} onChange={(e) => setTitle(e.target.value)} />
      <label className="flex items-center gap-2 text-sm">
        <input type="checkbox" checked={allDay} onChange={(e) => switchAllDay(e.target.checked)} className="h-4 w-4" />
        종일
      </label>
      <Field label="시작">
        <Input type={allDay ? 'date' : 'datetime-local'} value={start} onChange={(e) => setStart(e.target.value)} />
      </Field>
      <Field label="끝 (선택)">
        <Input type={allDay ? 'date' : 'datetime-local'} value={end} min={start} onChange={(e) => setEnd(e.target.value)} />
      </Field>
      <div className="flex gap-2 pt-1">
        {onDelete && <Button variant="danger" onClick={onDelete}>삭제</Button>}
        <div className="flex-1" />
        <Button disabled={!title.trim() || !start} onClick={() => onSave({ title: title.trim(), start_at: start, end_at: end, all_day: allDay })}>저장</Button>
      </div>
    </div>
  )
}

/** 지금부터 다음 정시 'HH:00' */
function nextHour(plus = 0) {
  const h = (new Date().getHours() + 1 + plus) % 24
  return `${h < 10 ? '0' : ''}${h}:00`
}

