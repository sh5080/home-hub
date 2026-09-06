import { useEffect, useRef, useState } from 'react'
import { useNavigate, useSearchParams } from 'react-router'
import { api, type CareDay, type CareKind, type CareLog } from '../api'
import { useBabyfood, useBFChildren, useBFFoods, useCareDay, useCareKinds, useCareLast, useInvalidating } from '../lib/hooks'
import { addDays, fmtDate, lifeDayOf, toDateTimeStr, today } from '../lib/date'
import { Button, Field, Input, PageHeader, Sheet, SkeletonList, TimeField } from '../components/ui'
import { useConfirm } from '../components/Confirm'
import DateNav from '../components/DateNav'
import FoodPicker from '../components/FoodPicker'
import { useEdgePull } from '../lib/useEdgePull'

/** 육아 기록. 버튼을 누르면 지금 시각으로 한 줄이 생기고, 양·시간은 그 줄을 눌러 고친다. */

const TONE: Record<string, string> = {
  breast: 'bg-rose-400', formula: 'bg-amber-400', solids: 'bg-teal-400', diaper: 'bg-stone-400',
  sleep: 'bg-indigo-400', pump_feed: 'bg-lime-500', pump: 'bg-emerald-400', bath: 'bg-sky-400',
  hospital: 'bg-red-500', medicine: 'bg-fuchsia-400', snack: 'bg-orange-400', milk: 'bg-yellow-300',
  water: 'bg-cyan-400', play: 'bg-violet-400', tummy: 'bg-pink-400', temp: 'bg-red-400', etc: 'bg-slate-400',
}
const EMOJI: Record<string, string> = {
  breast: '🤱', formula: '🍼', solids: '🥣', diaper: '🩲', sleep: '😴', pump_feed: '🍼', pump: '🫙',
  bath: '🛁', hospital: '🏥', medicine: '💊', snack: '🍪', milk: '🥛', water: '💧', play: '🧸', tummy: '🐢',
  temp: '🌡️', etc: '📝',
}

