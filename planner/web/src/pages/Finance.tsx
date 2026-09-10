import { useRef, useState } from 'react'
import { api, importFinance, type FinFixed, type FinIncome, type FinMonth, type FinFormat, type FinImportResult, type FinItem, type FinOverview } from '../api'
import { useFinance, useFinFormat, useInvalidating, useMe, useUsers } from '../lib/hooks'
import { fmtWon, fmtWonShort } from '../lib/date'
import { Button, Field, Input, PageHeader, Sheet, SkeletonList } from '../components/ui'
import { TAG_STYLE, TagChip, TagChips, TagPicker } from '../components/FinTags'
import { FinItemSheet, type FinItemTarget } from '../components/FinItemSheet'

// 재정. 가계부 앱에서 내보낸 파일을 사람별로 올려 쌓고, 합쳐서도 각자로도 본다. 어느 앱인지·규격은 서버 설정(finance-format.json).
const GROUP_LABEL: Record<string, string> = {
  liquid: '바로 쓸 수 있는 돈', saving: '저축', invest: '투자', pension: '연금',
  insurance: '보험(납입액)', other: '기타 자산', debt: '부채',
}
const GROUP_ORDER = ['liquid', 'saving', 'invest', 'pension', 'insurance', 'other', 'debt']

export default function Finance() {
  const me = useMe()
  const users = useUsers()
  const [owner, setOwner] = useState(0)
  const [month, setMonth] = useState<string | undefined>(undefined)
  const [uploading, setUploading] = useState<number | null>(null)
  const fmt = useFinFormat().data
  const [tagging, setTagging] = useState<{ keys: string[]; label: string } | null>(null)
  const [tagFilter, setTagFilter] = useState<number | null>(null)
  const [detail, setDetail] = useState<FinItemTarget | null>(null)
  const q = useFinance(owner, month)
  const d = q.data
  const people = (users.data ?? []).filter((u) => u.name !== '단아')
  const name = (id: number) => users.data?.find((u) => u.id === id)?.name ?? ''

  return (
    <div className="mx-auto max-w-lg pb-6">
      <PageHeader title="재정" right={<Button variant="ghost" onClick={() => setUploading(owner)}>+ 파일</Button>} />

      <div className="flex gap-1.5 px-4 pt-2">
        {[{ id: 0, name: '우리집' }, ...people].map((u) => (
          <button key={u.id} onClick={() => setOwner(u.id)}
            className={`rounded-lg px-3 py-1.5 text-sm font-medium ${owner === u.id ? 'bg-accent text-accent-ink' : 'bg-surface-2 text-muted'}`}>
            {u.name}
          </button>
        ))}
      </div>

      {q.isPending && <div className="px-4 pt-4"><SkeletonList rows={4} /></div>}
      {d && d.owners.length === 0 && (
        <EmptyOwner who={owner ? name(owner) : null} fmt={fmt} onUpload={() => setUploading(owner)} />
      )}
      {d && d.owners.length > 0 && (
        <>
          <UploadStatus d={d} people={owner ? people.filter((u) => u.id === owner) : people} onUpload={(id) => setUploading(id)} />
          <NetWorth d={d} name={name} />
          <Plan d={d} />
          <Flow d={d} />
          <FixedIncome d={d} onOpen={setDetail} filter={tagFilter} onTag={setTagging} onClearFilter={() => setTagFilter(null)} />
          <UnexpectedIncome d={d} onOpen={setDetail} onMonth={setMonth} filter={tagFilter} onTag={setTagging} onClearFilter={() => setTagFilter(null)} />
          <TagSpend d={d} filter={tagFilter} onFilter={setTagFilter} />
          <Fixed d={d} onOpen={setDetail} filter={tagFilter} onTag={setTagging} onClearFilter={() => setTagFilter(null)} />
          <Unexpected d={d} onOpen={setDetail} onMonth={setMonth} filter={tagFilter} onTag={setTagging} onClearFilter={() => setTagFilter(null)} />
          <Assets items={d.items} name={name} many={d.owners.length > 1} />
        </>
      )}

      {d && <FinItemSheet target={detail} owner={owner} d={d} onClose={() => setDetail(null)} onTag={setTagging} />}
      {d && <TagPicker target={tagging} tags={d.tags} links={d.tag_links} onClose={() => setTagging(null)} />}
      <UploadSheet open={uploading !== null} onClose={() => setUploading(null)} people={people} fmt={fmt}
        defaultOwner={uploading || me.data?.id || 0} />
    </div>
  )
}

function EmptyOwner({ who, fmt, onUpload }: { who: string | null; fmt?: FinFormat; onUpload: () => void }) {
  return (
    <div className="mx-4 mt-6 rounded-2xl border border-dashed border-line px-4 py-8 text-center">
      <p className="text-sm font-semibold text-ink-2">{who ? `${who}님의 재정은 아직 비어 있어요` : '아직 올린 파일이 없어요'}</p>
      <p className="mt-1 text-xs text-muted">
        {fmt?.configured ? `${fmt.source_name}에서 받은 파일을 올리면 자산·지출이 채워져요.` : '파일 규격이 아직 설정되지 않아 올릴 수 없어요.'}
      </p>
      {fmt?.configured && <Button className="mt-4" onClick={onUpload}>{who ? `${who} 파일 올리기` : '파일 올리기'}</Button>}
    </div>
  )
}

// 사람별 마지막 업로드. 올린 적 없거나 주기가 지난 사람만 알린다.
function UploadStatus({ d, people, onUpload }: { d: FinOverview; people: { id: number; name: string }[]; onUpload: (id: number) => void }) {
  const now = Date.now() / 1000
  const rows = people.map((u) => {
    const at = d.uploaded?.[u.id]
    const days = at ? Math.floor((now - at) / 86400) : null
    return { ...u, days }
  }).filter((r) => r.days === null || r.days >= d.upload_days)
  if (rows.length === 0) return null
  return (
    <div className="mx-4 mt-3 space-y-1.5">
      {rows.map((r) => (
        <button key={r.id} onClick={() => onUpload(r.id)}
          className="flex w-full items-center justify-between rounded-xl bg-amber-50 px-3 py-2 text-left text-xs text-amber-800">
          <span>{r.days === null ? `${r.name}님은 아직 올린 파일이 없어요 — 합계에 빠져 있어요` : `${r.name}님이 올린 지 ${r.days}일 지났어요`}</span>
          <span className="shrink-0 font-semibold underline">올리기</span>
        </button>
      ))}
    </div>
  )
}

