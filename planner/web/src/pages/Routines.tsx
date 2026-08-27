import { useState } from 'react'
import { api, type Routine } from '../api'
import { useInvalidating, useRoutineChecks, useRoutines, useUsers } from '../lib/hooks'
import { addDays, maskBit, startOfWeek, today, WEEKDAYS } from '../lib/date'
import { Avatar, Button, Empty, Field, Input, PageHeader, Sheet, SkeletonList } from '../components/ui'

export default function Routines() {
  const [weekStart, setWeekStart] = useState(() => startOfWeek(today()))
  const weekEnd = addDays(weekStart, 7)
  const days = Array.from({ length: 7 }, (_, i) => addDays(weekStart, i))

  const routines = useRoutines()
  const checks = useRoutineChecks(weekStart, weekEnd)
  const users = useUsers()
  const [editing, setEditing] = useState<Routine | 'new' | null>(null)

  const keys = [['routines']]
  const toggle = useInvalidating(({ id, date, on }: { id: number; date: string; on: boolean }) =>
    on ? api.put(`/api/routines/${id}/checks/${date}`) : api.del(`/api/routines/${id}/checks/${date}`), keys)
  const save = useInvalidating(({ id, ...body }: { id?: number } & Record<string, unknown>) =>
    id ? api.patch(`/api/routines/${id}`, body) : api.post('/api/routines', body), keys)
  const del = useInvalidating((id: number) => api.del(`/api/routines/${id}`), keys)

  const isChecked = (r: Routine, d: string) => checks.data?.[String(r.id)]?.includes(d) ?? false
  // i는 표시 인덱스(일=0)다 — 저장 마스크 비트(월=0)로 옮겨 본다.
  const scheduled = (r: Routine, i: number) => (r.weekdays_mask & (1 << maskBit(i))) !== 0
  const active = routines.data?.filter((r) => r.active) ?? []
  const paused = routines.data?.filter((r) => !r.active) ?? []
  const todayStr = today()

  return (
    <div className="mx-auto max-w-lg">
      <PageHeader title="루틴" right={<Button variant="ghost" onClick={() => setEditing('new')}>+ 추가</Button>} />

      {/* 주 이동 */}
      <div className="flex items-center justify-between px-4 pt-3">
        <button onClick={() => setWeekStart(addDays(weekStart, -7))} className="rounded-lg px-2 py-1 text-slate-500 active:bg-slate-200">‹</button>
        <button onClick={() => setWeekStart(startOfWeek(todayStr))} className="text-sm font-medium text-slate-600">
          {weekStart.slice(5).replace('-', '/')} – {addDays(weekStart, 6).slice(5).replace('-', '/')}
        </button>
        <button onClick={() => setWeekStart(addDays(weekStart, 7))} className="rounded-lg px-2 py-1 text-slate-500 active:bg-slate-200">›</button>
      </div>

      {/* 요일 머리 + 루틴별 카드.
          좁은 화면에서 제목과 7개 동그라미를 한 줄에 넣으면 제목이 "스..."로
          잘린다. 제목을 한 줄 주고 체크는 아래에 펼친다. */}
      <div className="px-4 py-3">
        <ul className="space-y-2">
          {active.map((r) => (
            <li key={r.id} className="rounded-2xl bg-white p-3 shadow-sm">
              <button onClick={() => setEditing(r)} className="flex w-full items-center gap-2 text-left">
                <span className="min-w-0 flex-1 truncate text-sm font-semibold">{r.title}</span>
                {r.time_of_day && (
                  <span className="shrink-0 rounded-md bg-slate-100 px-1.5 py-0.5 text-[11px] font-medium text-slate-500">
                    {r.time_of_day}
                  </span>
                )}
                {r.assignee_id && users.data && (
                  <Avatar name={users.data.find((u) => u.id === r.assignee_id)?.name ?? '?'} />
                )}
              </button>

              <div className="mt-2.5 flex justify-between gap-1">
                {days.map((d, i) => {
                  const on = scheduled(r, i)
                  const done = isChecked(r, d)
                  const isToday = d === todayStr
                  return (
                    <div key={d} className="flex min-w-0 flex-1 flex-col items-center gap-1">
                      <span className={`text-[10px] ${isToday ? 'font-bold text-slate-900' : i === 0 ? 'text-rose-400' : 'text-slate-400'}`}>
                        {WEEKDAYS[i]}
                      </span>
                      {on ? (
                        <button
                          aria-label={`${r.title} ${d}`}
                          onClick={() => toggle.mutate({ id: r.id, date: d, on: !done })}
                          className={`flex h-7 w-7 items-center justify-center rounded-full border-2 transition ${
                            done ? 'border-emerald-500 bg-emerald-500' : d < todayStr ? 'border-rose-300' : 'border-slate-300'
                          } ${isToday ? 'ring-2 ring-slate-900/10' : ''}`}
                        >
                          {done && (
                            <svg viewBox="0 0 24 24" className="h-4 w-4 text-white" fill="none" stroke="currentColor" strokeWidth="3" strokeLinecap="round" strokeLinejoin="round">
                              <path d="M5 13l4 4L19 7" />
                            </svg>
                          )}
                        </button>
                      ) : (
                        <span className="flex h-7 w-7 items-center justify-center">
                          <span className="h-1 w-1 rounded-full bg-slate-200" />
                        </span>
                      )}
                    </div>
                  )
                })}
              </div>
            </li>
          ))}
        </ul>
        {routines.isPending && <SkeletonList rows={3} />}
        {!routines.isPending && active.length === 0 && <Empty>루틴을 추가해보세요</Empty>}

        {paused.length > 0 && (
          <details className="mt-4">
            <summary className="text-xs text-slate-400">쉬는 중 {paused.length}</summary>
            <ul className="mt-2 space-y-1">
              {paused.map((r) => (
                <li key={r.id}><button onClick={() => setEditing(r)} className="w-full rounded-xl bg-white px-3 py-2 text-left text-sm text-slate-400 line-through">{r.title}</button></li>
              ))}
            </ul>
          </details>
        )}
      </div>

      <Sheet open={editing !== null} onClose={() => setEditing(null)} title={editing === 'new' ? '새 루틴' : '루틴'}>
        {editing !== null && (
          <RoutineForm
            key={editing === 'new' ? 'new' : editing.id}
            routine={editing === 'new' ? null : editing}
            users={users.data ?? []}
            onSave={(body) => { save.mutate(editing === 'new' ? body : { id: editing.id, ...body }); setEditing(null) }}
            onDelete={editing === 'new' ? undefined : () => { if (confirm('루틴을 삭제할까요? 체크 기록도 사라져요.')) { del.mutate(editing.id); setEditing(null) } }}
          />
        )}
      </Sheet>
    </div>
  )
}