export default function Care() {
  const nav = useNavigate()
  const confirm = useConfirm()
  const [params, setParams] = useSearchParams()
  const childID = params.get('child') ? Number(params.get('child')) : undefined
  const [date, setDate] = useState(today())
  const [editing, setEditing] = useState<CareLog | null>(null)

  const kids = useBFChildren()
  const kinds = useCareKinds()
  const day = useCareDay(date, childID)
  const last = useCareLast(childID)
  const child = kids.data?.find((c) => c.user_id === childID) ?? kids.data?.[0]
  const q = childID ? `?child=${childID}` : ''

  const keys = [['care'], ['care-last']]
  const add = useInvalidating((kind: string) => api.post<CareLog>(`/api/care${q}`, { kind }), keys)
  // 이유식 기록은 식단의 '먹었어요'도 바꾼다.
  const patch = useInvalidating(({ id, ...body }: { id: number } & Record<string, unknown>) => api.patch<CareLog>(`/api/care/${id}`, body), [...keys, ['babyfood']])
  const del = useInvalidating((id: number) => api.del(`/api/care/${id}`), keys)

  const [, tick] = useState(0)
  useEffect(() => { const t = window.setInterval(() => tick((n) => n + 1), 30_000); return () => window.clearInterval(t) }, [])

  if (kids.data && kids.data.length === 0) {
    return (
      <div className="mx-auto max-w-lg">
        <PageHeader title="기록" />
        <div className="px-4 pt-10 text-center">
          <p className="text-sm text-muted">가족에 생년월일을 넣고 이유식 대상으로 고르면 여기서 기록할 수 있어요.</p>
          <Button className="mt-4" onClick={() => nav('/babyfood')}>설정으로</Button>
        </div>
      </div>
    )
  }

  const kindOf = (key: string) => kinds.data?.find((k) => k.key === key)
  const isToday = date === today()

  // 목록 끝에서 더 밀면 전날, 맨 위에서 더 당기면 다음 날.
  const listRef = useRef<HTMLDivElement>(null)
  const go = (d: string) => {
    setDate(d)
    listRef.current?.scrollTo({ top: 0 })
  }
  const edge = useEdgePull(listRef, {
    onBottom: () => go(addDays(date, -1)),
    onTop: isToday ? undefined : () => go(addDays(date, 1)),
  })

  return (
    // 기록 줄 칸만 스크롤한다(끌어서 날짜 넘기기도 이 칸에서만).
    <div className="mx-auto flex h-full max-w-lg flex-col">
      <div className="shrink-0">
      <PageHeader
        title={child ? `${child.name} 기록` : '기록'}
        right={child && <span className="text-xs text-faint">{lifeDayOf(child.birth_date)}</span>}
      />

      {kids.data && kids.data.length > 1 && (
        <div className="flex gap-1.5 px-4 pt-3">
          {kids.data.map((c) => (
            <button key={c.user_id} onClick={() => setParams({ child: String(c.user_id) }, { replace: true })}
              className={`rounded-lg px-3 py-1.5 text-sm font-medium ${(child?.user_id ?? 0) === c.user_id ? 'bg-accent text-accent-ink' : 'bg-surface-2 text-muted'}`}>
              {c.name}
            </button>
          ))}
        </div>
      )}

      <div className="no-scrollbar flex gap-3 overflow-x-auto px-4 pt-4">
        {(kinds.data ?? []).map((k) => (
          <button
            key={k.key}
            disabled={add.isPending}
            onClick={async () => {
              const l = await add.mutateAsync(k.key)
              if (k.amount || k.free || (k.details && k.details.length > 0)) setEditing(l)
              if (!isToday) setDate(today())
            }}
            className="flex w-16 shrink-0 flex-col items-center gap-1.5"
          >
            <span className={`flex h-14 w-14 items-center justify-center rounded-full text-2xl shadow-sm active:scale-95 ${TONE[k.key] ?? 'bg-line'}`}>
              {EMOJI[k.key] ?? '•'}
            </span>
            <span className="text-[11px] font-medium text-ink-2">{k.label}</span>
          </button>
        ))}
      </div>

      {last.data && (
        <div className="mt-4 grid grid-cols-3 gap-2 px-4">
          <LastCard label="마지막 기저귀" log={last.data.diaper} sub={(l) => l.detail ?? ''} />
          <LastCard label="마지막 수유" log={latestOf(last.data, ['breast', 'formula', 'pump_feed'])} sub={(l) => l.amount_ml ? `${l.amount_ml}ml` : (kindOf(l.kind)?.label ?? '')} />
          <LastCard label="마지막 잠" log={last.data.sleep} sub={(l) => l.minutes !== null ? fmtMin(l.minutes) : ongoing(l) ? '자는 중' : (l.detail ?? '')} />
        </div>
      )}

      <div className="mt-4 px-4">
        <DateNav
          value={date}
          label={fmtDate(date)}
          isToday={isToday}
          onPick={(d) => setDate(d > today() ? today() : d)}
          onPrev={() => setDate(addDays(date, -1))}
          onNext={() => setDate(addDays(date, 1))}
          onToday={() => setDate(today())}
          nextDisabled={isToday}
        />
      </div>
      {day.data && day.data.logs.length > 0 && <DaySummary d={day.data} />}

      </div>

      <div ref={listRef} className="mt-2 min-h-0 flex-1 overflow-y-auto overscroll-y-none">
        {edge.pull?.edge === 'top' && (
        <p className="overflow-hidden py-2 text-center text-xs text-muted" style={{ height: Math.min(edge.pull.px, 60) }}>
          {edge.ready ? `놓으면 ${fmtDate(addDays(date, 1))}` : '더 당기면 다음 날'}
        </p>
      )}
      <ul className="px-4 pb-2">
        {day.isPending && <li><SkeletonList rows={3} /></li>}
        {day.data?.logs.length === 0 && <li className="py-8 text-center text-sm text-faint">이 날 기록이 없어요</li>}
        {day.data?.logs.map((l) => {
          const k = kindOf(l.kind)
          return (
            <li key={l.id} className="border-b border-line last:border-0">
              <button onClick={() => setEditing(l)} className="flex w-full items-center gap-3 py-2.5 text-left">
                <span className="w-14 shrink-0 text-sm tabular-nums text-ink-2">
                  {l.at.slice(11)}
                  {l.minutes !== null && l.minutes > 0 && (
                    <span className="block text-[11px] text-faint">~{addMinutes(l.at.slice(11), l.minutes)}</span>
                  )}
                </span>
                <span className={`h-2.5 w-2.5 shrink-0 rounded-full ${TONE[l.kind] ?? 'bg-line'}`} />
                <span className="min-w-0 flex-1">
                  <span className="text-sm font-semibold">{k?.label ?? l.kind}</span>
                  {l.detail && <span className="ml-1.5 text-sm text-muted">{l.detail}</span>}
                  <span className="block text-xs text-faint">
                    {[l.amount_ml !== null ? `${l.amount_ml}ml` : '',
                      k?.duration ? (l.minutes !== null ? fmtMin(l.minutes) : ongoing(l) ? '진행 중' : '길이 기록 없음') : (l.minutes !== null ? fmtMin(l.minutes) : ''),
                      l.note].filter(Boolean).join(' · ')}
                  </span>
                </span>
                {k?.duration && ongoing(l) && (
                  <span
                    role="button"
                    onClick={(ev) => { ev.stopPropagation(); patch.mutate({ id: l.id, minutes: Math.max(1, minutesSince(l.at)) }) }}
                    className="shrink-0 rounded-lg bg-accent px-2.5 py-1 text-xs font-semibold text-accent-ink"
                  >
                    끝
                  </span>
                )}
              </button>
            </li>
          )
        })}
      </ul>

      <p className={`px-4 pb-6 pt-2 text-center text-xs transition ${edge.ready && edge.pull?.edge === 'bottom' ? 'font-semibold text-ink' : 'text-faint'}`}>
        {edge.pull?.edge === 'bottom'
          ? (edge.ready ? `놓으면 ${fmtDate(addDays(date, -1))}` : '조금 더…')
          : `↑ 더 밀면 ${fmtDate(addDays(date, -1))}`}
      </p>
      </div>

      <Sheet open={!!editing} onClose={() => setEditing(null)} title={editing ? `${kindOf(editing.kind)?.label ?? ''} 기록` : ''}>
        {editing && kindOf(editing.kind) && (
          <CareForm
            log={editing}
            childID={child?.user_id}
            kind={kindOf(editing.kind)!}
            onSave={(body) => { patch.mutate({ id: editing.id, ...body }); setEditing(null) }}
            onDelete={async () => {
              if (await confirm({ title: '이 기록을 지울까요?', confirmLabel: '지우기', danger: true })) { del.mutate(editing.id); setEditing(null) }
            }}
            onClose={() => setEditing(null)}
          />
        )}
      </Sheet>
    </div>
  )
}

