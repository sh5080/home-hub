import { useEffect, useMemo, useRef, useState } from 'react'
import { useNavigate, useSearchParams } from 'react-router'
import { api, type BFDay, type BFFood, type BFLog, type BFMeal } from '../api'
import { useBabyfood, useBFFoods, useInvalidating } from '../lib/hooks'
import { addDays, fmtDate, lifeDay, startOfWeek, today, WEEKDAYS, weekdayIndex } from '../lib/date'
import { Button, Field, Input, PageHeader, Sheet, SkeletonList, TimeField } from '../components/ui'
import Byline from '../components/Byline'
import BabyfoodSettings from '../components/BabyfoodSettings'
import FoodPicker from '../components/FoodPicker'
import DateNav from '../components/DateNav'

// 하루를 중심에 두고 주 단위로 넘긴다.
export default function Babyfood() {
  const nav = useNavigate()
  const [params, setParams] = useSearchParams()
  const linked = params.get('date')
  const valid = (d: string | null) => !!d && /^\d{4}-\d{2}-\d{2}$/.test(d)
  const initial = valid(linked) ? linked! : today()

  const [selected, setSelectedState] = useState(initial)
  const [week, setWeek] = useState(() => startOfWeek(initial))
  const [editing, setEditing] = useState<BFMeal | null>(null)
  const [settings, setSettings] = useState(false)
  // 스와이프 뒤에 손가락 아래 날짜 버튼의 click 이 이어 터진다 — 그 click 은 무시한다.
  const touch = useRef({ x: 0, swiped: false })

  const setSelected = (d: string) => {
    setSelectedState(d)
    setParams({ date: d }, { replace: true })
  }

  // 주를 옮기면 선택도 같은 요일로 옮긴다(안 그러면 상세가 이전 주 날짜를 보여준다).
  const goToday = () => {
    setWeek(startOfWeek(today()))
    setSelected(today())
  }

  const shiftWeek = (weeks: number) => {
    const w = addDays(week, weeks * 7)
    setWeek(w)
    setSelected(addDays(w, weekdayIndex(selected)))
  }

  useEffect(() => {
    if (valid(linked) && linked !== selected) {
      setSelectedState(linked!)
      setWeek(startOfWeek(linked!))
    }
    // linked 만 본다 — selected 를 넣으면 사용자가 고른 날짜를 되돌린다.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [linked])

  // 대상이 하나면 서버가 고르므로 비워둔다.
  const [childID, setChildID] = useState<number | undefined>(() => {
    const v = params.get('child')
    return v ? Number(v) : undefined
  })

  const q = useBabyfood(week, addDays(week, 6), childID)
  const children = q.data?.children ?? []
  const child = q.data?.child
  const byDate = useMemo(() => {
    const m = new Map<string, BFDay>()
    for (const d of q.data?.days ?? []) m.set(d.date, d)
    return m
  }, [q.data])

  const day = byDate.get(selected)
  const days = Array.from({ length: 7 }, (_, i) => addDays(week, i))

  const patchMeal = useInvalidating(
    ({ id, ...body }: { id: number } & Record<string, unknown>) => api.patch<BFDay>(`/api/babyfood/meals/${id}`, body),
    [['babyfood'], ['babyfood-stock']],
  )
  // 반응·좋아함은 그날 그 재료에 남는다(BFSetLog). 음식 화면이 모아서 보여준다.
  const foods = useBFFoods(childID)
  const foodBy = useMemo(() => {
    const m = new Map<string, BFFood>()
    for (const f of foods.data ?? []) m.set(f.name, f)
    return m
  }, [foods.data])
  const setLog = useInvalidating(
    (body: { date: string; name: string; reaction?: boolean; liked?: boolean; disliked?: boolean }) =>
      api.post<BFDay>(`/api/babyfood/logs${childID ? `?child=${childID}` : ''}`, body),
    [['babyfood'], ['babyfood-foods']],
  )

  const settingsSheet = (
    <Sheet open={settings} onClose={() => setSettings(false)} title="이유식 설정">
      <BabyfoodSettings />
    </Sheet>
  )

  if (q.data && children.length === 0) {
    return (
      <div className="mx-auto max-w-lg">
        <PageHeader title="식단" right={<SettingsButton onClick={() => setSettings(true)} />} />
        <div className="px-4 pt-10 text-center">
          <p className="text-sm text-muted">이유식을 진행할 가족을 고르면 식단표가 날짜에 맞춰 펼쳐져요.</p>
          <Button className="mt-4" onClick={() => setSettings(true)}>이유식 설정</Button>
          <p className="mt-3 text-xs text-faint">
            생년월일이 있어야 고를 수 있어요.{' '}
            <button onClick={() => nav('/?settings=1')} className="font-medium text-muted underline">가족 설정</button>
          </p>
        </div>
        {settingsSheet}
      </div>
    )
  }

  return (
    <div className="mx-auto max-w-lg">
      <PageHeader
        title={children.length > 1 && child ? `${child.name} 식단` : '식단'}
        right={
          <div className="flex items-center gap-1.5">
            <Button variant="ghost" onClick={() => nav(`/babyfood/foods${childID ? `?child=${childID}` : ''}`)}>음식</Button>
            <Button variant="ghost" onClick={() => nav(`/babyfood/stock${childID ? `?child=${childID}` : ''}`)}>재고</Button>
            <SettingsButton onClick={() => setSettings(true)} />
          </div>
        }
      />

      {children.length > 1 && (
        <div className="flex gap-1.5 px-4 pt-3">
          {children.map((c) => (
            <button
              key={c.user_id}
              onClick={() => { setChildID(c.user_id); setParams({ date: selected, child: String(c.user_id) }, { replace: true }) }}
              className={`rounded-lg px-3 py-1.5 text-sm font-medium ${
                (child?.user_id ?? 0) === c.user_id ? 'bg-accent text-accent-ink' : 'bg-surface-2 text-muted'
              }`}
            >
              {c.name}
            </button>
          ))}
        </div>
      )}

      <div className="px-2 pt-2">
        <div className="mb-1 px-2">
          <DateNav
            value={selected}
            label={`${week.slice(5).replace('-', '/')} – ${addDays(week, 6).slice(5).replace('-', '/')}`}
            isToday={week === startOfWeek(today()) && selected === today()}
            onPick={(d) => { setWeek(startOfWeek(d)); setSelected(d) }}
            onPrev={() => shiftWeek(-1)}
            onNext={() => shiftWeek(1)}
            onToday={goToday}
          />
        </div>
        <div
          className="grid grid-cols-7 gap-1"
          style={{ touchAction: 'pan-y' }}
          onTouchStart={(e) => { touch.current = { x: e.touches[0].clientX, swiped: false } }}
          onTouchEnd={(e) => {
            const dx = e.changedTouches[0].clientX - touch.current.x
            // 40px 이상이면 주 이동, 작으면 탭.
            if (Math.abs(dx) > 40) {
              touch.current.swiped = true
              shiftWeek(dx < 0 ? 1 : -1)
            }
          }}
        >
          {days.map((d) => {
            const has = byDate.get(d)
            const isSel = d === selected
            const isToday = d === today()
            const wd = weekdayIndex(d)
            return (
              <button
                key={d}
                onClick={() => {
                  if (touch.current.swiped) { touch.current.swiped = false; return }
                  setSelected(d)
                }}
                className={`flex flex-col items-center rounded-xl py-1.5 ${isSel ? 'bg-accent text-accent-ink' : has ? 'bg-surface text-ink shadow-sm' : 'text-ghost'}`}
              >
                <span className={`text-[10px] ${isSel ? 'text-ghost' : wd === 0 ? 'text-rose-400' : wd === 6 ? 'text-sky-400' : 'text-faint'}`}>
                  {WEEKDAYS[wd]}
                </span>
                <span className={`mt-0.5 flex h-6 w-6 items-center justify-center rounded-full text-sm ${isToday && !isSel ? 'bg-line font-bold' : ''}`}>
                  {Number(d.slice(8))}
                </span>
                <span className={`mt-0.5 h-1 w-1 rounded-full ${has?.meals.some((m) => m.edited) ? (isSel ? 'bg-surface' : 'bg-amber-500') : 'bg-transparent'}`} />
              </button>
            )
          })}
        </div>
      </div>

      <section className="px-4 pt-4">
        <h2 className="text-base font-bold">{fmtDate(selected)}</h2>
        {day && <p className="text-xs text-faint">{lifeDay(day.dday)} · {day.label}</p>}

        {q.isPending && <div className="mt-3"><SkeletonList rows={3} /></div>}
        {!q.isPending && !day && (
          <p className="py-10 text-center text-sm text-faint">이 날짜에는 식단이 없어요</p>
        )}

        {day && (
          <div className="mt-3 space-y-3">
            {day.meals.map((m) => (
              <MealCard
                key={m.id}
                meal={m}
                menu={day.kind === 'menu'}
                foodBy={foodBy}
                onEdit={() => setEditing(m)}
                onSkip={(v) => patchMeal.mutate({ id: m.id, skipped: v })}
                onReset={() => patchMeal.mutate({ id: m.id, reset: true })}
              />
            ))}
            <DayLog day={day} onSet={(name, body) => setLog.mutate({ date: day.date, name, ...body })} />
          </div>
        )}
      </section>

      {settingsSheet}

      <Sheet open={!!editing} onClose={() => setEditing(null)} title={editing ? `${editing.title} 고치기` : ''}>
        {editing && (
          <MealForm
            meal={editing}
            foods={foods.data ?? []}
            onSubmit={(body) => {
              patchMeal.mutate({ id: editing.id, ...body })
              setEditing(null)
            }}
            onClose={() => setEditing(null)}
          />
        )}
      </Sheet>
    </div>
  )
}

function SettingsButton({ onClick }: { onClick: () => void }) {
  return (
    <button onClick={onClick} aria-label="이유식 설정" className="rounded-lg p-1.5 text-muted active:bg-line">
      <svg viewBox="0 0 24 24" className="h-5 w-5" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
        <circle cx="12" cy="12" r="3" />
        <path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 1 1-2.83 2.83l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 1 1-4 0v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 1 1-2.83-2.83l.06-.06a1.65 1.65 0 0 0 .33-1.82 1.65 1.65 0 0 0-1.51-1H3a2 2 0 1 1 0-4h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 1 1 2.83-2.83l.06.06A1.65 1.65 0 0 0 9 4.6a1.65 1.65 0 0 0 1-1.51V3a2 2 0 1 1 4 0v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 1 1 2.83 2.83l-.06.06a1.65 1.65 0 0 0-.33 1.82V9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 1 1 0 4h-.09a1.65 1.65 0 0 0-1.51 1z" />
      </svg>
    </button>
  )
}

// 그날 먹은 것들의 기록(처음 먹는 재료만이 아니라 전부).
function DayLog({ day, onSet }: {
  day: BFDay
  onSet: (name: string, body: { reaction?: boolean; liked?: boolean; disliked?: boolean }) => void
}) {
  const logs = new Map<string, BFLog>(day.logs.map((l) => [l.name, l]))

  const names: string[] = []
  const seen = new Set<string>()
  const push = (n?: string | null) => {
    if (n && !seen.has(n)) { seen.add(n); names.push(n) }
  }
  for (const m of day.meals) {
    // 건너뛴 끼니는 이미 적어둔 것만 남긴다.
    const keep = (n: string) => !m.skipped || logs.has(n)
    if (keep(m.base)) push(m.base)
    for (const t of m.toppings) if (keep(t)) push(t)
    if (m.snack && keep(m.snack)) push(m.snack)
  }
  push(day.new_item)
  if (names.length === 0) return null

  return (
    <div className="rounded-2xl bg-surface p-3.5 shadow-sm">
      <p className="text-[11px] font-medium text-muted">먹고 나서</p>
      <p className="mt-0.5 text-[11px] text-faint">
        두드러기·발진이 올라왔는지, 잘 먹었는지 남겨주세요. 반응은 보통 하루 안에 나타나요.
      </p>
      <ul className="mt-2 space-y-1">
        {names.map((n) => {
          const l = logs.get(n)
          return (
            <li key={n} className="flex items-center gap-1.5">
              <span className="min-w-0 flex-1 truncate text-sm">
                {n}
                {n === day.new_item && (
                  <span className="ml-1 rounded bg-amber-100 px-1 py-0.5 text-[10px] font-medium text-amber-700">처음</span>
                )}
              </span>
              <TagButton on={!!l?.reaction} onClick={() => onSet(n, { reaction: !l?.reaction })} tone="rose" compact>
                반응
              </TagButton>
              <TagButton on={!!l?.liked} onClick={() => onSet(n, { liked: !l?.liked })} tone="amber" compact>
                좋아함
              </TagButton>
              <TagButton on={!!l?.disliked} onClick={() => onSet(n, { disliked: !l?.disliked })} tone="sky" compact>
                싫어함
              </TagButton>
            </li>
          )
        })}
      </ul>
    </div>
  )
}

function TagButton({ on, onClick, tone, compact, children }: {
  on: boolean
  onClick: () => void
  tone: 'rose' | 'amber' | 'sky'
  compact?: boolean
  children: React.ReactNode
}) {
  const active = { rose: 'bg-rose-500 text-white', amber: 'bg-amber-400 text-white', sky: 'bg-sky-500 text-white' }[tone]
  const size = compact
    ? 'h-7 shrink-0 rounded-lg px-2 text-xs font-semibold'
    : 'flex-1 rounded-lg py-2 text-sm font-semibold'
  return (
    <button
      onClick={onClick}
      aria-pressed={on}
      className={`${size} ${on ? active : 'bg-surface-2 text-faint'}`}
    >
      {children}
    </button>
  )
}

function MealCard({ meal, menu, foodBy, onEdit, onSkip, onReset }: {
  meal: BFMeal
  menu: boolean
  foodBy: Map<string, BFFood>
  onEdit: () => void
  onSkip: (v: boolean) => void
  onReset: () => void
}) {
  return (
    <div className={`rounded-2xl p-3.5 shadow-sm ${meal.eaten ? 'bg-emerald-50 ring-1 ring-emerald-200' : 'bg-surface'} ${meal.skipped ? 'opacity-50' : ''}`}>
      <div className="flex items-center gap-2">
        <span className="rounded-md bg-surface-2 px-1.5 py-0.5 text-[11px] font-medium text-ink-2">{meal.title}</span>
        {meal.at && (
          <span className={`text-[11px] ${meal.at_set ? 'font-medium text-ink-2' : 'text-faint'}`}>{meal.at}</span>
        )}
        {meal.edited && <span className="rounded-md bg-amber-100 px-1.5 py-0.5 text-[11px] font-medium text-amber-700">수정됨</span>}
        {meal.skipped && <span className="rounded-md bg-line px-1.5 py-0.5 text-[11px] text-ink-2">건너뜀</span>}
        {meal.eaten && (
          <span className="ml-auto text-[11px] font-semibold text-emerald-700">
            ✓ 먹었어요 {meal.eaten.at.slice(11)}{meal.eaten.amount_ml != null && ` · ${meal.eaten.amount_ml}ml`}
          </span>
        )}
      </div>

      {/* 칸 전체가 수정 진입점이다. */}
      <button onClick={onEdit} className="mt-2 block w-full text-left">
        <p className="flex items-center gap-1 text-sm font-semibold">
          <span className="min-w-0 flex-1 truncate">{meal.base || '—'}</span>
          <svg viewBox="0 0 24 24" className="h-3.5 w-3.5 shrink-0 text-ghost" fill="none" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round"><path d="M9 6l6 6-6 6" /></svg>
        </p>
        {!menu && meal.toppings.length > 0 && (
          <div className="mt-1.5 flex flex-wrap gap-1">
            {meal.toppings.map((t, i) => {
              const f = foodBy.get(t)
              // 반응이 가장 세다 — 잘 먹더라도 빼야 한다.
              const cls = f?.reaction
                ? 'bg-rose-100 text-rose-700'
                : f?.disliked
                  ? 'bg-sky-100 text-sky-700'
                  : f?.liked
                    ? 'bg-amber-100 text-amber-800'
                    : 'bg-surface-2 text-ink-2'
              return (
                <span key={`${t}-${i}`} className={`rounded-md px-1.5 py-0.5 text-xs ${cls}`}>{t}</span>
              )
            })}
          </div>
        )}
        {menu && meal.toppings.length > 0 && (
          <p className="mt-1 text-xs text-muted">{meal.toppings.join(' · ')}</p>
        )}
        {meal.snack && <p className="mt-1.5 text-xs text-muted">간식 · {meal.snack}</p>}
        {(meal.eaten_g != null || meal.served_g != null) && (
          <p className="mt-1.5 text-xs text-faint">먹은 양 {meal.eaten_g ?? '—'} / {meal.served_g ?? '—'} g</p>
        )}
      </button>

      {meal.edited && (
        <div className="mt-2 border-t border-line pt-2">
          <p className="text-[11px] text-faint">
            원래 · {meal.at_set && '시각 기본값 · '}{meal.src.base}
            {meal.src.toppings.length > 0 && ` / ${meal.src.toppings.join(' · ')}`}
            {meal.src.snack && ` / 간식 ${meal.src.snack}`}
          </p>
          <Byline label="고친 사람" by={meal.edited_by} at={meal.edited_at} className="mt-0.5" />
          <button onClick={onReset} className="mt-1 text-[11px] font-medium text-muted underline">원래대로</button>
        </div>
      )}

      <button
        onClick={() => onSkip(!meal.skipped)}
        className="mt-2 text-[11px] text-faint underline"
      >
        {meal.skipped ? '건너뜀 취소' : '이 끼니 건너뜀'}
      </button>
    </div>
  )
}

function MealForm({ meal, foods, onSubmit, onClose }: {
  meal: BFMeal
  foods: BFFood[]
  onSubmit: (body: Record<string, unknown>) => void
  onClose: () => void
}) {
  // 셋 다 이름 목록. 베이스·간식은 한 칸짜리.
  const [base, setBase] = useState<string[]>(meal.base ? [meal.base] : [])
  const [tops, setTops] = useState<string[]>(meal.toppings)
  const [snack, setSnack] = useState<string[]>(meal.snack ? [meal.snack] : [])
  const [eaten, setEaten] = useState(meal.eaten_g?.toString() ?? '')
  const [served, setServed] = useState(meal.served_g?.toString() ?? '')
  // 직접 넣은 값이 없으면 비워둔다('설정을 따르는 중' 표시).
  const [at, setAt] = useState(meal.at_set ? meal.at : '')

  const num = (s: string) => (s.trim() === '' ? -1 : Number(s))

  return (
    <form
      onSubmit={(e) => {
        e.preventDefault()
        onSubmit({
          base: base[0] ?? '',
          toppings: tops,
          snack: snack[0] ?? '',
          eaten_g: num(eaten),
          served_g: num(served),
          at,
        })
      }}
      className="space-y-3"
    >
      <Field label="시각">
        <TimeField value={at} fallback={meal.at} onChange={setAt} />
        <p className="mt-1 flex items-center gap-1.5 text-[11px] text-faint">
          {at
            ? <>이 끼니만 이 시각으로 둡니다.
                <button type="button" onClick={() => setAt('')} className="font-medium text-muted underline">기본값으로</button>
              </>
            : `설정의 기본값(${meal.at})을 따르는 중이에요.`}
        </p>
      </Field>
      <Field label="베이스">
        <FoodPicker value={base} foods={foods} prefer="base" src={[meal.src.base]} onChange={setBase} />
      </Field>
      <Field label="토핑">
        <FoodPicker value={tops} foods={foods} multi prefer="cube" src={meal.src.toppings} onChange={setTops} />
      </Field>
      <Field label="간식">
        <FoodPicker value={snack} foods={foods} prefer="dish" src={meal.src.snack ? [meal.src.snack] : []} onChange={setSnack} />
      </Field>
      <div className="flex gap-2">
        <div className="flex-1">
          <Field label="먹은 양 (g)">
            <Input inputMode="numeric" value={eaten} onChange={(e) => setEaten(e.target.value)} />
          </Field>
        </div>
        <div className="flex-1">
          <Field label="차린 양 (g)">
            <Input inputMode="numeric" value={served} onChange={(e) => setServed(e.target.value)} />
          </Field>
        </div>
      </div>
      <p className="text-xs text-faint">고쳐도 원래 내용은 남아 있어서 언제든 되돌릴 수 있어요.</p>
      <div className="flex justify-end gap-2 pt-1">
        <Button type="button" variant="ghost" onClick={onClose}>취소</Button>
        <Button type="submit">저장</Button>
      </div>
    </form>
  )
}