function Chevron({ open }: { open: boolean }) {
  return (
    <svg viewBox="0 0 24 24" className={`h-4 w-4 shrink-0 text-ghost transition-transform ${open ? 'rotate-180' : ''}`} fill="none" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round">
      <path d="M6 9l6 6 6-6" />
    </svg>
  )
}

// 제목을 누르면 접고 편다. 접힌 상태는 기기마다 기억한다. 접혔을 때 right 대신 summary(있으면)를 보인다.
function Card({ id, title, children, right, summary }: { id?: string; title: string; children: React.ReactNode; right?: React.ReactNode; summary?: React.ReactNode }) {
  const storeKey = `fin.card.${id ?? title}`
  const [open, setOpen] = useState(() => { try { return localStorage.getItem(storeKey) !== '0' } catch { return true } })
  const toggle = () => {
    const v = !open
    setOpen(v)
    try { if (v) localStorage.removeItem(storeKey); else localStorage.setItem(storeKey, '0') } catch { /* 저장 못 해도 동작은 한다 */ }
  }
  return (
    <section className="mx-4 mt-3 rounded-2xl bg-surface p-4 shadow-sm">
      <div className={`flex items-center justify-between gap-2 ${open ? 'mb-2' : ''}`}>
        <button type="button" onClick={toggle} aria-expanded={open} className="-m-1 flex min-w-0 flex-1 items-center gap-1 rounded-lg p-1 text-left">
          <h2 className="text-sm font-bold text-ink-2">{title}</h2>
          <Chevron open={open} />
        </button>
        {!open && summary !== undefined ? summary : right}
      </div>
      {open && children}
    </section>
  )
}

function NetWorth({ d, name }: { d: FinOverview; name: (id: number) => string }) {
  const hist = d.history ?? []
  const max = Math.max(1, ...hist.map((h) => Math.abs(h.net_worth)))
  return (
    <Card title="순자산" summary={<span className="text-sm font-bold tabular-nums">{fmtWon(d.net_worth)}</span>} right={<span className="text-[11px] text-faint">{d.owners.map((o) => `${name(o)} ${d.as_of[o]?.slice(5).replace('-', '/')}`).join(' · ')} 기준</span>}>
      <p className="text-2xl font-bold tabular-nums">{fmtWon(d.net_worth)}</p>
      <p className="mt-0.5 text-xs text-muted">자산 {fmtWon(d.total_asset)} · 부채 {fmtWon(d.total_debt)}</p>
      {hist.length > 1 && (
        <div className="mt-3 flex h-16 items-end gap-1">
          {hist.map((h) => (
            <div key={h.month} className="flex flex-1 flex-col items-center gap-1">
              <div className="w-full rounded-t bg-accent/70" style={{ height: `${Math.max(4, (Math.abs(h.net_worth) / max) * 48)}px` }} />
              <span className="text-[9px] text-faint">{h.month.slice(2).replace('-', '.')}</span>
            </div>
          ))}
        </div>
      )}
    </Card>
  )
}

/** 금액 줄. 누르면 어떤 항목들인지 펼친다(기본은 닫힘). */
function ExpandRow({ label, value, items }: { label: string; value: number; items: FixedRow[] }) {
  const [open, setOpen] = useState(false)
  if (value <= 0 && items.length === 0) return null
  return (
    <div>
      <button type="button" onClick={() => setOpen(!open)} className="-mx-1 flex w-full items-center justify-between rounded-lg px-1 text-sm">
        <span className="flex items-center gap-1"><span className="mr-0.5 text-faint">−</span>{label}<span className="text-[11px] text-faint">{items.length}</span><Chevron open={open} /></span>
        <span className="tabular-nums">{fmtWon(value)}</span>
      </button>
      {open && (
        <ul className="mb-1 ml-3 mt-0.5 space-y-0.5 border-l border-line pl-2">
          {items.map((f) => (
            <li key={f.label} className="flex justify-between gap-2 text-xs text-muted">
              <span className="min-w-0 truncate">{f.label}</span>
              <span className="shrink-0 tabular-nums">{fmtWon(f.monthly)}</span>
            </li>
          ))}
        </ul>
      )}
    </div>
  )
}

/** 남겨둘 생활비 줄. 누르면 근거(최근 6개월 월별 쓴 돈)를 펼친다. */
function BufferBasis({ d }: { d: FinOverview }) {
  const p = d.plan
  const [open, setOpen] = useState(false)
  const six = d.months.filter((m) => !m.partial).slice(-6)
  return (
    <div>
      <button type="button" onClick={() => setOpen(!open)} className="-mx-1 flex w-full items-center justify-between rounded-lg px-1 text-left">
        <span className="flex items-center gap-1"><span className="mr-0.5 text-faint">−</span>남겨둘 생활비 {p.buffer_months}개월 <span className="text-faint">(평소 한 달 {fmtWon(p.spend_avg)})</span><Chevron open={open} /></span>
        <span className="shrink-0 tabular-nums">{fmtWon(p.spend_avg * p.buffer_months)}</span>
      </button>
      {open && (
        <div className="mb-1 ml-3 mt-1 border-l border-line pl-2">
          <div className="grid grid-cols-[2.6rem_1fr_1fr_1fr] gap-x-2 text-[11px] text-faint">
            <span>달</span><span className="text-right">쓴 돈</span><span className="text-right">− 예상 외</span><span className="text-right">= 평소</span>
          </div>
          {six.map((m) => (
            <div key={m.month} className="grid grid-cols-[2.6rem_1fr_1fr_1fr] gap-x-2 text-[11px] tabular-nums">
              <span className="text-muted">{m.month.slice(2).replace('-', '.')}</span>
              <span className="text-right text-faint">{fmtWonShort(m.spend)}</span>
              <span className="text-right text-faint">{m.unexpected ? fmtWonShort(m.unexpected) : '-'}</span>
              <span className="text-right">{fmtWon(m.spend - m.unexpected)}</span>
            </div>
          ))}
          <div className="mt-0.5 grid grid-cols-[2.6rem_1fr_1fr_1fr] gap-x-2 border-t border-line pt-0.5 text-[11px] font-semibold tabular-nums">
            <span>평균</span><span /><span /><span className="text-right">{fmtWon(p.spend_avg)}</span>
          </div>
          <p className="mt-1 text-[10px] text-faint">평소 생활비 평균 × {p.buffer_months}개월 = {fmtWon(p.spend_avg * p.buffer_months)}. 가전 같은 예상 외 지출은 다시 안 쓸 돈이라 빼요. 쓴 돈엔 적금·보험, 내 계좌끼리 옮긴 돈, 이미 센 카드값, '지출 아님'으로 뺀 건 원래 들어가지 않아요.</p>
        </div>
      )}
    </div>
  )
}

