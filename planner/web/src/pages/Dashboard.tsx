import { useEffect, useState } from 'react'
import { Link, useNavigate, useSearchParams } from 'react-router'
import { useQueryClient } from '@tanstack/react-query'
import { api, type BFRangeData, type SearchAll } from '../api'
import { useBabyfood, useCalendar, useDiary, useFamily, useInvalidating, useMe, useSearch, useToday, useUsers } from '../lib/hooks'
import { Plant } from '../components/Plant'
import { addDays, ageOf, ampm, dueLabel, fmtDate, fmtDue, fmtTime, hasTime, lifeDay, lifeDayOf, maskBit, today, weekdayIndex, WEEKDAYS } from '../lib/date'
import { Avatar, Button, Collapsible, Field, Input, PasswordInput, PageHeader, Sheet, SkeletonList } from '../components/ui'
import { useDiaryViewer } from '../components/DiaryViewer'
import SortToggle, { Stars } from '../components/SortToggle'
import StorageBar from '../components/StorageBar'
import ThemeToggle from '../components/ThemeToggle'
import type { CalendarData, SortMode } from '../lib/hooks'

export default function Dashboard() {
  const nav = useNavigate()
  const me = useMe()
  const users = useUsers()
  const todayStr = today()
  const [sort, setSortState] = useState<SortMode>(() => {
    try { return (localStorage.getItem('planner.homeSort') as SortMode) || 'time' } catch { return 'time' }
  })
  const setSort = (m: SortMode) => { setSortState(m); try { localStorage.setItem('planner.homeSort', m) } catch { /* ignore */ } }
  const day = useToday(todayStr, sort)
  // 내일부터 — 오늘까지는 '오늘 할 일'이 맡는다(겹치면 같은 카드가 두 번 나온다).
  const week = useCalendar(addDays(todayStr, 1), addDays(todayStr, 8))
  const bf = useBabyfood(todayStr, todayStr)
  // 검색은 헤더에서 펼친다.
  const [searching, setSearching] = useState(false)
  const [q, setQ] = useState('')
  const [debouncedQ, setDebouncedQ] = useState('')
  useEffect(() => {
    const t = setTimeout(() => setDebouncedQ(q), 200)
    return () => clearTimeout(t)
  }, [q])
  const results = useSearch(debouncedQ)
  const closeSearch = () => { setSearching(false); setQ(''); setDebouncedQ('') }

  // 식단 탭이 생일 입력을 요구할 때 ?settings=1 로 온다.
  const [params, setParams] = useSearchParams()
  const [settings, setSettings] = useState(() => params.get('settings') != null)
  const closeSettings = () => {
    setSettings(false)
    if (params.get('settings') != null) setParams({}, { replace: true })
  }

  const keys = [['today'], ['routines'], ['board'], ['boards'], ['calendar']]
  const toggleRoutine = useInvalidating(({ id, on }: { id: number; on: boolean }) =>
    on ? api.put(`/api/routines/${id}/checks/${todayStr}`) : api.del(`/api/routines/${id}/checks/${todayStr}`), keys)
  const completeCard = useInvalidating(({ id, doneCol }: { id: number; doneCol: number }) =>
    api.patch(`/api/cards/${id}`, { column_id: doneCol, position: 0 }), keys)

  const routines = day.data?.routines ?? []
  const cards = day.data?.cards ?? []
  const done = routines.filter((r) => r.checked_by).length
  const total = routines.length + cards.length

  // 루틴 먼저, 남는 자리에 카드 — 최대 5줄.
  const MAX_ROWS = 5
  const shownRoutines = routines.slice(0, MAX_ROWS)
  const shownCards = cards.slice(0, Math.max(0, MAX_ROWS - shownRoutines.length))
  const hidden = total - shownRoutines.length - shownCards.length
  const userName = (id: number | null | undefined) => users.data?.find((u) => u.id === id)?.name
  const multiBoard = new Set(cards.map((c) => c.board_id)).size > 1

  return (
    <div className="mx-auto max-w-lg">
      <PageHeader
        title={
          searching ? (
            <input
              autoFocus
              type="search"
              value={q}
              onChange={(e) => setQ(e.target.value)}
              onKeyDown={(e) => { if (e.key === 'Escape') closeSearch() }}
              placeholder="제목과 본문에서 찾아요"
              className="w-full rounded-xl border border-line bg-surface px-3 py-1.5 text-base outline-none focus:border-ghost"
            />
          ) : (
            <span>{greeting()}, {me.data?.name}</span>
          )
        }
        right={
          searching ? (
            <button onClick={closeSearch} aria-label="검색 닫기" className="rounded-lg p-1 text-muted active:bg-line">
              <svg viewBox="0 0 24 24" className="h-6 w-6" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round"><path d="M18 6L6 18M6 6l12 12" /></svg>
            </button>
          ) : (
          <div className="flex items-center gap-1">
          <button onClick={() => nav('/notify')} aria-label="알림" className="rounded-lg p-1 text-muted active:bg-line">
            <svg viewBox="0 0 24 24" className="h-6 w-6" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round"><path d="M6 8a6 6 0 1 1 12 0c0 7 3 9 3 9H3s3-2 3-9" /><path d="M10.3 21a1.9 1.9 0 0 0 3.4 0" /></svg>
          </button>
          <button onClick={() => setSearching(true)} aria-label="검색" className="rounded-lg p-1 text-muted active:bg-line">
            <svg viewBox="0 0 24 24" className="h-6 w-6" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round"><circle cx="11" cy="11" r="7" /><path d="M20 20l-3.5-3.5" /></svg>
          </button>
          <button onClick={() => setSettings(true)} aria-label="설정" className="rounded-lg p-1 text-muted active:bg-line">
            <svg viewBox="0 0 24 24" className="h-6 w-6" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round"><circle cx="12" cy="12" r="3" /><path d="M19.4 15a1.7 1.7 0 0 0 .3 1.8l.1.1a2 2 0 1 1-2.8 2.8l-.1-.1a1.7 1.7 0 0 0-1.8-.3 1.7 1.7 0 0 0-1 1.5V21a2 2 0 1 1-4 0v-.1a1.7 1.7 0 0 0-1.1-1.5 1.7 1.7 0 0 0-1.8.3l-.1.1a2 2 0 1 1-2.8-2.8l.1-.1a1.7 1.7 0 0 0 .3-1.8 1.7 1.7 0 0 0-1.5-1H3a2 2 0 1 1 0-4h.1a1.7 1.7 0 0 0 1.5-1.1 1.7 1.7 0 0 0-.3-1.8l-.1-.1a2 2 0 1 1 2.8-2.8l.1.1a1.7 1.7 0 0 0 1.8.3H9a1.7 1.7 0 0 0 1-1.5V3a2 2 0 1 1 4 0v.1a1.7 1.7 0 0 0 1 1.5 1.7 1.7 0 0 0 1.8-.3l.1-.1a2 2 0 1 1 2.8 2.8l-.1.1a1.7 1.7 0 0 0-.3 1.8V9a1.7 1.7 0 0 0 1.5 1H21a2 2 0 1 1 0 4h-.1a1.7 1.7 0 0 0-1.5 1z" /></svg>
          </button>
          </div>
          )
        }
        dropdown={
          searching && debouncedQ.trim() !== '' ? (
            <SearchPanel q={debouncedQ} data={results.data} pending={results.isPending} onGo={(to) => { closeSearch(); nav(to) }} />
          ) : undefined
        }
      />

      <div className="space-y-6 px-4 py-4">
        <FamilyCard />
        <section>
          <div className="mb-2 flex items-center justify-between gap-2">
            <h2 className="text-base font-bold">오늘 할 일</h2>
            <div className="flex items-center gap-2">
              {!day.isPending && <span className="text-xs text-faint">{done}/{total}</span>}
              <SortToggle
                value={sort}
                onChange={setSort}
                modes={[{ key: 'time', label: '시간' }, { key: 'priority', label: '중요도' }]}
              />
            </div>
          </div>
          <p className="mb-2 text-xs text-faint">{fmtDate(todayStr)}</p>
          {total > 0 && (
            <div className="mb-2 h-1.5 overflow-hidden rounded-full bg-line">
              <div className="h-full rounded-full bg-emerald-500 transition-all" style={{ width: `${(done / total) * 100}%` }} />
            </div>
          )}
          {day.isPending && <SkeletonList rows={3} />}
          <ul className="divide-y divide-line overflow-hidden rounded-2xl bg-surface shadow-sm empty:hidden">
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
                  className="flex items-center justify-center gap-1.5 py-3 text-sm font-medium text-muted active:bg-canvas"
                >
                  <svg viewBox="0 0 24 24" className="h-4 w-4" fill="none" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round"><path d="M12 5v14M5 12h14" /></svg>
                  {hidden}건 더 보기
                </Link>
              </li>
            )}
            {!day.isPending && total === 0 && (
              <li className="p-4 text-center text-sm text-faint">
                오늘 할 일이 없어요 · <Link to="/boards" className="underline">할 일</Link> · <Link to="/routines" className="underline">루틴</Link>
              </li>
            )}
          </ul>
        </section>

        <Birthdays users={users.data} today={todayStr} />

        <TodayMeals data={bf.data} />

        <TodayDiary />

        <section>
          <div className="mb-2 flex items-baseline justify-between">
            <h2 className="text-base font-bold">앞으로 7일</h2>
            <Link to="/calendar" className="text-xs text-faint">캘린더 ›</Link>
          </div>
          {week.isPending ? <SkeletonList rows={2} /> : <WeekStrip data={week.data} from={addDays(todayStr, 1)} />}
        </section>
      </div>

      <SettingsSheet open={settings} onClose={closeSettings} />
    </div>
  )
}