function RoutineForm({ routine, users, onSave, onDelete }: { routine: Routine | null; users: { id: number; name: string }[]; onSave: (b: Record<string, unknown>) => void; onDelete?: () => void }) {
  const [title, setTitle] = useState(routine?.title ?? '')
  const [mask, setMask] = useState(routine?.weekdays_mask ?? 0x7f)
  const [time, setTime] = useState(routine?.time_of_day ?? '')
  const [assignee, setAssignee] = useState(routine?.assignee_id ?? 0)
  const [active, setActive] = useState(routine?.active ?? true)

  return (
    <div className="space-y-3">
      <Input autoFocus placeholder="예: 아침 스트레칭" value={title} onChange={(e) => setTitle(e.target.value)} />
      <Field label="요일">
        <div className="flex gap-1">
          {WEEKDAYS.map((w, i) => {
            const bit = 1 << maskBit(i)
            const on = (mask & bit) !== 0
            return (
              <button key={w} type="button" onClick={() => setMask(mask ^ bit)} className={`h-9 flex-1 rounded-lg text-sm font-medium ${on ? 'bg-slate-900 text-white' : i === 0 ? 'bg-slate-100 text-rose-400' : 'bg-slate-100 text-slate-500'}`}>{w}</button>
            )
          })}
        </div>
      </Field>
      <Field label="시간 (선택)">
        <Input type="time" value={time} onChange={(e) => setTime(e.target.value)} />
      </Field>
      <Field label="담당">
        <select value={assignee} onChange={(e) => setAssignee(Number(e.target.value))} className="w-full rounded-xl border border-slate-200 bg-white px-3 py-2.5 text-base">
          <option value={0}>없음</option>
          {users.map((u) => <option key={u.id} value={u.id}>{u.name}</option>)}
        </select>
      </Field>
      {routine && (
        <label className="flex items-center gap-2 text-sm">
          <input type="checkbox" checked={active} onChange={(e) => setActive(e.target.checked)} className="h-4 w-4" />
          활성
        </label>
      )}
      <div className="flex gap-2 pt-1">
        {onDelete && <Button variant="danger" onClick={onDelete}>삭제</Button>}
        <div className="flex-1" />
        <Button disabled={!title.trim() || mask === 0} onClick={() => onSave({ title: title.trim(), weekdays_mask: mask, time_of_day: time, assignee_id: assignee, active })}>저장</Button>
      </div>
    </div>
  )
}