// 길이 없는 기록이 '진행 중'인 건 24시간 안의 것뿐(가져온 잠 중 시작만 찍힌 게 많다).
function ongoing(l: CareLog) {
  return l.minutes === null && minutesSince(l.at) < 24 * 60
}

function latestOf(m: Record<string, CareLog>, keys: string[]): CareLog | undefined {
  let best: CareLog | undefined
  for (const k of keys) {
    const l = m[k]
    if (l && (!best || l.at > best.at)) best = l
  }
  return best
}

function minutesSince(at: string) {
  const [d, t] = at.split('T')
  const [y, mo, da] = d.split('-').map(Number)
  const [h, mi] = t.split(':').map(Number)
  return Math.floor((Date.now() - new Date(y, mo - 1, da, h, mi).getTime()) / 60_000)
}

function fmtMin(n: number) {
  if (n < 60) return `${n}분`
  return n % 60 === 0 ? `${n / 60}시간` : `${Math.floor(n / 60)}시간 ${n % 60}분`
}

function ago(at: string) {
  const m = minutesSince(at)
  if (m < -5) return '시각 확인 필요' // 앞으로의 시각 — 잘못 적힌 기록
  if (m < 1) return '방금'
  if (m < 60) return `${m}분 전`
  // 카드 폭이 1/3 이라 10시간 넘으면 분은 뺀다(한 줄 유지).
  if (m < 10 * 60) return `${Math.floor(m / 60)}시간 ${m % 60}분 전`
  if (m < 24 * 60) return `${Math.floor(m / 60)}시간 전`
  return `${Math.floor(m / 1440)}일 전`
}