function Plan({ d }: { d: FinOverview }) {
  const p = d.plan
  const [editing, setEditing] = useState(false)
  const [income, setIncome] = useState('')
  const [buffer, setBuffer] = useState(String(p.buffer_months))
  const save = useInvalidating((b: Record<string, number>) => api.post('/api/finance/settings', b), [['finance']])
  const base = p.income_base || p.income_hint
  const row = (label: string, v: number, sign = '−', muted = false) => (
    <div className={`flex justify-between text-sm ${muted ? 'text-muted' : ''}`}>
      <span>{sign !== '' && <span className="mr-1 text-faint">{sign}</span>}{label}</span>
      <span className="tabular-nums">{fmtWon(v)}</span>
    </div>
  )
  return (
    <Card title="한 달에 쓸 수 있는 돈" summary={<span className={`text-sm font-bold tabular-nums ${p.spendable < 0 ? 'text-rose-500' : ''}`}>{fmtWon(p.spendable)}</span>} right={<button onClick={() => { setIncome(String(p.income_base || '')); setEditing(true) }} className="text-xs text-muted underline">기준 바꾸기</button>}>
      <p className={`text-2xl font-bold tabular-nums ${p.spendable < 0 ? 'text-rose-500' : ''}`}>{fmtWon(p.spendable)}</p>
      <p className="mt-0.5 text-xs text-muted">최근 3개월 변동지출 평균(예상 외 뺌) {fmtWon(p.variable_avg)}{p.variable_avg > p.spendable && p.spendable > 0 && ' — 기준보다 더 쓰고 있어요'}</p>
      <div className="mt-3 space-y-1 border-t border-line pt-2">
        {row(p.income_base ? '월 수입(정한 값)' : '월 수입(고정수입 한 달 보통)', base, '')}
        <ExpandRow label="고정지출" value={p.fixed_spend} items={mergeFixed(d.fixed.filter((f) => f.fixed && f.kind === 'spend'))} />
        <ExpandRow label="적금·보험 등" value={p.fixed_save} items={mergeFixed(d.fixed.filter((f) => f.fixed && f.kind === 'save'))} />
        {p.goals_monthly > 0 && row('목표 적립', p.goals_monthly)}
      </div>
      {!p.income_base && <p className="mt-2 text-[11px] text-faint">적금·보험·청약은 쓴 돈은 아니지만 여기서 빼요. MMF·파킹통장은 비상금이라 수입·지출 어디에도 넣지 않아요. 수입이 달마다 들쭉날쭉하면 '기준 바꾸기'에서 한 달 수입을 정해두세요.</p>}
      <div className="mt-3 rounded-xl bg-surface-2 p-3 text-sm">
        <p className={`font-semibold ${p.free < 0 ? 'text-rose-500' : ''}`}>여유자금 {fmtWon(p.free)}</p>
        <p className="mt-0.5 text-[11px] text-faint">당장 꺼내 쓸 수 있는 돈 중, 비상금으로 남겨둘 생활비를 빼고도 남는 돈</p>
        <div className="mt-2 space-y-0.5 text-xs text-muted">
          <div className="flex justify-between"><span>입출금·현금·페이 잔액</span><span className="tabular-nums">{fmtWon(p.liquid)}</span></div>
          <div className="flex justify-between"><span><span className="mr-1 text-faint">+</span>비상금(MMF·파킹)</span><span className="tabular-nums">{fmtWon(p.emergency)}</span></div>
          <BufferBasis d={d} />
        </div>
        <p className="mt-1.5 text-[11px] text-faint">평소 한 달 생활비는 최근 6개월에 쓴 돈에서 예상 외 지출을 뺀 평균이에요(위 줄을 누르면 월별 근거). 몇 개월치를 남길지는 '기준 바꾸기'에서 정해요. 적금·청약·정기예탁·주식은 바로 꺼내 쓰는 돈이 아니라 넣지 않아요.</p>
      </div>
      <Sheet open={editing} onClose={() => setEditing(false)} title="기준">
        <form className="space-y-3" onSubmit={async (e) => {
          e.preventDefault()
          await save.mutateAsync({ income: Math.max(0, Number(income) || 0), buffer_months: Math.max(0, Math.min(24, Number(buffer) || 0)) })
          setEditing(false)
        }}>
          <Field label="한 달 수입 (원) — 비우면 고정수입 한 달 보통">
            <Input inputMode="numeric" value={income} onChange={(e) => setIncome(e.target.value.replace(/[^0-9]/g, ''))} placeholder={String(p.income_hint)} />
          </Field>
          <Field label="비상금으로 남겨둘 생활비 (개월)">
            <Input inputMode="numeric" value={buffer} onChange={(e) => setBuffer(e.target.value.replace(/[^0-9]/g, ''))} />
          </Field>
          <div className="flex justify-end gap-2"><Button type="button" variant="ghost" onClick={() => setEditing(false)}>취소</Button><Button type="submit">저장</Button></div>
        </form>
      </Sheet>
    </Card>
  )
}

