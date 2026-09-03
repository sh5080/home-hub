import { useEffect, useState } from 'react'
import { useSearchParams } from 'react-router'
import { useConfirm } from '../components/Confirm'
import { api, type Routine } from '../api'
import { useInvalidating, useRoutineChecks, useRoutines, useUsers } from '../lib/hooks'
import { addDays, maskBit, startOfWeek, today, WEEKDAYS } from '../lib/date'
import { Avatar, Button, Empty, Field, Input, PageHeader, Sheet, SkeletonList } from '../components/ui'
import Byline from '../components/Byline'
import DateNav from '../components/DateNav'

export default function Routines() {
  const confirm = useConfirm()
  const [weekStart, setWeekStart] = useState(() => startOfWeek(today()))
  const weekEnd = addDays(weekStart, 7)
  const days = Array.from({ length: 7 }, (_, i) => addDays(weekStart, i))

  const routines = useRoutines()
  const checks = useRoutineChecks(weekStart, weekEnd)
  const users = useUsers()
  const [editing, setEditing] = useState<Routine | 'new' | null>(null)

  // ?edit=<id> 로 오면 그 루틴을 바로 연다.
  const [params, setParams] = useSearchParams()
  const editID = params.get('edit')
  useEffect(() => {
    if (!editID || !routines.data) return
    const r = routines.data.find((x) => String(x.id) === editID)
    if (r) setEditing(r)
    setParams({}, { replace: true })
  }, [editID, routines.data, setParams])

  const keys = [['routines']]
  const toggle = useInvalidating(({ id, date, on }: { id: number; date: string; on: boolean }) =>
    on ? api.put(`/api/routines/${id}/checks/${date}`) : api.del(`/api/routines/${id}/checks/${date}`), keys)
  const save = useInvalidating(({ id, ...body }: { id?: number } & Record<string, unknown>) =>
    id ? api.patch(`/api/routines/${id}`, body) : api.post('/api/routines', body), keys)
  const del = useInvalidating((id: number) => api.del(`/api/routines/${id}`), keys)

  const isChecked = (r: Routine, d: string) => checks.data?.[String(r.id)]?.includes(d) ?? false
  // i 는 표시 인덱스(일=0) → 저장 비트(월=0)로.
  const scheduled = (r: Routine, i: number) => (r.weekdays_mask & (1 << maskBit(i))) !== 0
  const active = routines.data?.filter((r) => r.active) ?? []
  const paused = routines.data?.filter((r) => !r.active) ?? []
  const todayStr = today()

  return (
    <div className="mx-auto max-w-lg">
      <PageHeader title="루틴" right={<Button variant="ghost" onClick={() => setEditing('new')}>+ 추가</Button>} />


      <div className="px-4 pt-3">
        <DateNav
          value={weekStart === startOfWeek(todayStr) ? todayStr : weekStart}
          label={`${weekStart.slice(5).replace('-', '/')} – ${addDays(weekStart, 6).slice(5).replace('-', '/')}`}
          isToday={weekStart === startOfWeek(todayStr)}
          onPick={(d) => setWeekStart(startOfWeek(d))}
          onPrev={() => setWeekStart(addDays(weekStart, -7))}
          onNext={() => setWeekStart(addDays(weekStart, 7))}
          onToday={() => setWeekStart(startOfWeek(todayStr))}
        />
      </div>

      {/* 제목 한 줄, 체크는 아래에(한 줄에 넣으면 제목이 잘린다). */}
      <div className="px-4 py-3">
        <ul className="space-y-2">
          {active.map((r) => (
            <li key={r.id} className="rounded-2xl bg-surface p-3 shadow-sm">
              <button onClick={() => setEditing(r)} className="flex w-full items-center gap-2 text-left">
                <span className="min-w-0 flex-1 truncate text-sm font-semibold">{r.title}</span>
                {r.time_of_day && (
                  <span className="shrink-0 rounded-md bg-surface-2 px-1.5 py-0.5 text-[11px] font-medium text-muted">
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
                      <span className={`text-[10px] ${isToday ? 'font-bold text-ink' : i === 0 ? 'text-rose-400' : 'text-faint'}`}>
                        {WEEKDAYS[i]}
                      </span>
                      {on ? (
                        <button
                          aria-label={`${r.title} ${d}`}
                          onClick={() => toggle.mutate({ id: r.id, date: d, on: !done })}
                          className={`flex h-7 w-7 items-center justify-center rounded-full border-2 transition ${
                            done ? 'border-emerald-500 bg-emerald-500' : d < todayStr ? 'border-rose-300' : 'border-line'
                          } ${isToday ? 'ring-2 ring-accent/10' : ''}`}
                        >
                          {done && (
                            <svg viewBox="0 0 24 24" className="h-4 w-4 text-white" fill="none" stroke="currentColor" strokeWidth="3" strokeLinecap="round" strokeLinejoin="round">
                              <path d="M5 13l4 4L19 7" />
                            </svg>
                          )}
                        </button>
                      ) : (
                        <span className="flex h-7 w-7 items-center justify-center">
                          <span className="h-1 w-1 rounded-full bg-line" />
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
            <summary className="text-xs text-faint">쉬는 중 {paused.length}</summary>
            <ul className="mt-2 space-y-1">
              {paused.map((r) => (
                <li key={r.id}><button onClick={() => setEditing(r)} className="w-full rounded-xl bg-surface px-3 py-2 text-left text-sm text-faint line-through">{r.title}</button></li>
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
            onDelete={editing === 'new' ? undefined : async () => { if (await confirm({ title: '루틴을 삭제할까요?', body: '지금까지의 체크 기록도 함께 사라져요.', confirmLabel: '삭제', danger: true })) { del.mutate(editing.id); setEditing(null) } }}
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
              <button key={w} type="button" onClick={() => setMask(mask ^ bit)} className={`h-9 flex-1 rounded-lg text-sm font-medium ${on ? 'bg-accent text-accent-ink' : i === 0 ? 'bg-surface-2 text-rose-400' : 'bg-surface-2 text-muted'}`}>{w}</button>
            )
          })}
        </div>
      </Field>
      <Field label="시간 (선택)">
        <Input type="time" value={time} onChange={(e) => setTime(e.target.value)} />
      </Field>
      <Field label="담당">
        <select value={assignee} onChange={(e) => setAssignee(Number(e.target.value))} className="w-full rounded-xl border border-line bg-surface px-3 py-2.5 text-base">
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
      {routine && <Byline by={routine.created_by} at={routine.created_at} />}
      <div className="flex gap-2 pt-1">
        {onDelete && <Button variant="danger" onClick={onDelete}>삭제</Button>}
        <div className="flex-1" />
        <Button disabled={!title.trim() || mask === 0} onClick={() => onSave({ title: title.trim(), weekdays_mask: mask, time_of_day: time, assignee_id: assignee, active })}>저장</Button>
      </div>
    </div>
  )
}