function LastCard({ label, log, sub }: { label: string; log?: CareLog; sub: (l: CareLog) => string }) {
  return (
    <div className="min-w-0 rounded-2xl bg-surface px-1.5 py-3 text-center shadow-sm">
      <p className="text-[11px] text-faint">{label}</p>
      {log ? (
        <>
          <p className="mt-0.5 whitespace-nowrap text-sm font-bold tracking-tight">{ago(log.at)}</p>
          <p className="text-[11px] text-muted">{sub(log)}</p>
        </>
      ) : (
        <p className="mt-0.5 text-sm text-faint">—</p>
      )}
    </div>
  )
}

function CareForm({ log, childID, kind, onSave, onDelete, onClose }: {
  log: CareLog
  childID?: number
  kind: CareKind
  onSave: (body: Record<string, unknown>) => void
  onDelete: () => void
  onClose: () => void
}) {
  const [day, setDay] = useState(log.at.slice(0, 10))
  const [time, setTime] = useState(log.at.slice(11))
  // 미래 시각이 되면 어제로 옮긴다(자정 넘어 어젯밤 일을 적는 경우). 서버도 미래 시각을 거부한다.
  const setTimeSmart = (t: string) => {
    setTime(t)
    const nowStr = toDateTimeStr(new Date())
    if (`${day}T${t}` > nowStr && day === today()) setDay(addDays(day, -1))
    else if (`${day}T${t}` <= nowStr && day === addDays(today(), -1) && log.at.slice(0, 10) === today() && t <= nowStr.slice(11)) setDay(today())
  }
  // 끝이 시작보다 이르면 자정을 넘긴 것으로 본다.
  const [end, setEnd] = useState(log.minutes !== null ? addMinutes(log.at.slice(11), log.minutes) : '')
  const timed = kind.duration || kind.amount
  const minutes = end ? spanMinutes(time, end) : null
  const [amount, setAmount] = useState(log.amount_ml?.toString() ?? '')
  const [detail, setDetail] = useState(log.detail ?? '')
  const [note, setNote] = useState(log.note)
  const solids = kind.key === 'solids'
  const [items, setItems] = useState<string[]>(log.items.map((i) => i.name))
  const [mealID, setMealID] = useState<number | null>(log.meal_id)
  const num = (s: string) => (s.trim() === '' ? -1 : Math.max(0, Math.round(Number(s))))

  return (
    <form
      onSubmit={(e) => {
        e.preventDefault()
        onSave(solids
          ? { at: `${day}T${time}`, minutes: minutes ?? -1, amount_ml: num(amount), note, ingredients: items, meal_id: mealID ?? 0 }
          : { at: `${day}T${time}`, minutes: minutes ?? -1, amount_ml: num(amount), detail, note })
      }}
      className="space-y-3"
    >
      <Field label={timed ? '시작' : '시각'}>
        <TimeField value={time} onChange={setTimeSmart} />
        <div className="mt-1.5 flex items-center gap-1.5">
          {[{ d: addDays(today(), -1), l: '어제' }, { d: today(), l: '오늘' }].map((o) => (
            <button key={o.l} type="button" onClick={() => setDay(o.d)}
              className={`rounded-lg px-2.5 py-1 text-xs font-medium ${day === o.d ? 'bg-accent text-accent-ink' : 'bg-surface-2 text-muted'}`}>
              {o.l}
            </button>
          ))}
          <input type="date" value={day} max={today()} onChange={(e) => e.target.value && setDay(e.target.value)}
            className="rounded-lg border border-line bg-surface px-2 py-1 text-xs" />
        </div>
      </Field>
      {timed && (
        <Field label="끝">
          {end ? (
            <>
              <TimeField value={end} fallback={time} onChange={setEnd} />
              <p className="mt-1 flex items-center gap-2 text-xs text-muted">
                <span>{time} ~ {end}{minutes !== null && spanCrossesMidnight(time, end) && ' (다음 날)'} · <b className="text-ink">{fmtMin(minutes ?? 0)}</b></span>
                <button type="button" onClick={() => setEnd('')} className="text-faint underline">끝 지우기</button>
              </p>
            </>
          ) : (
            <div className="flex flex-wrap gap-1.5">
              <button type="button" onClick={() => setEnd(nowHM())} className="rounded-lg bg-surface-2 px-2.5 py-1.5 text-xs font-medium text-ink-2">지금</button>
              {[10, 20, 30, 60].map((m) => (
                <button key={m} type="button" onClick={() => setEnd(addMinutes(time, m))} className="rounded-lg bg-surface-2 px-2.5 py-1.5 text-xs font-medium text-muted">
                  +{m < 60 ? `${m}분` : '1시간'}
                </button>
              ))}
              <span className="self-center text-[11px] text-faint">{kind.duration ? '비우면 진행 중' : '안 적어도 돼요'}</span>
            </div>
          )}
        </Field>
      )}
      {kind.details && kind.details.length > 0 && (
        <Field label="상세">
          <div className="flex gap-1.5">
            {kind.details.map((d) => (
              <button key={d} type="button" onClick={() => setDetail(detail === d ? '' : d)}
                className={`flex-1 rounded-lg py-2 text-sm font-medium ${detail === d ? 'bg-accent text-accent-ink' : 'bg-surface-2 text-muted'}`}>
                {d}
              </button>
            ))}
          </div>
        </Field>
      )}
      {solids && (
        <SolidsPicker
          childID={childID}
          date={day}
          items={items}
          mealID={mealID}
          onItems={setItems}
          onMeal={(id, names) => { setMealID(id); if (names) setItems(names) }}
        />
      )}
      {kind.free && !solids && (
        <Field label={kind.key === 'medicine' ? '약' : kind.key === 'temp' ? '체온' : '내용'}>
          <Input value={detail} onChange={(e) => setDetail(e.target.value)} placeholder={kind.key === 'solids' ? '쌀미음, 소고기…' : ''} />
        </Field>
      )}
      <div className="flex gap-2">
        {kind.amount && (
          <div className="flex-1"><Field label="양 (ml)"><Input inputMode="numeric" value={amount} onChange={(e) => setAmount(e.target.value)} /></Field></div>
        )}
      </div>
      <Field label="메모"><Input value={note} onChange={(e) => setNote(e.target.value)} placeholder="없으면 비워두세요" /></Field>
      <div className="flex items-center justify-between pt-1">
        <Button type="button" variant="ghost" onClick={onDelete} className="text-rose-500">지우기</Button>
        <div className="flex gap-2">
          <Button type="button" variant="ghost" onClick={onClose}>취소</Button>
          <Button type="submit">저장</Button>
        </div>
      </div>
    </form>
  )
}