function Flow({ d }: { d: FinOverview }) {
  const ms = d.months
  const max = Math.max(1, ...ms.map((m) => Math.max(m.in, m.spend)))
  return (
    <Card title="월별 흐름" right={<span className="flex gap-2 text-[10px] text-faint"><i className="inline-block h-2 w-2 rounded-sm bg-emerald-500" />수입<i className="inline-block h-2 w-2 rounded-sm bg-rose-400" />쓴 돈<i className="inline-block h-2 w-2 rounded-sm bg-rose-200" />그중 고정</span>}>
      <div className="flex h-28 items-end gap-1">
        {ms.map((m) => (
          <div key={m.month} className={`flex flex-1 flex-col items-center gap-1 ${m.partial ? 'opacity-50' : ''}`}>
            <div className="flex w-full items-end justify-center gap-px" style={{ height: 96 }}>
              <div className="w-1/2 rounded-t bg-emerald-500" style={{ height: `${(m.in / max) * 96}px` }} />
              <div className="flex w-1/2 flex-col justify-end" style={{ height: `${(m.spend / max) * 96}px` }}>
                <div className="flex-1 rounded-t bg-rose-400" />
                <div className="bg-rose-200" style={{ height: `${m.spend ? (m.fixed / m.spend) * 100 : 0}%` }} />
              </div>
            </div>
            <span className="text-[9px] text-faint">{Number(m.month.slice(5))}</span>
          </div>
        ))}
      </div>
      <p className="mt-2 text-[11px] text-faint">수입은 수입원에서 켠 것만, 내 계좌끼리·가족과 옮긴 돈은 빼고 셌어요. 흐린 막대는 이번 달(아직 안 끝남).</p>
    </Card>
  )
}

// 고정·수입 판정 뒤집기는 key 마다. 자동 판정과 같아지면 뒤집은 기록을 지운다(null).
type Pair = [string, boolean | null]
const pairsFor = (keys: string[], want: boolean, auto: boolean): Pair[] => keys.map((k) => [k, want === auto ? null : want])
function useSetFixed() {
  return useInvalidating((pairs: Pair[]) => Promise.all(pairs.map(([key, fixed]) => api.post('/api/finance/fixed', { key, fixed }))), [['finance']])
}
// '지출 아님': spend=false 면 쓴 돈에서 뺀다.
function useSetSpend() {
  return useInvalidating((b: { keys: string[]; spend: boolean }) => Promise.all(b.keys.map((key) => api.post('/api/finance/spend', { key, spend: b.spend }))), [['finance']])
}
const NotBtn = ({ children, onClick }: { children: React.ReactNode; onClick: () => void }) => (
  <button type="button" onClick={onClick} className="text-[11px] text-faint underline hover:text-muted">{children}</button>
)
function useSetIncome() {
  return useInvalidating((pairs: Pair[]) => Promise.all(pairs.map(([key, income]) => api.post('/api/finance/income', { key, income }))), [['finance']])
}

// 제목이 같은 줄은 하나로 묶어 금액을 더한다. members 는 버튼이 적용될 원래 항목들.
const uniq = <T,>(xs: T[]) => [...new Set(xs)]
function byTitle<T>(xs: T[], title: (x: T) => string): T[][] {
  const m = new Map<string, T[]>()
  for (const x of xs) {
    const k = title(x)
    m.set(k, [...(m.get(k) ?? []), x])
  }
  return [...m.values()]
}
type IncomeRow = FinIncome & { members: FinIncome[] }
function mergeIncome(xs: FinIncome[]): IncomeRow[] {
  return byTitle(xs, (x) => x.label).map((g) => ({
    ...g[0], members: g, keys: uniq(g.flatMap((x) => x.keys)),
    monthly: g.reduce((a, x) => a + x.monthly, 0), total: g.reduce((a, x) => a + x.total, 0),
    months: Math.max(...g.map((x) => x.months)), months6: Math.max(...g.map((x) => x.months6)),
    edited: g.some((x) => x.edited), fixed_override: g.find((x) => x.fixed_override !== null)?.fixed_override ?? null,
  }))
}
type FixedRow = FinFixed & { keys: string[]; members: FinFixed[] }
function mergeFixed(xs: FinFixed[]): FixedRow[] {
  return byTitle(xs, (x) => x.label).map((g) => ({
    ...g[0], members: g, keys: uniq(g.map((x) => x.key)),
    monthly: g.reduce((a, x) => a + x.monthly, 0), months: Math.max(...g.map((x) => x.months)),
    edited: g.some((x) => x.edited), override: g.find((x) => x.override !== null)?.override ?? null,
  }))
}
const datesOf = (ats: string[]) => uniq(ats.map((a) => a.slice(5, 10).replace('-', '/'))).sort().join(', ')

const Edited = () => <span className="ml-1 rounded bg-surface-2 px-1 text-[10px] text-muted">수정됨</span>
const Count = ({ n }: { n: number }) => (n > 1 ? <span className="ml-1 text-[11px] font-normal text-faint">{n}건</span> : null)

type TagTarget = { keys: string[]; label: string }
type TagProps = { onOpen: (t: FinItemTarget) => void; filter: number | null; onTag: (t: TagTarget) => void; onClearFilter: () => void }

function FilterNote({ d, filter, onClear }: { d: FinOverview; filter: number | null; onClear: () => void }) {
  const t = d.tags.find((x) => x.id === filter)
  if (!t) return null
  return (
    <button onClick={onClear} className="mb-1.5 flex items-center gap-1 text-[11px] text-muted">
      <TagChip tag={t} /> 태그만 보는 중 <span className="underline">모두 보기</span>
    </button>
  )
}

const hasTag = (d: FinOverview, keys: string[], filter: number | null) => filter === null || keys.some((k) => (d.tag_links[k] ?? []).includes(filter))

/** 목록 아래의 접힌 묶음(고정에서 뺀 것, 수입에서 뺀 입금 등). */
function Tucked({ title, count, children }: { title: string; count: number; children: React.ReactNode }) {
  const [open, setOpen] = useState(false)
  if (count === 0) return null
  return (
    <div className="mt-2 border-t border-line pt-2">
      <button onClick={() => setOpen(!open)} className="flex w-full items-center justify-between text-xs text-muted">
        <span>{title} {count}</span><Chevron open={open} />
      </button>
      {open && <ul className="mt-1 divide-y divide-line">{children}</ul>}
    </div>
  )
}

const MoveBtn = ({ children, onClick }: { children: React.ReactNode; onClick: () => void }) => (
  <button type="button" onClick={onClick} className="shrink-0 rounded-md border border-line px-2 py-1 text-[11px] font-medium text-ink-2 hover:bg-surface-2">{children}</button>
)

function MonthSelect({ d, onMonth }: { d: FinOverview; onMonth: (m: string) => void }) {
  const months = d.months.map((m) => m.month).reverse()
  return (
    <select value={d.month} onChange={(e) => onMonth(e.target.value)} onClick={(e) => e.stopPropagation()} className="rounded-md bg-surface-2 px-2 py-1 text-xs">
      {months.map((m) => <option key={m} value={m}>{m.slice(0, 4)}년 {Number(m.slice(5))}월</option>)}
    </select>
  )
}

