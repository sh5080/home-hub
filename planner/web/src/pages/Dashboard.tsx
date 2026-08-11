import { useState } from 'react'
import { Link, useNavigate } from 'react-router'
import { useQueryClient } from '@tanstack/react-query'
import { api } from '../api'
import { useCalendar, useInvalidating, useMe, useToday, useUsers } from '../lib/hooks'
import { addDays, dueLabel, fmtDate, fmtTime, isoWeekday, today, WEEKDAYS } from '../lib/date'
import { Avatar, Button, Field, Input, PageHeader, Sheet } from '../components/ui'

export default function Dashboard() {
  const me = useMe()
  const users = useUsers()
  const todayStr = today()
  const day = useToday(todayStr)
  const week = useCalendar(todayStr, addDays(todayStr, 7))
  const [settings, setSettings] = useState(false)

  const keys = [['today'], ['routines'], ['board'], ['boards'], ['calendar']]
  const toggleRoutine = useInvalidating(({ id, on }: { id: number; on: boolean }) =>
    on ? api.put(`/api/routines/${id}/checks/${todayStr}`) : api.del(`/api/routines/${id}/checks/${todayStr}`), keys)
  const completeCard = useInvalidating(({ id, doneCol }: { id: number; doneCol: number }) =>
    api.patch(`/api/cards/${id}`, { column_id: doneCol, position: 0 }), keys)

  const routines = day.data?.routines ?? []
  const cards = day.data?.cards ?? []
  const done = routines.filter((r) => r.checked_by).length
  const total = routines.length + cards.length
  const userName = (id: number | null | undefined) => users.data?.find((u) => u.id === id)?.name
  const multiBoard = new Set(cards.map((c) => c.board_id)).size > 1

  return (
    <div className="mx-auto max-w-lg">
      <PageHeader
        title={<span>{greeting()}, {me.data?.name}</span>}
        right={
          <button onClick={() => setSettings(true)} aria-label="설정" className="rounded-lg p-1 text-slate-500 active:bg-slate-200">
            <svg viewBox="0 0 24 24" className="h-6 w-6" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round"><circle cx="12" cy="12" r="3" /><path d="M19.4 15a1.7 1.7 0 0 0 .3 1.8l.1.1a2 2 0 1 1-2.8 2.8l-.1-.1a1.7 1.7 0 0 0-1.8-.3 1.7 1.7 0 0 0-1 1.5V21a2 2 0 1 1-4 0v-.1a1.7 1.7 0 0 0-1.1-1.5 1.7 1.7 0 0 0-1.8.3l-.1.1a2 2 0 1 1-2.8-2.8l.1-.1a1.7 1.7 0 0 0 .3-1.8 1.7 1.7 0 0 0-1.5-1H3a2 2 0 1 1 0-4h.1a1.7 1.7 0 0 0 1.5-1.1 1.7 1.7 0 0 0-.3-1.8l-.1-.1a2 2 0 1 1 2.8-2.8l.1.1a1.7 1.7 0 0 0 1.8.3H9a1.7 1.7 0 0 0 1-1.5V3a2 2 0 1 1 4 0v.1a1.7 1.7 0 0 0 1 1.5 1.7 1.7 0 0 0 1.8-.3l.1-.1a2 2 0 1 1 2.8 2.8l-.1.1a1.7 1.7 0 0 0-.3 1.8V9a1.7 1.7 0 0 0 1.5 1H21a2 2 0 1 1 0 4h-.1a1.7 1.7 0 0 0-1.5 1z" /></svg>
          </button>
        }
      />

      <div className="space-y-6 px-4 py-4">
        {/* 오늘 할 일: 루틴 + 보드의 '할 일' 컬럼 카드를 한 목록으로 */}
        <section>
          <div className="mb-2 flex items-baseline justify-between">
            <h2 className="text-base font-bold">오늘 할 일</h2>
            <span className="text-xs text-slate-400">{fmtDate(todayStr)} · {done}/{total}</span>
          </div>
          {total > 0 && (
            <div className="mb-2 h-1.5 overflow-hidden rounded-full bg-slate-200">
              <div className="h-full rounded-full bg-emerald-500 transition-all" style={{ width: `${(done / total) * 100}%` }} />
            </div>
          )}
          <ul className="space-y-1.5">
            {routines.map((r) => {
              const on = !!r.checked_by
              return (
                <li key={`r${r.id}`}>
                  <TodoRow
                    checked={on}
                    onToggle={() => toggleRoutine.mutate({ id: r.id, on: !on })}
                    title={r.title}
                    meta={r.time_of_day ?? undefined}
                    tag="루틴"
                    avatar={on ? userName(r.checked_by) : userName(r.assignee_id)}
                  />
                </li>
              )
            })}
            {cards.map((c) => (
              <li key={`c${c.id}`}>
                <TodoRow
                  checked={false}
                  onToggle={() => completeCard.mutate({ id: c.id, doneCol: c.done_column_id })}
                  title={c.title}
                  meta={c.due_date ? dueLabel(c.due_date) : undefined}
                  metaTone={c.due_date && c.due_date < todayStr ? 'danger' : c.due_date === todayStr ? 'warn' : undefined}
                  tag={multiBoard ? c.board_name : undefined}
                  avatar={userName(c.assignee_id)}
                  href={`/boards/${c.board_id}`}
                />
              </li>
            ))}
            {total === 0 && (
              <li className="rounded-xl bg-white p-4 text-center text-sm text-slate-400">
                오늘 할 일이 없어요 · <Link to="/boards" className="underline">보드</Link> · <Link to="/routines" className="underline">루틴</Link>
              </li>
            )}
          </ul>
        </section>

        <section>
          <div className="mb-2 flex items-baseline justify-between">
            <h2 className="text-base font-bold">앞으로 7일</h2>
            <Link to="/calendar" className="text-xs text-slate-400">캘린더 ›</Link>
          </div>
          <WeekStrip data={week.data} from={todayStr} />
        </section>
      </div>

      <SettingsSheet open={settings} onClose={() => setSettings(false)} />
    </div>
  )
}