function TodoRow({ checked, onToggle, title, meta, metaTone, tag, avatar, href, priority = 0 }: {
  checked: boolean; onToggle: () => void; title: string; meta?: string; metaTone?: 'danger' | 'warn'; tag?: string; avatar?: string; href?: string; priority?: number
}) {
  const metaCls = metaTone === 'danger' ? 'text-rose-500' : metaTone === 'warn' ? 'text-amber-600' : 'text-faint'
  const body = (
    <>
      <span className={`flex-1 truncate text-sm font-medium ${checked ? 'text-faint line-through' : ''}`}>{title}</span>
      <Stars n={priority} />
      {tag && <span className="shrink-0 rounded-md bg-surface-2 px-1.5 py-0.5 text-[10px] text-muted">{tag}</span>}
      {meta && <span className={`shrink-0 text-xs ${metaCls}`}>{meta}</span>}
      {avatar && <Avatar name={avatar} />}
    </>
  )
  return (
    <div className={`flex items-center gap-3 px-3.5 py-3 transition ${checked ? 'bg-emerald-50' : ''}`}>
      {/* 체크는 행 이동과 겹치므로 전파를 끊는다 */}
      <button
        onClick={(e) => { e.preventDefault(); e.stopPropagation(); onToggle() }}
        aria-label={checked ? '완료 취소' : '완료'}
        className={`flex h-6 w-6 shrink-0 items-center justify-center rounded-full border-2 ${checked ? 'border-emerald-500 bg-emerald-500' : 'border-line active:border-emerald-400'}`}
      >
        {checked && <svg viewBox="0 0 24 24" className="h-4 w-4 text-white" fill="none" stroke="currentColor" strokeWidth="3" strokeLinecap="round" strokeLinejoin="round"><path d="M5 13l4 4L19 7" /></svg>}
      </button>
      {href ? (
        <Link to={href} className="flex min-w-0 flex-1 items-center gap-2 active:opacity-70">
          {body}
          <svg viewBox="0 0 24 24" className="h-4 w-4 shrink-0 text-ghost" fill="none" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round"><path d="M9 6l6 6-6 6" /></svg>
        </Link>
      ) : (
        <div className="flex min-w-0 flex-1 items-center gap-2">{body}</div>
      )}
    </div>
  )
}