// 끝난 달들의 월평균(연간). 이번 달은 아직 안 끝나 뺀다.
const yearAvg = (d: FinOverview, pick: (m: FinMonth) => number) => {
  const done = d.months.filter((m) => !m.partial)
  return done.length ? Math.round(done.reduce((a, m) => a + pick(m), 0) / done.length) : 0
}

/** 최근 12개월 + 이번 달 막대. 금액을 막대 아래에 적는다. */
function MonthBars({ d, pick, color }: { d: FinOverview; pick: (m: FinMonth) => number; color: string }) {
  const max = Math.max(1, ...d.months.map(pick))
  const avg = yearAvg(d, pick)
  return (
    <div className="mt-3 border-t border-line pt-2">
      <div className="mb-1 flex justify-between text-[11px] text-faint"><span>월별</span><span>점선: 월평균 {fmtWonShort(avg)}</span></div>
      <div className="relative flex items-end gap-0.5" style={{ height: 64 }}>
        <div className="pointer-events-none absolute inset-x-0 border-t border-dashed border-ghost" style={{ bottom: `${(avg / max) * 64}px` }} />
        {d.months.map((m) => (
          <div key={m.month} title={`${m.month} ${fmtWon(pick(m))}`} className={`flex-1 rounded-t ${color} ${m.partial ? 'opacity-40' : ''}`} style={{ height: `${Math.max(1, (pick(m) / max) * 64)}px` }} />
        ))}
      </div>
      <div className="mt-0.5 flex gap-0.5">
        {d.months.map((m) => (
          <div key={m.month} className={`flex-1 text-center leading-tight ${m.partial ? 'opacity-50' : ''}`}>
            <p className="text-[9px] text-faint">{Number(m.month.slice(5))}월</p>
            <p className="text-[8px] tabular-nums text-muted">{pick(m) ? fmtWonShort(pick(m)).replace('원', '') : '-'}</p>
          </div>
        ))}
      </div>
    </div>
  )
}

function FixedIncome({ d, onOpen, filter, onTag, onClearFilter }: { d: FinOverview } & TagProps) {
  const setFixed = useSetFixed()
  const on = mergeIncome(d.income.filter((f) => f.income && f.fixed)).filter((f) => hasTag(d, f.keys, filter))
  const move = (f: IncomeRow, want: boolean) => setFixed.mutate(f.members.flatMap((m) => pairsFor(m.keys, want, m.fixed_auto)))
  return (
    <Card id="fixed-income" title="고정수입" right={<span className="text-xs text-muted">연 월평균 {fmtWon(yearAvg(d, (m) => m.fixed_in))}</span>}>
      <FilterNote d={d} filter={filter} onClear={onClearFilter} />
      {on.length === 0 && <p className="text-xs text-faint">최근 6개월에 3달 이상 들어온 수입이 없어요. 예상 외 수입에서 '고정'을 눌러 옮길 수 있어요.</p>}
      <ul className="divide-y divide-line">
        {on.map((f) => (
          <li key={f.label} className="flex items-center gap-2 py-2">
            <div className="min-w-0 flex-1">
              <button type="button" onClick={() => onOpen({ keys: f.keys, label: f.label, cat1: f.cat1 })} className="-mx-1 flex w-full items-center gap-2 rounded-lg px-1 py-0.5 text-left">
                <div className="min-w-0 flex-1">
                  <p className="truncate text-sm">{f.label}{f.edited && <Edited />}</p>
                  <p className="text-[11px] text-faint">{f.cat1 || '미분류'} · 최근 6개월 중 {f.months6}달{f.fixed_override !== null && ' · 직접 정함'}</p>
                </div>
                <span className="shrink-0 text-sm tabular-nums">{fmtWonShort(f.monthly)}</span>
              </button>
              <TagChips ids={d.tag_links[f.keys[0]]} tags={d.tags} onEdit={() => onTag({ keys: f.keys, label: f.label })} />
            </div>
            <MoveBtn onClick={() => move(f, false)}>예상 외</MoveBtn>
          </li>
        ))}
      </ul>
      <MonthBars d={d} pick={(m) => m.fixed_in} color="bg-emerald-500" />
      <p className="mt-2 text-[11px] text-faint">최근 6개월 중 3달 이상 들어온 수입. 월평균은 끝난 12개월 평균, 막대는 달마다 실제로 들어온 고정수입 합이에요(같은 급여가 이름이 달라도 한 번만 세요). 이름이 같은 줄은 합쳐 보여요. '예상 외'를 누르면 예상 외 수입으로 옮겨요.</p>
    </Card>
  )
}