/** 이유식 재료는 식단 재료 목록에서만 고른다. 끼니를 누르면 그 재료가 담기고 끼니에 연결된다. */
function SolidsPicker({ childID, date, items, mealID, onItems, onMeal }: {
  childID?: number
  date: string
  items: string[]
  mealID: number | null
  onItems: (v: string[]) => void
  onMeal: (id: number | null, names?: string[]) => void
}) {
  const foods = useBFFoods(childID)
  const plan = useBabyfood(date, date, childID)
  const meals = plan.data?.days?.[0]?.meals ?? []
  return (
    <>
      {meals.length > 0 && (
        <Field label="그날 식단에서">
          <div className="space-y-1.5">
            {meals.map((m) => {
              const names = [m.base, ...m.toppings].filter(Boolean)
              const on = mealID === m.id
              return (
                <button key={m.id} type="button"
                  onClick={() => (on ? onMeal(null) : onMeal(m.id, names))}
                  className={`w-full rounded-xl p-2.5 text-left ${on ? 'bg-emerald-50 ring-1 ring-emerald-500' : 'bg-surface-2'}`}>
                  <p className="text-xs font-semibold">{m.title} {m.at}{on && <span className="ml-1.5 text-emerald-700">✓ 이 끼니</span>}</p>
                  <p className="truncate text-xs text-muted">{names.join(', ')}</p>
                </button>
              )
            })}
          </div>
        </Field>
      )}
      <Field label="먹인 재료">
        <FoodPicker value={items} foods={foods.data ?? []} multi prefer="cube" src={[]} onChange={onItems} />
      </Field>
    </>
  )
}

