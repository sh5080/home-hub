import { useState } from 'react'
import { Link, useNavigate } from 'react-router'
import { useQueryClient } from '@tanstack/react-query'
import { api } from '../api'
import { useCalendar, useInvalidating, useMe, useToday, useUsers } from '../lib/hooks'
import { addDays, ampm, fmtDate, fmtDue, fmtTime, hasTime, today, weekdayIndex, WEEKDAYS } from '../lib/date'
import { Avatar, Button, Field, Input, PageHeader, Sheet } from '../components/ui'
import SortToggle, { Stars } from '../components/SortToggle'
import StorageBar from '../components/StorageBar'
import type { CalendarData, SortMode } from '../lib/hooks'

export default function Dashboard() {
  const me = useMe()
  const users = useUsers()
  const todayStr = today()
  const [sort, setSortState] = useState<SortMode>(() => {
    try { return (localStorage.getItem('planner.homeSort') as SortMode) || 'time' } catch { return 'time' }
  })
  const setSort = (m: SortMode) => { setSortState(m); try { localStorage.setItem('planner.homeSort', m) } catch { /* ignore */ } }
  const day = useToday(todayStr, sort)
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

  // 홈은 훑어보는 화면이다. 루틴을 먼저 두고 남는 자리에 카드를 채워
  // 최대 5줄만 보여준다. 나머지는 할 일 탭에서 본다.
  const MAX_ROWS = 5
  const shownRoutines = routines.slice(0, MAX_ROWS)
  const shownCards = cards.slice(0, Math.max(0, MAX_ROWS - shownRoutines.length))
  const hidden = total - shownRoutines.length - shownCards.length
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
          <div className="mb-2 flex items-center justify-between gap-2">
            <h2 className="text-base font-bold">오늘 할 일</h2>
            <div className="flex items-center gap-2">
              <span className="text-xs text-slate-400">{done}/{total}</span>
              <SortToggle
                value={sort}
                onChange={setSort}
                modes={[{ key: 'time', label: '시간' }, { key: 'priority', label: '중요도' }]}
              />
            </div>
          </div>
          <p className="mb-2 text-xs text-slate-400">{fmtDate(todayStr)}</p>
          {total > 0 && (
            <div className="mb-2 h-1.5 overflow-hidden rounded-full bg-slate-200">
              <div className="h-full rounded-full bg-emerald-500 transition-all" style={{ width: `${(done / total) * 100}%` }} />
            </div>
          )}
          <ul className="space-y-1.5">
            {shownRoutines.map((r) => {
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
                    href="/routines"
                  />
                </li>
              )
            })}
            {shownCards.map((c) => (
              <li key={`c${c.id}`}>
                <TodoRow
                  checked={false}
                  onToggle={() => completeCard.mutate({ id: c.id, doneCol: c.done_column_id })}
                  title={c.title}
                  meta={c.due_at ? fmtDue(c.due_at) : undefined}
                  metaTone={c.due_at && c.due_at < todayStr ? 'danger' : c.due_at === todayStr ? 'warn' : undefined}
                  tag={multiBoard ? c.board_name : undefined}
                  priority={c.priority}
                  avatar={userName(c.assignee_id)}
                  href={`/cards/${c.id}`}
                />
              </li>
            ))}
            {hidden > 0 && (
              <li>
                <Link
                  to="/boards"
                  className="flex items-center justify-center gap-1.5 rounded-xl border border-dashed border-slate-300 py-2.5 text-sm font-medium text-slate-500 active:bg-slate-100"
                >
                  <svg viewBox="0 0 24 24" className="h-4 w-4" fill="none" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round"><path d="M12 5v14M5 12h14" /></svg>
                  {hidden}건 더 보기
                </Link>
              </li>
            )}
            {total === 0 && (
              <li className="rounded-xl bg-white p-4 text-center text-sm text-slate-400">
                오늘 할 일이 없어요 · <Link to="/boards" className="underline">할 일</Link> · <Link to="/routines" className="underline">루틴</Link>
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

function TodoRow({ checked, onToggle, title, meta, metaTone, tag, avatar, href, priority = 0 }: {
  checked: boolean; onToggle: () => void; title: string; meta?: string; metaTone?: 'danger' | 'warn'; tag?: string; avatar?: string; href?: string; priority?: number
}) {
  const metaCls = metaTone === 'danger' ? 'text-rose-500' : metaTone === 'warn' ? 'text-amber-600' : 'text-slate-400'
  const body = (
    <>
      <span className={`flex-1 truncate text-sm font-medium ${checked ? 'text-slate-400 line-through' : ''}`}>{title}</span>
      <Stars n={priority} />
      {tag && <span className="shrink-0 rounded-md bg-slate-100 px-1.5 py-0.5 text-[10px] text-slate-500">{tag}</span>}
      {meta && <span className={`shrink-0 text-xs ${metaCls}`}>{meta}</span>}
      {avatar && <Avatar name={avatar} />}
    </>
  )
  return (
    <div className={`flex items-center gap-3 rounded-xl p-3 shadow-sm transition ${checked ? 'bg-emerald-50' : 'bg-white'}`}>
      {/* 체크는 행 이동과 겹치므로 전파를 끊는다 */}
      <button
        onClick={(e) => { e.preventDefault(); e.stopPropagation(); onToggle() }}
        aria-label={checked ? '완료 취소' : '완료'}
        className={`flex h-6 w-6 shrink-0 items-center justify-center rounded-full border-2 ${checked ? 'border-emerald-500 bg-emerald-500' : 'border-slate-300 active:border-emerald-400'}`}
      >
        {checked && <svg viewBox="0 0 24 24" className="h-4 w-4 text-white" fill="none" stroke="currentColor" strokeWidth="3" strokeLinecap="round" strokeLinejoin="round"><path d="M5 13l4 4L19 7" /></svg>}
      </button>
      {href ? (
        <Link to={href} className="flex min-w-0 flex-1 items-center gap-2 active:opacity-70">
          {body}
          <svg viewBox="0 0 24 24" className="h-4 w-4 shrink-0 text-slate-300" fill="none" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round"><path d="M9 6l6 6-6 6" /></svg>
        </Link>
      ) : (
        <div className="flex min-w-0 flex-1 items-center gap-2">{body}</div>
      )}
    </div>
  )
}

function WeekStrip({ data, from }: { data?: CalendarData; from: string }) {
  const days = Array.from({ length: 7 }, (_, i) => addDays(from, i))
  // 캘린더는 날짜가 있는 카드를 본 것이다 — 일정과 할 일이 한 목록이다.
  const items = days.map((d) => ({
    d,
    cards: data?.cards.filter((c) => {
      if (!c.due_at) return false
      return c.due_at.slice(0, 10) <= d && (c.end_at ?? c.due_at).slice(0, 10) >= d
    }) ?? [],
  })).filter((x) => x.cards.length)

  if (items.length === 0) return <p className="rounded-xl bg-white p-4 text-center text-sm text-slate-400">이번 주 일정이 없어요</p>
  return (
    <ul className="space-y-2">
      {items.map(({ d, cards }) => (
        <li key={d}>
          {/* 날짜 칸을 누르면 캘린더의 그 날로 간다 */}
          <Link to={`/calendar?date=${d}`} className="block rounded-xl bg-white p-3 shadow-sm active:bg-slate-50">
          <p className="mb-1 flex items-center gap-1 text-xs font-semibold text-slate-500">
            {d === from ? '오늘' : d === addDays(from, 1) ? '내일' : `${WEEKDAYS[weekdayIndex(d)]} ${Number(d.slice(8))}일`}
            <svg viewBox="0 0 24 24" className="h-3 w-3 text-slate-300" fill="none" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round"><path d="M9 6l6 6-6 6" /></svg>
          </p>
          {cards.map((c) => (
            <p key={c.id} className="flex items-center gap-2 text-sm">
              <i className={`h-1.5 w-1.5 shrink-0 rounded-full ${hasTime(c.due_at!) ? 'bg-sky-500' : 'bg-amber-500'}`} />
              <span className="truncate">{c.title}</span>
              {hasTime(c.due_at!) && <span className="shrink-0 text-xs text-slate-400">{ampm(c.due_at!)} {fmtTime(c.due_at!)}</span>}
            </p>
          ))}
          </Link>
        </li>
      ))}
    </ul>
  )
}

function SettingsSheet({ open, onClose }: { open: boolean; onClose: () => void }) {
  const nav = useNavigate()
  const qc = useQueryClient()
  const me = useMe()
  const users = useUsers()
  const [adding, setAdding] = useState(false)
  const [resetting, setResetting] = useState<{ id: number; name: string } | null>(null)
  const [name, setName] = useState('')
  const [pw, setPw] = useState('')
  const [err, setErr] = useState<string | null>(null)
  const addUser = useInvalidating((b: { name: string; password: string }) => api.post('/api/users', b), [['users']])

  return (
    <>
      <Sheet open={open && !resetting} onClose={onClose} title="설정">
        <div className="space-y-4">
          <div>
            <p className="mb-2 text-xs font-medium text-slate-500">가족</p>
            <ul className="space-y-1">
              {users.data?.map((u) => (
                <li key={u.id} className="flex items-center gap-2 rounded-xl bg-slate-50 py-2 pl-2 pr-1">
                  <Avatar name={u.name} />
                  <span className="flex-1 text-sm font-medium">
                    {u.name}
                    {u.id === me.data?.id && <span className="ml-1 text-xs text-slate-400">(나)</span>}
                  </span>
                  <button
                    onClick={() => { setErr(null); setResetting({ id: u.id, name: u.name }) }}
                    className="rounded-lg px-2 py-1 text-xs font-medium text-slate-500 active:bg-slate-200"
                  >
                    {u.id === me.data?.id ? '비밀번호 변경' : '비밀번호 재설정'}
                  </button>
                </li>
              ))}
            </ul>
            {!adding && (
              <button onClick={() => { setErr(null); setAdding(true) }} className="mt-2 w-full rounded-xl border border-dashed border-slate-300 py-2 text-sm text-slate-500">
                + 가족 추가
              </button>
            )}
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
              <Field label="비밀번호 (8자 이상)"><Input type="password" autoComplete="new-password" value={pw} onChange={(e) => setPw(e.target.value)} /></Field>
              {err && <p className="text-xs text-rose-500">{err}</p>}
              <div className="flex justify-end gap-2">
                <Button variant="ghost" type="button" onClick={() => setAdding(false)}>취소</Button>
                <Button type="submit" disabled={!name.trim() || pw.length < 8}>추가</Button>
              </div>
            </form>
          )}

          <StorageBar />

          <Button variant="ghost" className="w-full" onClick={async () => { await api.post('/api/logout'); qc.clear(); nav('/login', { replace: true }) }}>로그아웃</Button>
        </div>
      </Sheet>

      <PasswordSheet
        target={resetting}
        isSelf={resetting?.id === me.data?.id}
        actorName={me.data?.name ?? ''}
        onClose={() => setResetting(null)}
      />
    </>
  )
}

