import { useEffect, useMemo, useRef, useState } from 'react'
import { useNavigate, useSearchParams } from 'react-router'
import { api, type BFDay, type BFFood, type BFMeal } from '../api'
import { useBabyfood, useBFFoods, useInvalidating } from '../lib/hooks'
import { addDays, fmtDate, startOfWeek, today, WEEKDAYS, weekdayIndex } from '../lib/date'
import { Button, Field, Input, PageHeader, Sheet, SkeletonList } from '../components/ui'

// 책은 6칸짜리 가로 표지만, 폰에서는 그걸 그대로 못 읽는다. 하루를 중심에 두고
// 주 단위로 넘긴다.
export default function Babyfood() {
  const nav = useNavigate()
  const [params, setParams] = useSearchParams()
  const linked = params.get('date')
  const valid = (d: string | null) => !!d && /^\d{4}-\d{2}-\d{2}$/.test(d)
  const initial = valid(linked) ? linked! : today()

  const [selected, setSelectedState] = useState(initial)
  const [week, setWeek] = useState(() => startOfWeek(initial))
  const [editing, setEditing] = useState<BFMeal | null>(null)
  // 스와이프로 주를 넘길 때, 손가락 아래 있던 날짜 버튼의 click 이 뒤이어
  // 터진다. 그대로 두면 방금 넘긴 주의 선택이 이전 주 날짜로 되돌아간다.
  const touch = useRef({ x: 0, swiped: false })

  const setSelected = (d: string) => {
    setSelectedState(d)
    setParams({ date: d }, { replace: true })
  }

  // 주만 옮기고 선택을 두면 넘어간 주에 짚힌 날이 없어서, 아래 상세가 이전
  // 주의 날짜를 계속 보여준다. 같은 요일로 함께 옮긴다.
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

  // 주 스트립과 선택한 날을 한 번에 받는다.
  const q = useBabyfood(week, addDays(week, 6))
  const profile = q.data?.profile
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
  // 알레르기 반응·좋아함은 날짜가 아니라 **재료**에 붙는다. 같은 재료가 194일에
  // 걸쳐 수십 번 나오니까, 날짜에 달면 한눈에 모이지 않는다.
  const foods = useBFFoods()
  const foodBy = useMemo(() => {
    const m = new Map<string, BFFood>()
    for (const f of foods.data ?? []) m.set(f.name, f)
    return m
  }, [foods.data])
  const tagFood = useInvalidating(
    (body: { name: string; reaction?: boolean; liked?: boolean }) => api.post<BFFood>('/api/babyfood/foods/tag', body),
    [['babyfood-foods']],
  )

  // 생일이 없으면 날짜를 계산할 수 없다. 설정으로 보낸다.
  if (profile && !profile.birth_date) {
    return (
      <div className="mx-auto max-w-lg">
        <PageHeader title="이유식" />
        <div className="px-4 pt-10 text-center">
          <p className="text-sm text-slate-500">아이 생일을 입력하면 식단표가 날짜에 맞춰 펼쳐져요.</p>
          <Button className="mt-4" onClick={() => nav('/?settings=1')}>생일 입력하기</Button>
        </div>
      </div>
    )
  }

  return (
    <div className="mx-auto max-w-lg">
      <PageHeader
        title="이유식"
        right={
          <div className="flex items-center gap-1.5">
            <Button variant="ghost" onClick={() => nav('/babyfood/foods')}>음식</Button>
            <Button variant="ghost" onClick={() => nav('/babyfood/stock')}>재고</Button>
          </div>
        }
      />

      {/* 주 스트립 — 일요일 시작 */}
      <div className="px-2 pt-2">
        <div className="mb-1 flex items-center justify-between px-2">
          <button onClick={() => shiftWeek(-1)} aria-label="지난 주" className="rounded-lg px-2 py-1 text-slate-500 active:bg-slate-200">‹</button>
          {/* 루틴과 같다 — 가운데 라벨을 누르면 오늘로 돌아온다. */}
          <button onClick={goToday} className="text-sm font-medium text-slate-600">
            {week.slice(5).replace('-', '/')} – {addDays(week, 6).slice(5).replace('-', '/')}
          </button>
          <button onClick={() => shiftWeek(1)} aria-label="다음 주" className="rounded-lg px-2 py-1 text-slate-500 active:bg-slate-200">›</button>
        </div>
        <div
          className="grid grid-cols-7 gap-1"
          style={{ touchAction: 'pan-y' }}
          onTouchStart={(e) => { touch.current = { x: e.touches[0].clientX, swiped: false } }}
          onTouchEnd={(e) => {
            const dx = e.changedTouches[0].clientX - touch.current.x
            // 40px 이상 움직였을 때만 주 이동. 그보다 작으면 탭으로 본다.
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
                className={`flex flex-col items-center rounded-xl py-1.5 ${isSel ? 'bg-slate-900 text-white' : has ? 'bg-white text-slate-800 shadow-sm' : 'text-slate-300'}`}
              >
                <span className={`text-[10px] ${isSel ? 'text-slate-300' : wd === 0 ? 'text-rose-400' : wd === 6 ? 'text-sky-400' : 'text-slate-400'}`}>
                  {WEEKDAYS[wd]}
                </span>
                <span className={`mt-0.5 flex h-6 w-6 items-center justify-center rounded-full text-sm ${isToday && !isSel ? 'bg-slate-200 font-bold' : ''}`}>
                  {Number(d.slice(8))}
                </span>
                {/* 그날 수정한 끼니가 있으면 점 하나 */}
                <span className={`mt-0.5 h-1 w-1 rounded-full ${has?.meals.some((m) => m.edited) ? (isSel ? 'bg-white' : 'bg-amber-500') : 'bg-transparent'}`} />
              </button>
            )
          })}
        </div>
      </div>

      <section className="px-4 pt-4">
        <h2 className="text-base font-bold">{fmtDate(selected)}</h2>
        {day && <p className="text-xs text-slate-400">D+{day.dday} · {day.label}</p>}

        {q.isPending && <div className="mt-3"><SkeletonList rows={3} /></div>}
        {!q.isPending && !day && (
          <p className="py-10 text-center text-sm text-slate-400">이 날짜에는 식단이 없어요</p>
        )}

        {day && (
          <div className="mt-3 space-y-3">
            {day.new_item && (
              <NewItemCard
                name={day.new_item}
                food={foodBy.get(day.new_item)}
                onTag={(body) => tagFood.mutate({ name: day.new_item!, ...body })}
              />
            )}
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
          </div>
        )}
      </section>

      <Sheet open={!!editing} onClose={() => setEditing(null)} title={editing ? `${editing.slot} 고치기` : ''}>
        {editing && (
          <MealForm
            meal={editing}
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

// 그날 처음 먹이는 재료. 여기서 바로 표시를 달 수 있게 둔다 — 먹인 직후가
// 기록하기 가장 쉬운 순간이라서.
function NewItemCard({ name, food, onTag }: {
  name: string
  food?: BFFood
  onTag: (body: { reaction?: boolean; liked?: boolean }) => void
}) {
  return (
    <div className="rounded-xl bg-amber-50 p-3">
      <p className="text-[11px] font-medium text-amber-700">오늘 처음 먹는 재료</p>
      <p className="mt-0.5 text-base font-semibold text-amber-900">{name}</p>
      <p className="mt-1 text-[11px] text-amber-700/70">
        두드러기·발진이 올라왔는지, 잘 먹었는지 남겨주세요. 반응은 보통 하루 안에 나타나요.
      </p>
      <div className="mt-2 flex gap-2">
        <TagButton on={!!food?.reaction} onClick={() => onTag({ reaction: !food?.reaction })} tone="rose">
          알레르기 반응
        </TagButton>
        <TagButton on={!!food?.liked} onClick={() => onTag({ liked: !food?.liked })} tone="amber">
          좋아함
        </TagButton>
      </div>
    </div>
  )
}

function TagButton({ on, onClick, tone, children }: {
  on: boolean
  onClick: () => void
  tone: 'rose' | 'amber'
  children: React.ReactNode
}) {
  const active = tone === 'rose' ? 'bg-rose-500 text-white' : 'bg-amber-400 text-white'
  return (
    <button
      onClick={onClick}
      aria-pressed={on}
      className={`flex-1 rounded-lg py-2 text-sm font-semibold ${on ? active : 'bg-white text-slate-500'}`}
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
    <div className={`rounded-xl bg-white p-3 shadow-sm ${meal.skipped ? 'opacity-50' : ''}`}>
      <div className="flex items-center gap-2">
        <span className="rounded-md bg-slate-100 px-1.5 py-0.5 text-[11px] font-medium text-slate-600">{meal.slot}</span>
        {meal.edited && <span className="rounded-md bg-amber-100 px-1.5 py-0.5 text-[11px] font-medium text-amber-700">수정됨</span>}
        {meal.skipped && <span className="rounded-md bg-slate-200 px-1.5 py-0.5 text-[11px] text-slate-600">건너뜀</span>}
        <button onClick={onEdit} className="ml-auto rounded-lg px-2 py-1 text-xs text-slate-500 active:bg-slate-100">고치기</button>
      </div>

      <button onClick={onEdit} className="mt-2 block w-full text-left">
        <p className="text-sm font-semibold">{meal.base || '—'}</p>
        {!menu && meal.toppings.length > 0 && (
          <div className="mt-1.5 flex flex-wrap gap-1">
            {meal.toppings.map((t, i) => {
              const f = foodBy.get(t)
              // 반응이 우선이다 — 잘 먹더라도 빼야 하는 재료니까.
              const cls = f?.reaction
                ? 'bg-rose-100 text-rose-700'
                : f?.liked
                  ? 'bg-amber-100 text-amber-800'
                  : 'bg-slate-100 text-slate-700'
              return (
                <span key={`${t}-${i}`} className={`rounded-md px-1.5 py-0.5 text-xs ${cls}`}>{t}</span>
              )
            })}
          </div>
        )}
        {menu && meal.toppings.length > 0 && (
          <p className="mt-1 text-xs text-slate-500">{meal.toppings.join(' · ')}</p>
        )}
        {meal.snack && <p className="mt-1.5 text-xs text-slate-500">간식 · {meal.snack}</p>}
        {(meal.eaten_g != null || meal.served_g != null) && (
          <p className="mt-1.5 text-xs text-slate-400">먹은 양 {meal.eaten_g ?? '—'} / {meal.served_g ?? '—'} g</p>
        )}
      </button>

      {meal.edited && (
        <div className="mt-2 border-t border-slate-100 pt-2">
          <p className="text-[11px] text-slate-400">
            원래 · {meal.src.base}
            {meal.src.toppings.length > 0 && ` / ${meal.src.toppings.join(' · ')}`}
            {meal.src.snack && ` / 간식 ${meal.src.snack}`}
          </p>
          <button onClick={onReset} className="mt-1 text-[11px] font-medium text-slate-500 underline">원래대로</button>
        </div>
      )}

      <button
        onClick={() => onSkip(!meal.skipped)}
        className="mt-2 text-[11px] text-slate-400 underline"
      >
        {meal.skipped ? '건너뜀 취소' : '이 끼니 건너뜀'}
      </button>
    </div>
  )
}

// 토핑은 쉼표로 받는다. 폰에서 칩을 하나씩 추가하는 것보다 빠르다.
function MealForm({ meal, onSubmit, onClose }: {
  meal: BFMeal
  onSubmit: (body: Record<string, unknown>) => void
  onClose: () => void
}) {
  const [base, setBase] = useState(meal.base)
  const [tops, setTops] = useState(meal.toppings.join(', '))
  const [snack, setSnack] = useState(meal.snack ?? '')
  const [eaten, setEaten] = useState(meal.eaten_g?.toString() ?? '')
  const [served, setServed] = useState(meal.served_g?.toString() ?? '')

  const num = (s: string) => (s.trim() === '' ? -1 : Number(s))

  return (
    <form
      onSubmit={(e) => {
        e.preventDefault()
        onSubmit({
          base,
          toppings: tops.split(',').map((t) => t.trim()).filter(Boolean),
          snack,
          eaten_g: num(eaten),
          served_g: num(served),
        })
      }}
      className="space-y-3"
    >
      <Field label="베이스">
        <Input value={base} onChange={(e) => setBase(e.target.value)} />
      </Field>
      <Field label="토핑 (쉼표로 구분)">
        <Input value={tops} onChange={(e) => setTops(e.target.value)} placeholder="쉼표로 구분해서 적어주세요" />
      </Field>
      <Field label="간식">
        <Input value={snack} onChange={(e) => setSnack(e.target.value)} placeholder="없으면 비워두세요" />
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
      <p className="text-xs text-slate-400">고쳐도 원래 내용은 남아 있어서 언제든 되돌릴 수 있어요.</p>
      <div className="flex justify-end gap-2 pt-1">
        <Button type="button" variant="ghost" onClick={onClose}>취소</Button>
        <Button type="submit">저장</Button>
      </div>
    </form>
  )
}