function UnexpectedIncome({ d, onOpen, onMonth, filter, onTag, onClearFilter }: { d: FinOverview; onMonth: (m: string) => void } & TagProps) {
  const setFixed = useSetFixed()
  const setIncome = useSetIncome()
  const byKey = new Map(d.income.map((f) => [f.key, f]))
  const rows = byTitle(d.income_tx.filter((t) => hasTag(d, t.keys, filter)), (t) => t.content).map((g) => ({
    content: g[0].content, cat1: g[0].cat1, count: g.length, dates: datesOf(g.map((t) => t.at)),
    amount: g.reduce((a, t) => a + t.amount, 0), keys: uniq(g.flatMap((t) => t.keys)), edited: g.some((t) => t.edited),
    groups: uniq(g.map((t) => t.group)).map((k) => byKey.get(k)).filter((x): x is FinIncome => !!x),
  }))
  const total = rows.reduce((a, t) => a + t.amount, 0)
  const notIncome = mergeIncome(d.income.filter((f) => !f.income))
  return (
    <Card id="unexpected-income" title="예상 외 수입" right={<MonthSelect d={d} onMonth={onMonth} />}>
      <FilterNote d={d} filter={filter} onClear={onClearFilter} />
      {rows.length === 0 ? (
        <p className="text-xs text-faint">이 달엔 고정수입 말고 들어온 수입이 없어요.</p>
      ) : (
        <>
          <p className="mb-1 text-xs text-muted">{rows.length}개 · {fmtWon(total)}</p>
          <ul className="divide-y divide-line">
            {rows.map((t) => (
              <li key={t.content} className="flex items-center gap-2 py-2">
                <div className="min-w-0 flex-1">
                  <button type="button" onClick={() => onOpen({ keys: t.keys, label: t.content, cat1: t.cat1 })} className="-mx-1 block w-full rounded-lg px-1 py-0.5 text-left">
                    <div className="flex items-baseline gap-2">
                      <span className="min-w-0 flex-1 truncate text-sm">{t.content}<Count n={t.count} />{t.edited && <Edited />}</span>
                      <span className="shrink-0 text-sm font-semibold tabular-nums text-emerald-700">{fmtWon(t.amount)}</span>
                    </div>
                    <p className="truncate text-[11px] text-faint">{t.dates} · {t.cat1 || '미분류'}</p>
                  </button>
                  <TagChips ids={d.tag_links[t.keys[0]]} tags={d.tags} onEdit={() => onTag({ keys: t.keys, label: t.content })} />
                </div>
                <div className="flex shrink-0 flex-col items-end gap-1">
                  <MoveBtn onClick={() => setFixed.mutate(t.groups.flatMap((g) => pairsFor(g.keys, true, g.fixed_auto)))}>고정</MoveBtn>
                  <button type="button" onClick={() => setIncome.mutate(t.groups.flatMap((g) => pairsFor(g.keys, false, g.auto)))} className="text-[11px] text-faint underline">수입 아님</button>
                </div>
              </li>
            ))}
          </ul>
        </>
      )}
      <Tucked title="수입에서 뺀 입금" count={notIncome.length}>
        {notIncome.map((f) => (
          <li key={f.label} className="flex items-center gap-2 py-1.5">
            <button type="button" onClick={() => onOpen({ keys: f.keys, label: f.label, cat1: f.cat1 })} className="min-w-0 flex-1 text-left">
              <p className="truncate text-sm text-muted">{f.label}{f.edited && <Edited />}</p>
              <p className="text-[11px] text-faint">{f.cat1 || '미분류'} · 최근 12개월 {f.months}달 · 합 {fmtWonShort(f.total)}</p>
            </button>
            <MoveBtn onClick={() => setIncome.mutate(f.members.flatMap((m) => pairsFor(m.keys, true, m.auto)))}>수입으로</MoveBtn>
          </li>
        ))}
      </Tucked>
      <p className="mt-2 text-[11px] text-faint">이름이 같은 건 합쳐서 보여요. 적금·예금 해지, 파킹통장 인출, 내 계좌·가족에게서 옮긴 돈은 내 돈이 돌아온 것이라 수입에서 빼요.</p>
    </Card>
  )
}

function Fixed({ d, onOpen, filter, onTag, onClearFilter }: { d: FinOverview } & TagProps) {
  const setFixed = useSetFixed()
  const setSpend = useSetSpend()
  const fixedOn = mergeFixed(d.fixed.filter((f) => f.fixed)).filter((f) => hasTag(d, f.keys, filter))
  const on = fixedOn.filter((f) => f.kind === 'spend')
  const saves = fixedOn.filter((f) => f.kind === 'save')
  const move = (f: FixedRow, want: boolean) => setFixed.mutate(f.members.flatMap((m) => pairsFor([m.key], want, m.auto)))
  const row = (f: FixedRow) => (
    <li key={f.label} className="flex items-center gap-2 py-2">
      <div className="min-w-0 flex-1">
        <button type="button" onClick={() => onOpen({ keys: f.keys, label: f.label, cat1: f.cat1 })} className="-mx-1 flex w-full items-center gap-2 rounded-lg px-1 py-0.5 text-left">
          <div className="min-w-0 flex-1">
            <p className="truncate text-sm">{f.label}<Count n={f.members.length} />{f.edited && <Edited />}</p>
            <p className="text-[11px] text-faint">{f.cat1 || '미분류'} · 최근 6개월 중 {f.months}달{f.override !== null && ' · 직접 정함'}</p>
          </div>
          <span className="shrink-0 text-sm tabular-nums">{fmtWonShort(f.monthly)}</span>
        </button>
        <TagChips ids={d.tag_links[f.keys[0]]} tags={d.tags} onEdit={() => onTag({ keys: f.keys, label: f.label })} />
      </div>
      <div className="flex shrink-0 flex-col items-end gap-1">
        <MoveBtn onClick={() => move(f, false)}>예상 외</MoveBtn>
        <NotBtn onClick={() => setSpend.mutate({ keys: f.keys, spend: false })}>{f.kind === 'save' ? '저축 아님' : '지출 아님'}</NotBtn>
      </div>
    </li>
  )
  return (
    <Card id="fixed" title="고정지출" right={<span className="text-xs text-muted">연 월평균 {fmtWon(yearAvg(d, (m) => m.fixed + m.fixed_save))}</span>}>
      <FilterNote d={d} filter={filter} onClear={onClearFilter} />
      {fixedOn.length === 0 && <p className="text-xs text-faint">{filter === null ? '최근 6개월에 매달 비슷하게 나간 게 없어요.' : '이 태그가 붙은 고정지출이 없어요.'}</p>}
      <ul className="divide-y divide-line">{on.map(row)}</ul>
      {saves.length > 0 && (
        <div className="mt-2 border-t border-line pt-2">
          <p className="flex justify-between text-xs font-semibold text-ink-2">
            <span>적금·보험 등 <span className="font-normal text-faint">쓴 돈은 아니지만 쓸 돈에서 빼요</span></span>
            <span className="tabular-nums">월 {fmtWon(d.plan.fixed_save)}</span>
          </p>
          <ul className="divide-y divide-line">{saves.map(row)}</ul>
        </div>
      )}
      <MonthBars d={d} pick={(m) => m.fixed + m.fixed_save} color="bg-rose-400" />
      <p className="mt-2 text-[11px] text-faint">월평균은 끝난 12개월, 막대는 달마다 실제로 나간 고정지출(고정 저축 포함) 합이에요. 최근 6개월 중 3달 이상, 금액이 비슷하게(±25%) 나간 것. 관리비·통신·보험은 금액이 달라도 고정으로 봐요. 이름이 같은 건 합쳐서 보여요. '예상 외'를 누르면 고정에서 빠져 예상 외 지출 판정으로 가요.</p>
    </Card>
  )
}