function TodoRow({ checked, onToggle, title, meta, metaTone, tag, avatar, href }: {
  checked: boolean; onToggle: () => void; title: string; meta?: string; metaTone?: 'danger' | 'warn'; tag?: string; avatar?: string; href?: string
}) {
  const metaCls = metaTone === 'danger' ? 'text-rose-500' : metaTone === 'warn' ? 'text-amber-600' : 'text-slate-400'
  const body = (
    <>
      <span className={`flex-1 truncate text-sm font-medium ${checked ? 'text-slate-400 line-through' : ''}`}>{title}</span>
      {tag && <span className="shrink-0 rounded-md bg-slate-100 px-1.5 py-0.5 text-[10px] text-slate-500">{tag}</span>}
      {meta && <span className={`shrink-0 text-xs ${metaCls}`}>{meta}</span>}
      {avatar && <Avatar name={avatar} />}
    </>
  )
  return (
    <div className={`flex items-center gap-3 rounded-xl p-3 shadow-sm transition ${checked ? 'bg-emerald-50' : 'bg-white'}`}>
      <button
        onClick={onToggle}
        aria-label={checked ? '완료 취소' : '완료'}
        className={`flex h-6 w-6 shrink-0 items-center justify-center rounded-full border-2 ${checked ? 'border-emerald-500 bg-emerald-500' : 'border-slate-300 active:border-emerald-400'}`}
      >
        {checked && <svg viewBox="0 0 24 24" className="h-4 w-4 text-white" fill="none" stroke="currentColor" strokeWidth="3" strokeLinecap="round" strokeLinejoin="round"><path d="M5 13l4 4L19 7" /></svg>}
      </button>
      {href ? <Link to={href} className="flex min-w-0 flex-1 items-center gap-2">{body}</Link> : <div className="flex min-w-0 flex-1 items-center gap-2">{body}</div>}
    </div>
  )
}