/** 오늘 일기. 없으면 쓰기 버튼 한 줄. */
function TodayDiary() {
  const nav = useNavigate()
  const viewer = useDiaryViewer()
  const day = today()
  const list = useDiary(day, day)
  const entries = list.data ?? []

  return (
    <section>
      <div className="mb-2 flex items-baseline justify-between">
        <h2 className="text-base font-bold">오늘 일기</h2>
        <Link to="/diary" className="text-xs text-faint">다이어리 ›</Link>
      </div>
      {entries.length === 0 ? (
        <button
          onClick={() => nav('/diary/new')}
          className="w-full rounded-2xl border border-dashed border-line py-3 text-sm text-muted active:bg-surface-2"
        >
          오늘 있었던 일 남기기
        </button>
      ) : (
        <ul className="space-y-2">
          {entries.map((e) => (
            <li key={e.id}>
              <button
                onClick={() => viewer.open(e.id)}
                className="w-full rounded-2xl bg-surface p-3.5 text-left shadow-sm active:bg-canvas"
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
    </section>
  )
}

function Birthdays({ users, today }: { users?: { id: number; name: string; birth_date: string | null }[]; today: string }) {
  const md = today.slice(5)
  const born = (users ?? []).filter((u) => u.birth_date && u.birth_date.slice(5) === md)
  if (born.length === 0) return null
  return (
    <section>
      <div className="rounded-2xl bg-amber-50 p-3.5">
        <p className="text-sm font-semibold text-amber-900">
          🎂 오늘은 {born.map((u) => u.name).join(', ')}의 생일이에요
        </p>
      </div>
    </section>
  )
}

// 오늘 이유식. 대상이나 식단이 없으면 아무것도 안 그린다.
function TodayMeals({ data }: { data?: BFRangeData }) {
  const day = data?.days?.[0]
  if (!day) return null
  const many = (data?.children?.length ?? 0) > 1
  return (
    <section>
      <div className="mb-2 flex items-baseline justify-between">
        <h2 className="text-base font-bold">오늘 이유식{many && data?.child ? ` · ${data.child.name}` : ''}</h2>
        <Link to="/babyfood" className="text-xs text-faint">이유식 ›</Link>
      </div>
      <Link to="/babyfood" className="block rounded-2xl bg-surface p-3.5 shadow-sm active:bg-canvas">
        <p className="mb-1.5 text-[11px] text-faint">{lifeDay(day.dday)} · {day.label}</p>
        <ul className="space-y-1">
          {day.meals.map((m) => (
            <li key={m.id} className="flex gap-2 text-sm">
              <span className="w-[4.5rem] shrink-0 text-faint">
                {m.title}
                {m.at && <span className="ml-1 text-[11px]">{m.at}</span>}
              </span>
              <span className={`min-w-0 flex-1 truncate ${m.eaten ? 'text-emerald-700' : ''}`}>
                {m.eaten && '✓ '}{m.base}
                {m.toppings.length > 0 && <span className="text-muted"> · {m.toppings.join(' ')}</span>}
              </span>
            </li>
          ))}
        </ul>
        {day.new_item && (
          <p className="mt-1.5 text-[11px] font-medium text-amber-700">처음 먹는 재료 · {day.new_item}</p>
        )}
      </Link>
    </section>
  )
}

/** 검색 결과. 헤더에 매달려 본문을 덮는다(흐름에 넣으면 화면이 출렁인다). */
function SearchPanel({ q, data, pending, onGo }: {
  q: string
  data?: SearchAll
  pending: boolean
  onGo: (to: string) => void
}) {
  const cards = data?.cards ?? []
  const routines = data?.routines ?? []
  const foods = data?.foods ?? []
  const diary = data?.diary ?? []
  const total = cards.length + routines.length + foods.length + diary.length

  const row = (key: string, to: string, title: string, meta: string, tail?: React.ReactNode) => (
    <li key={key}>
      <button
        onClick={() => onGo(to)}
        className="flex w-full items-center gap-2 rounded-lg bg-surface px-2.5 py-2 text-left shadow-sm active:bg-surface-2"
      >
        <div className="min-w-0 flex-1">
          <p className="truncate text-sm font-medium">{title}</p>
          <p className="truncate text-[11px] text-faint">{meta}</p>
        </div>
        {tail}
      </button>
    </li>
  )

  return (
    <div className="px-4 pt-3">
      <div className="max-h-80 overflow-y-auto rounded-xl border border-line bg-surface p-2 shadow-lg">
        {pending && <SkeletonList rows={2} />}
        {!pending && total === 0 && (
          <p className="py-4 text-center text-xs text-faint">“{q}” 에 맞는 게 없어요</p>
        )}

        {cards.length > 0 && <Group label="할 일" />}
        <ul className="space-y-1.5">
          {cards.map((h) =>
            row(`c${h.id}`, `/cards/${h.id}`, h.title,
              [h.column_name, h.snippet, h.due_at ? dueLabel(h.due_at.slice(0, 10)) : ''].filter(Boolean).join(' · '),
              h.priority > 0 ? <Stars n={h.priority} /> : undefined))}
        </ul>

        {routines.length > 0 && <Group label="루틴" />}
        <ul className="space-y-1.5">
          {routines.map((r) =>
            row(`r${r.id}`, `/routines?edit=${r.id}`, r.title,
              [weekdayText(r.weekdays_mask), r.time_of_day ?? '', r.active ? '' : '멈춤'].filter(Boolean).join(' · ')))}
        </ul>

        {diary.length > 0 && <Group label="다이어리" />}
        <ul className="space-y-1.5">
          {diary.map((e) =>
            row(`d${e.id}`, `/diary/${e.id}`, e.title || '제목 없음',
              [fmtDate(e.date), e.plain].filter(Boolean).join(' · ')))}
        </ul>

        {foods.length > 0 && <Group label="이유식 재료" />}
        <ul className="space-y-1.5">
          {foods.map((f) =>
            row(`f${f.name}`, `/babyfood/foods?q=${encodeURIComponent(f.name)}`, f.name,
              [f.reaction ? '알레르기 반응' : '', f.liked ? '잘 먹어요' : '',
               f.first_date ? `${f.first_date.slice(5).replace('-', '/')}부터` : '',
               f.uses > 0 ? `${f.uses}번` : ''].filter(Boolean).join(' · ') || '표시 없음'))}
        </ul>
      </div>
    </div>
  )
}

function Group({ label }: { label: string }) {
  return <p className="px-1 pb-1 pt-2 text-[10px] font-medium text-faint first:pt-0">{label}</p>
}

/** 요일 마스크 → '월·수'. 마스크는 월=bit0. */
function weekdayText(mask: number) {
  if (mask === 0x7F) return '매일'
  const on = WEEKDAYS.filter((_, i) => (mask & (1 << maskBit(i))) !== 0)
  return on.length ? on.join('·') : ''
}

function WeekStrip({ data, from }: { data?: CalendarData; from: string }) {
  // 라벨은 진짜 오늘 기준(from 은 내일부터).
  const t = today()
  const days = Array.from({ length: 7 }, (_, i) => addDays(from, i))
  const items = days.map((d) => ({
    d,
    cards: data?.cards.filter((c) => {
      if (!c.due_at) return false
      return c.due_at.slice(0, 10) <= d && (c.end_at ?? c.due_at).slice(0, 10) >= d
    }) ?? [],
  })).filter((x) => x.cards.length)

  if (items.length === 0) return <p className="rounded-2xl bg-surface p-4 text-center text-sm text-faint">이번 주 일정이 없어요</p>
  return (
    <ul className="divide-y divide-line overflow-hidden rounded-2xl bg-surface shadow-sm">
      {items.map(({ d, cards }) => (
        <li key={d}>
          <Link to={`/calendar?date=${d}`} className="block px-3.5 py-3 active:bg-canvas">
          <p className="mb-1 flex items-center gap-1 text-xs font-semibold text-muted">
            {d === addDays(t, 1) ? '내일' : `${WEEKDAYS[weekdayIndex(d)]} ${Number(d.slice(8))}일`}
            <svg viewBox="0 0 24 24" className="h-3 w-3 text-ghost" fill="none" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round"><path d="M9 6l6 6-6 6" /></svg>
          </p>
          {cards.map((c) => (
            <p key={c.id} className="flex items-center gap-2 text-sm">
              <i className={`h-1.5 w-1.5 shrink-0 rounded-full ${hasTime(c.due_at!) ? 'bg-sky-500' : 'bg-amber-500'}`} />
              <span className="truncate">{c.title}</span>
              {hasTime(c.due_at!) && <span className="shrink-0 text-xs text-faint">{ampm(c.due_at!)} {fmtTime(c.due_at!)}</span>}
            </p>
          ))}
          </Link>
        </li>
      ))}
    </ul>
  )
}

/** 가족 한 사람. 눌러 펼치면 생년월일·비밀번호. */
function MemberRow({ user, isMe, open, onToggle, onBirth, onPassword }: {
  user: { id: number; name: string; birth_date: string | null }
  isMe: boolean
  open: boolean
  onToggle: () => void
  onBirth: (date: string) => void
  onPassword: () => void
}) {
  const birth = user.birth_date
  return (
    <li className="rounded-xl bg-canvas">
      <button onClick={onToggle} aria-expanded={open} className="flex w-full items-center gap-2 p-2 text-left">
        <Avatar name={user.name} />
        <span className="min-w-0 flex-1">
          <span className="block text-sm font-medium">
            {user.name}
            {isMe && <span className="ml-1 text-xs text-faint">(나)</span>}
          </span>
          <span className="block text-[11px] text-faint">
            {birth ? `${fmtDate(birth)} · ${lifeDayOf(birth)} · 만 ${ageOf(birth)}세` : '생년월일 없음'}
          </span>
        </span>
        <svg
          viewBox="0 0 24 24"
          className={`h-4 w-4 shrink-0 text-ghost transition-transform ${open ? 'rotate-90' : ''}`}
          fill="none" stroke="currentColor" strokeWidth="2.4" strokeLinecap="round" strokeLinejoin="round"
        >
          <path d="M9 6l6 6-6 6" />
        </svg>
      </button>

      <div className="px-2 pb-2" hidden={!open}>
        <div className="flex items-center gap-2 pl-1">
          <span className="text-[11px] text-faint">생년월일</span>
          <input
            type="date"
            defaultValue={birth ?? ''}
            onChange={(e) => { if (e.target.value) onBirth(e.target.value) }}
            className="rounded-lg border border-line bg-surface px-2 py-1 text-xs"
          />
        </div>
        <button
          onClick={onPassword}
          className="mt-2 w-full rounded-lg bg-surface py-2 text-xs font-medium text-muted active:bg-line"
        >
          {isMe ? '비밀번호 변경' : '비밀번호 재설정'}
        </button>
      </div>
    </li>
  )
}

function SettingsSheet({ open, onClose }: { open: boolean; onClose: () => void }) {
  const nav = useNavigate()
  const qc = useQueryClient()
  const me = useMe()
  const users = useUsers()
  const [adding, setAdding] = useState(false)
  const [openMember, setOpenMember] = useState<number | null>(null)
  const [resetting, setResetting] = useState<{ id: number; name: string } | null>(null)
  const [name, setName] = useState('')
  const [pw, setPw] = useState('')
  const [err, setErr] = useState<string | null>(null)
  const addUser = useInvalidating((b: { name: string; password: string }) => api.post('/api/users', b), [['users']])
  const setBirth = useInvalidating(
    ({ id, ...b }: { id: number; birth_date: string }) => api.patch(`/api/users/${id}/birthdate`, b),
    [['users'], ['babyfood']],
  )

  return (
    <>
      <Sheet open={open && !resetting} onClose={onClose} title="설정">
        <div className="space-y-4">
          <Collapsible title="가족" defaultOpen>
            <ul className="space-y-1">
              {users.data?.map((u) => (
                <MemberRow
                  key={u.id}
                  user={u}
                  isMe={u.id === me.data?.id}
                  open={openMember === u.id}
                  onToggle={() => setOpenMember(openMember === u.id ? null : u.id)}
                  onBirth={(birth_date) => setBirth.mutate({ id: u.id, birth_date })}
                  onPassword={() => { setErr(null); setResetting({ id: u.id, name: u.name }) }}
                />
              ))}
            </ul>
            {!adding && (
              <button onClick={() => { setErr(null); setAdding(true) }} className="mt-2 w-full rounded-xl border border-dashed border-line py-2 text-sm text-muted">
                + 가족 추가
              </button>
            )}

            {adding && (
            <form
              onSubmit={async (e) => {
                e.preventDefault(); setErr(null)
                try { await addUser.mutateAsync({ name: name.trim(), password: pw }); setName(''); setPw(''); setAdding(false) }
                catch (ex) { setErr(ex instanceof Error ? ex.message : '실패') }
              }}
              className="space-y-2 rounded-xl bg-canvas p-3"
            >
              <Field label="이름"><Input autoFocus value={name} onChange={(e) => setName(e.target.value)} /></Field>
              <Field label="비밀번호 (8자 이상)"><PasswordInput autoComplete="new-password" value={pw} onChange={(e) => setPw(e.target.value)} /></Field>
              {err && <p className="text-xs text-rose-500">{err}</p>}
              <div className="flex justify-end gap-2">
                <Button variant="ghost" type="button" onClick={() => setAdding(false)}>취소</Button>
                <Button type="submit" disabled={!name.trim() || pw.length < 8}>추가</Button>
              </div>
            </form>
            )}
          </Collapsible>

          <Collapsible title="테마" defaultOpen>
            <ThemeToggle />
          </Collapsible>

          <Collapsible title="저장 공간">
            <StorageBar />
          </Collapsible>

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

// 비밀번호 재설정. 남의 것을 바꾸려면 본인 비밀번호가 필요하다(탈취된 세션만으론 못 뺏는다).
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
          <PasswordInput autoComplete="current-password" autoFocus value={current} onChange={(e) => setCurrent(e.target.value)} />
        </Field>
        <Field label={isSelf ? '새 비밀번호 (8자 이상)' : `${target?.name}님의 새 비밀번호 (8자 이상)`}>
          <PasswordInput autoComplete="new-password" value={next} onChange={(e) => setNext(e.target.value)} />
        </Field>
        <Field label="새 비밀번호 확인">
          <PasswordInput autoComplete="new-password" value={again} onChange={(e) => setAgain(e.target.value)} />
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

const STREAK_NAME: Record<string, string> = { routines: '루틴 연속', diary: '일기 연속', care: '육아 연속 기록', budget: '생활비 페이스 연속' }

// 홈의 '함께' 카드: 식물, 이번 주 퀘스트, 새 마일스톤. 누르면 '함께' 화면.
function FamilyCard() {
  const q = useFamily()
  const b = q.data
  if (!b) return null
  const p = b.plant
  const done = b.quests.filter((x) => x.done).length
  const fresh = b.milestones.filter((m) => m.new)
  const best = [...b.streaks].sort((x, y) => y.current - x.current)[0]
  return (
    <Link to="/family" className="family-theme dark relative flex items-center gap-3 overflow-hidden rounded-3xl bg-canvas p-4 text-ink shadow-[0_12px_28px_-16px_rgba(16,28,52,0.9)]">
      <div className="min-w-0 flex-1">
        <p className="text-[11px] font-bold tracking-wide text-[#c9a64a]">우리 가족 · {p.name || '콩이'}</p>
        <p className="hero-title mt-1 text-xl font-black leading-tight">{p.ready ? '다 키웠어요!' : `콩이 ${p.stage}단계`}</p>
        <div className="mt-2 space-y-0.5 text-xs text-muted">
          <p>🎯 이번 주 퀘스트 <b className="text-ink">{done}/{b.quests.length}</b></p>
          {best && best.current > 0 && <p>🔥 {STREAK_NAME[best.key] ?? best.label} <b className="text-ink">{best.current}일</b></p>}
          {p.today > 0 && <p>💧 오늘 받은 물방울 <b className="text-ink">+{p.today}</b></p>}
          {p.credits > 0 && <p className="font-semibold text-[#e2b84a]">🎟️ 구매권 {p.credits}장 — 위시리스트에서 사요</p>}
          {fresh.length > 0 && <p className="truncate font-semibold text-[#e2b84a]">🎉 {fresh[0].label}{fresh.length > 1 && ` 외 ${fresh.length - 1}개`}</p>}
        </div>
      </div>
      <div className="-my-2 -mr-2 shrink-0"><Plant stage={p.stage} size={112} happy={p.ready || p.today > 0} /></div>
    </Link>
  )
}