function TagSpend({ d, filter, onFilter }: { d: FinOverview; filter: number | null; onFilter: (id: number | null) => void }) {
  const rows = d.tag_spend
  const max = Math.max(1, ...rows.map((r) => r.amount), d.untagged)
  return (
    <Card id="tag-spend" title="태그별 지출" right={<span className="text-xs text-muted">{Number(d.month.slice(5))}월</span>}>
      {rows.length === 0 ? (
        <p className="text-xs text-faint">아직 태그를 붙인 항목이 없어요. 각 항목의 '+ 태그'로 붙여보세요.</p>
      ) : (
        <ul className="space-y-1.5">
          {rows.map((r) => {
            const t = d.tags.find((x) => x.id === r.tag_id)
            if (!t) return null
            const on = filter === t.id
            return (
              <li key={r.tag_id}>
                <button onClick={() => onFilter(on ? null : t.id)} className={`w-full rounded-lg px-1 py-0.5 text-left ${on ? 'bg-surface-2' : ''}`}>
                  <div className="flex items-baseline justify-between text-sm">
                    <span className="flex items-center gap-1.5"><TagChip tag={t} /><span className="text-[11px] text-faint">{r.count}건</span></span>
                    <span className="tabular-nums">{fmtWon(r.amount)}</span>
                  </div>
                  <div className="mt-0.5 h-1.5 rounded-full bg-surface-2">
                    <div className={`h-1.5 rounded-full ${(TAG_STYLE[t.color] ?? TAG_STYLE.slate).dot}`} style={{ width: `${(r.amount / max) * 100}%` }} />
                  </div>
                </button>
              </li>
            )
          })}
          <li className="flex justify-between px-1 pt-1 text-xs text-faint"><span>태그 없음</span><span className="tabular-nums">{fmtWon(d.untagged)}</span></li>
        </ul>
      )}
      <p className="mt-2 text-[11px] text-faint">누르면 고정·예상 외 목록을 그 태그로 걸러 봐요. 태그를 여러 개 붙인 거래는 각 태그에 모두 세요.</p>
    </Card>
  )
}

function Unexpected({ d, onOpen, onMonth, filter, onTag, onClearFilter }: { d: FinOverview; onMonth: (m: string) => void } & TagProps) {
  const setFixed = useSetFixed()
  const setSpend = useSetSpend()
  const notSpend = byTitle(d.not_spend, (x) => x.label).map((g) => ({
    label: g[0].label, cat1: g[0].cat1, keys: g.map((x) => x.key), total: g.reduce((a, x) => a + x.total, 0),
    count: g.reduce((a, x) => a + x.count, 0), edited: g.some((x) => x.edited),
  }))
  const shown = byTitle(d.unexpected.filter((u) => hasTag(d, [u.key], filter)), (u) => u.content).map((g) => ({
    content: g[0].content, cat1: g[0].cat1, count: g.length, dates: datesOf(g.map((u) => u.at)),
    amount: g.reduce((a, u) => a + u.amount, 0), keys: uniq(g.map((u) => u.key)), edited: g.some((u) => u.edited),
    reason: uniq(g.map((u) => u.reason)).join(' / '),
  })).sort((a, b) => b.amount - a.amount)
  const total = shown.reduce((a, u) => a + u.amount, 0)
  return (
    <Card id="unexpected" title="예상 외 지출" right={<MonthSelect d={d} onMonth={onMonth} />}>
      <FilterNote d={d} filter={filter} onClear={onClearFilter} />
      {shown.length === 0 ? (
        <p className="text-xs text-faint">{d.unexpected.length === 0 ? '평소와 다르게 큰 지출이 없었어요.' : '이 태그가 붙은 예상 외 지출이 없어요.'}</p>
      ) : (
        <>
          <p className="mb-1 text-xs text-muted">{shown.length}개 · {fmtWon(total)}</p>
          <ul className="divide-y divide-line">
            {shown.map((u) => (
              <li key={u.content} className="flex items-center gap-2 py-2">
                <div className="min-w-0 flex-1">
                  <button type="button" onClick={() => onOpen({ keys: u.keys, label: u.content, cat1: u.cat1 })} className="-mx-1 block w-full rounded-lg px-1 py-0.5 text-left">
                    <div className="flex items-baseline gap-2">
                      <span className="min-w-0 flex-1 truncate text-sm">{u.content}<Count n={u.count} />{u.edited && <Edited />}</span>
                      <span className="shrink-0 text-sm font-semibold tabular-nums">{fmtWon(u.amount)}</span>
                    </div>
                    <p className="text-[11px] text-faint">{u.dates} · {u.reason}</p>
                  </button>
                  <TagChips ids={d.tag_links[u.keys[0]]} tags={d.tags} onEdit={() => onTag({ keys: u.keys, label: u.content })} />
                </div>
                <div className="flex shrink-0 flex-col items-end gap-1">
                  <MoveBtn onClick={() => setFixed.mutate(u.keys.map((k): Pair => [k, true]))}>고정</MoveBtn>
                  <NotBtn onClick={() => setSpend.mutate({ keys: u.keys, spend: false })}>지출 아님</NotBtn>
                </div>
              </li>
            ))}
          </ul>
        </>
      )}
      <Tucked title="지출에서 뺀 항목" count={notSpend.length}>
        {notSpend.map((x) => (
          <li key={x.label} className="flex items-center gap-2 py-1.5">
            <button type="button" onClick={() => onOpen({ keys: x.keys, label: x.label, cat1: x.cat1 })} className="min-w-0 flex-1 text-left">
              <p className="truncate text-sm text-muted">{x.label}{x.edited && <Edited />}</p>
              <p className="text-[11px] text-faint">{x.cat1 || '미분류'} · 최근 13개월 {x.count}건 · 합 {fmtWonShort(x.total)}</p>
            </button>
            <MoveBtn onClick={() => setSpend.mutate({ keys: x.keys, spend: true })}>지출로</MoveBtn>
          </li>
        ))}
      </Tucked>
      <p className="mt-2 text-[11px] text-faint">이름이 같은 건 합쳐서 보여요. 내 다른 계좌로 옮긴 돈처럼 쓴 돈이 아니면 '지출 아님'으로 빼요(고정지출·예상 외·월별 흐름 모두에서 빠져요).</p>
    </Card>
  )
}