function WeekStrip({ data, from }: { data?: { events: { id: number; title: string; start_at: string; end_at: string | null; all_day: boolean }[]; due_cards: { id: number; title: string; due_date: string | null }[] }; from: string }) {
  const days = Array.from({ length: 7 }, (_, i) => addDays(from, i))
  const items = days.map((d) => ({
    d,
    events: data?.events.filter((e) => e.start_at.slice(0, 10) <= d && (e.end_at ?? e.start_at).slice(0, 10) >= d) ?? [],
    due: data?.due_cards.filter((c) => c.due_date === d) ?? [],
  })).filter((x) => x.events.length || x.due.length)

  if (items.length === 0) return <p className="rounded-xl bg-white p-4 text-center text-sm text-slate-400">이번 주 일정이 없어요</p>
  return (
    <ul className="space-y-2">
      {items.map(({ d, events, due }) => (
        <li key={d} className="rounded-xl bg-white p-3 shadow-sm">
          <p className="mb-1 text-xs font-semibold text-slate-500">
            {d === from ? '오늘' : d === addDays(from, 1) ? '내일' : `${WEEKDAYS[isoWeekday(d)]} ${Number(d.slice(8))}일`}
          </p>
          {events.map((e) => (
            <p key={e.id} className="flex items-center gap-2 text-sm"><i className="h-1.5 w-1.5 rounded-full bg-sky-500" />{e.title}{!e.all_day && <span className="text-xs text-slate-400">{fmtTime(e.start_at)}</span>}</p>
          ))}
          {due.map((c) => (
            <p key={c.id} className="flex items-center gap-2 text-sm"><i className="h-1.5 w-1.5 rounded-full bg-amber-500" />{c.title}<span className="text-xs text-slate-400">마감</span></p>
          ))}
        </li>
      ))}
    </ul>
  )
}

function SettingsSheet({ open, onClose }: { open: boolean; onClose: () => void }) {
  const nav = useNavigate()
  const qc = useQueryClient()
  const users = useUsers()
  const [adding, setAdding] = useState(false)
  const [name, setName] = useState('')
  const [pw, setPw] = useState('')
  const [err, setErr] = useState<string | null>(null)
  const addUser = useInvalidating((b: { name: string; password: string }) => api.post('/api/users', b), [['users']])

  return (
    <Sheet open={open} onClose={onClose} title="설정">
      <div className="space-y-4">
        <div>
          <p className="mb-2 text-xs font-medium text-slate-500">가족</p>
          <ul className="flex flex-wrap gap-2">
            {users.data?.map((u) => <li key={u.id} className="flex items-center gap-1.5 rounded-full bg-slate-100 py-1 pl-1 pr-3 text-sm"><Avatar name={u.name} />{u.name}</li>)}
            <li><button onClick={() => setAdding(true)} className="rounded-full border border-dashed border-slate-300 px-3 py-1 text-sm text-slate-500">+ 추가</button></li>
          </ul>
        </div>
        {adding && (
          <form
            onSubmit={async (e) => {
              e.preventDefault(); setErr(null)
              try { await addUser.mutateAsync({ name: name.trim(), password: pw }); setName(''); setPw(''); setAdding(false) }
              catch (ex) { setErr(ex instanceof Error ? ex.message : '실패') }
            }}
            className="space-y-2 rounded-xl bg-slate-50 p-3"
          >
            <Field label="이름"><Input autoFocus value={name} onChange={(e) => setName(e.target.value)} /></Field>
            <Field label="비밀번호 (4자 이상)"><Input type="password" value={pw} onChange={(e) => setPw(e.target.value)} /></Field>
            {err && <p className="text-xs text-rose-500">{err}</p>}
            <div className="flex justify-end gap-2"><Button variant="ghost" type="button" onClick={() => setAdding(false)}>취소</Button><Button type="submit" disabled={!name.trim() || pw.length < 4}>추가</Button></div>
          </form>
        )}
        <Button variant="ghost" className="w-full" onClick={async () => { await api.post('/api/logout'); qc.clear(); nav('/login', { replace: true }) }}>로그아웃</Button>
      </div>
    </Sheet>
  )
}

function greeting() {
  const h = new Date().getHours()
  if (h < 6) return '늦은 밤이에요'
  if (h < 12) return '좋은 아침'
  if (h < 18) return '좋은 오후'
  return '좋은 저녁'
}