/** 하루 합계: 총량(분유+이유식)과 각각을 점과 같은 색으로, 잠은 밤잠·낮잠. */
function DaySummary({ d }: { d: CareDay }) {
  const total = d.formula_ml + d.solids_ml
  return (
    <div className="flex flex-wrap items-baseline justify-end gap-x-3 gap-y-1 px-4 pt-1.5 text-xs text-muted">
      {total > 0 && (
        <span className="tabular-nums">
          <b className="text-ink">{total}ml</b>
          {d.formula_ml > 0 && d.solids_ml > 0 && (
            <>
              (<b className="text-amber-500">{d.formula_ml}</b>+<b className="text-teal-500">{d.solids_ml}</b>)
            </>
          )}
          {!(d.formula_ml > 0 && d.solids_ml > 0) && (
            <span className={d.formula_ml > 0 ? 'text-amber-500' : 'text-teal-500'}> {d.formula_ml > 0 ? '분유' : '이유식'}</span>
          )}
        </span>
      )}
      {d.breast_min > 0 && <span>모유 <b className="text-rose-400">{fmtMin(d.breast_min)}</b></span>}
      {d.night_min > 0 && (
        <span className="text-indigo-500" title="밤잠">🌙 <b>{fmtHM(d.night_min)}</b></span>
      )}
      {d.nap_min > 0 && (
        <span className="text-indigo-500" title="낮잠">☀︎ <b>{fmtHM(d.nap_min)}</b></span>
      )}
      {d.diapers > 0 && <span>기저귀 <b className="text-stone-500">{d.diapers}</b></span>}
    </div>
  )
}

function fmtHM(n: number) {
  const h = Math.floor(n / 60), m = n % 60
  if (h === 0) return `${m}분`
  return m === 0 ? `${h}시간` : `${h}시간 ${m}분`
}

function addMinutes(hm: string, m: number) {
  const [h, mi] = hm.split(':').map(Number)
  const t = ((h * 60 + mi + m) % 1440 + 1440) % 1440
  return `${String(Math.floor(t / 60)).padStart(2, '0')}:${String(t % 60).padStart(2, '0')}`
}
/** 끝이 시작보다 이르면 다음 날로 본다(같으면 0분). */
function spanMinutes(start: string, end: string) {
  const toM = (x: string) => { const [h, m] = x.split(':').map(Number); return h * 60 + m }
  const d = toM(end) - toM(start)
  return d >= 0 ? d : d + 1440
}
function spanCrossesMidnight(start: string, end: string) {
  return end < start
}
function nowHM() {
  const d = new Date()
  return `${String(d.getHours()).padStart(2, '0')}:${String(d.getMinutes()).padStart(2, '0')}`
}