function Assets({ items: all, name, many }: { items: FinItem[]; name: (id: number) => string; many: boolean }) {
  const [open, setOpen] = useState<string | null>('liquid')
  const items = all.filter((i) => i.amount !== 0)
  return (
    <Card title="자산·부채">
      <ul className="divide-y divide-line">
        {GROUP_ORDER.filter((g) => items.some((i) => i.group === g)).map((g) => {
          const list = items.filter((i) => i.group === g)
          const sum = list.reduce((a, i) => a + i.amount, 0)
          return (
            <li key={g}>
              <button onClick={() => setOpen(open === g ? null : g)} className="flex w-full items-center justify-between py-2.5 text-left">
                <span className="text-sm font-medium">{GROUP_LABEL[g]} <span className="text-[11px] text-faint">{list.length}</span></span>
                <span className={`text-sm font-semibold tabular-nums ${g === 'debt' ? 'text-rose-500' : ''}`}>{fmtWon(sum)}</span>
              </button>
              {open === g && (
                <ul className="pb-2">
                  {list.map((i, k) => (
                    <li key={k} className="flex items-baseline gap-2 py-1 pl-2 text-xs">
                      <span className="min-w-0 flex-1 truncate text-muted">
                        {i.name}
                        {i.institution && <span className="text-faint"> · {i.institution}</span>}
                        {many && <span className="text-faint"> · {name(i.owner_id)}</span>}
                      </span>
                      {i.rate !== null && g === 'invest' && (
                        <span className={`shrink-0 tabular-nums ${i.rate >= 0 ? 'text-rose-500' : 'text-sky-600'}`}>{i.rate >= 0 ? '+' : ''}{i.rate.toFixed(1)}%</span>
                      )}
                      <span className="shrink-0 tabular-nums">{fmtWon(i.amount)}</span>
                    </li>
                  ))}
                </ul>
              )}
            </li>
          )
        })}
      </ul>
    </Card>
  )
}

function UploadSheet({ open, onClose, people, defaultOwner, fmt }: {
  open: boolean
  onClose: () => void
  people: { id: number; name: string }[]
  defaultOwner: number
  fmt?: FinFormat
}) {
  const [owner, setOwner] = useState(0)
  const [file, setFile] = useState<File | null>(null)
  const [preview, setPreview] = useState<FinImportResult | null>(null)
  const [busy, setBusy] = useState(false)
  const [err, setErr] = useState<string | null>(null)
  const [done, setDone] = useState<FinImportResult | null>(null)
  const input = useRef<HTMLInputElement>(null)
  const inv = useInvalidating(async () => null, [['finance']])
  const who = owner || defaultOwner
  const cols = fmt?.tx_header ? Object.entries(fmt.tx_header).sort(([a], [b]) => a.localeCompare(b)) : []

  const reset = () => { setFile(null); setPreview(null); setErr(null); setDone(null); if (input.current) input.current.value = '' }
  const pick = async (f: File | null) => {
    reset()
    if (!f) return
    setFile(f); setBusy(true)
    try { setPreview(await importFinance(f, who, false)) } catch (e) { setErr(e instanceof Error ? e.message : '읽지 못했어요') } finally { setBusy(false) }
  }
  const apply = async () => {
    if (!file) return
    setBusy(true); setErr(null)
    try { setDone(await importFinance(file, who, true)); setPreview(null); inv.mutate(undefined) } catch (e) { setErr(e instanceof Error ? e.message : '넣지 못했어요') } finally { setBusy(false) }
  }

  return (
    <Sheet open={open} onClose={() => { reset(); setOwner(0); onClose() }} title={fmt?.configured ? `${fmt.source_name} 파일 올리기` : '재정 파일 올리기'}>
      {fmt && !fmt.configured ? (
        <p className="text-sm text-muted">파일 규격이 서버에 설정되지 않아 올릴 수 없어요. (data/finance-format.json)</p>
      ) : (
      <div className="space-y-3">
        <div className="space-y-1.5 text-xs text-muted">
          <p><b className="text-ink-2">받는 법</b> {fmt?.how_to}</p>
          <p>파일은 저장하지 않고 숫자만 읽어요. 같은 거래는 다시 올려도 한 번만 들어가요.</p>
          <details>
            <summary className="cursor-pointer text-faint">읽는 규격 보기</summary>
            <ul className="mt-1 space-y-0.5 pl-3 text-faint">
              <li>시트 '{fmt?.summary_sheet}' — 섹션 {fmt?.sections?.map((x) => `'${x}'`).join(', ')}</li>
              <li>시트 '{fmt?.tx_sheet}' — 첫 줄 {cols.map(([c, h]) => `${c}:${h}`).join(' ')}</li>
            </ul>
            <p className="mt-1 text-faint">서비스 쪽 규격이 바뀌면 어느 시트·열이 다른지 알려주고 넣지 않아요.</p>
          </details>
        </div>
        <Field label="누구의 파일인가요">
          <div className="flex gap-1.5">
            {people.map((u) => (
              <button key={u.id} type="button" onClick={() => { setOwner(u.id); reset() }}
                className={`rounded-lg px-3 py-1.5 text-sm font-medium ${who === u.id ? 'bg-accent text-accent-ink' : 'bg-surface-2 text-muted'}`}>{u.name}</button>
            ))}
          </div>
        </Field>
        <input ref={input} type="file" accept=".xlsx,application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
          onChange={(e) => pick(e.target.files?.[0] ?? null)} className="block w-full text-sm" />
        {busy && <p className="text-xs text-muted">읽는 중…</p>}
        {err && <p className="text-xs text-rose-500">{err}</p>}
        {preview && (
          <div className="rounded-xl bg-surface-2 p-3 text-sm">
            <p className="font-semibold">{preview.taken_at} 기준</p>
            <p className="mt-1 text-xs text-muted">자산 {fmtWon(preview.total_asset)} · 부채 {fmtWon(preview.total_debt)} · 항목 {preview.items}개</p>
            <p className="text-xs text-muted">거래 {preview.tx_total.toLocaleString()}건 중 새 거래 <b>{preview.tx_new.toLocaleString()}건</b> ({preview.tx_from.slice(0, 10)} ~ {preview.tx_to.slice(0, 10)})</p>
            {preview.replace_snapshot && <p className="text-xs text-amber-700">같은 날짜 기록이 있어 이걸로 바꿔요.</p>}
            <Button className="mt-2 w-full" onClick={apply} disabled={busy}>넣기</Button>
          </div>
        )}
        {done && <p className="text-sm text-emerald-700">넣었어요. 새 거래 {done.tx_new.toLocaleString()}건.</p>}
      </div>
      )}
    </Sheet>
  )
}