// 비밀번호 재설정. 이메일 없이 되찾는 경로라, 남의 비밀번호를 바꾸려면
// **본인 비밀번호**를 넣어야 한다 — 탈취된 세션만으로는 계정을 못 뺏는다.
function PasswordSheet({ target, isSelf, actorName, onClose }: {
  target: { id: number; name: string } | null
  isSelf: boolean
  actorName: string
  onClose: () => void
}) {
  const [current, setCurrent] = useState('')
  const [next, setNext] = useState('')
  const [again, setAgain] = useState('')
  const [err, setErr] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)

  const reset = () => { setCurrent(''); setNext(''); setAgain(''); setErr(null); setBusy(false) }
  const close = () => { reset(); onClose() }

  const mismatch = again.length > 0 && next !== again
  const ok = current.length > 0 && next.length >= 8 && next === again

  async function submit(e: React.FormEvent) {
    e.preventDefault()
    if (!target) return
    setBusy(true); setErr(null)
    try {
      await api.post(`/api/users/${target.id}/password`, { current_password: current, new_password: next })
      close()
    } catch (ex) {
      setErr(ex instanceof Error ? ex.message : '실패했어요')
      setBusy(false)
    }
  }

  return (
    <Sheet open={!!target} onClose={close} title={isSelf ? '내 비밀번호 변경' : `${target?.name} 비밀번호 재설정`}>
      <form onSubmit={submit} className="space-y-3">
        {!isSelf && (
          <p className="rounded-xl bg-amber-50 p-3 text-xs leading-relaxed text-amber-800">
            <b>{target?.name}</b>님의 비밀번호를 새로 정합니다. 지금 로그인된 {target?.name}님의 기기는 모두 로그아웃돼요.
          </p>
        )}
        <Field label={`${actorName}(나)의 현재 비밀번호`}>
          <Input type="password" autoComplete="current-password" autoFocus value={current} onChange={(e) => setCurrent(e.target.value)} />
        </Field>
        <Field label={isSelf ? '새 비밀번호 (8자 이상)' : `${target?.name}님의 새 비밀번호 (8자 이상)`}>
          <Input type="password" autoComplete="new-password" value={next} onChange={(e) => setNext(e.target.value)} />
        </Field>
        <Field label="새 비밀번호 확인">
          <Input type="password" autoComplete="new-password" value={again} onChange={(e) => setAgain(e.target.value)} />
        </Field>
        {mismatch && <p className="text-xs text-rose-500">새 비밀번호가 서로 달라요</p>}
        {err && <p className="text-xs text-rose-500">{err}</p>}
        <div className="flex justify-end gap-2 pt-1">
          <Button variant="ghost" type="button" onClick={close}>취소</Button>
          <Button type="submit" disabled={!ok || busy}>{busy ? '변경 중…' : '변경'}</Button>
        </div>
      </form>
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
