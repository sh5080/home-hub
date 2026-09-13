import { useEffect, useState } from 'react'
import { Link, useNavigate } from 'react-router'
import { api, type FamilyBoard } from '../api'
import { useFamily, useInvalidating } from '../lib/hooks'
import { fmtWon } from '../lib/date'
import { Button, Field, Input, PageHeader, Sheet, SkeletonList } from '../components/ui'
import { useConfirm } from '../components/Confirm'
import { PokablePlant } from '../components/Plant'

// 함께: 여러 기능의 기록으로 물방울을 모아 식물을 키운다. 경쟁이 아니라 같이 채운다.
export default function Family() {
  const nav = useNavigate()
  const q = useFamily()
  const raw = q.data
  const b = raw && { ...raw, quests: raw.quests ?? [], streaks: raw.streaks ?? [], milestones: raw.milestones ?? [], wishes: raw.wishes ?? [], together: raw.together ?? {},
    jar: raw.jar && { ...raw.jar, months: raw.jar.months ?? [] }, plant: { ...raw.plant, recent: raw.plant.recent ?? [] } }
  const seen = useInvalidating((keys: string[]) => api.post('/api/family/seen', { keys }), [['family']])
  // 새 마일스톤은 이 화면을 열면 본 걸로 친다(표시는 이번 화면 동안 남긴다).
  const [fresh, setFresh] = useState<string[]>([])
  useEffect(() => {
    const keys = b?.milestones.filter((m) => m.new).map((m) => m.key) ?? []
    if (keys.length) { setFresh(keys); seen.mutate(keys) }
  }, [b?.milestones]) // eslint-disable-line react-hooks/exhaustive-deps

  return (
    <div className="mx-auto max-w-lg pb-10">
      <PageHeader title="함께" back={() => nav(-1)} />
      {q.isPending && <div className="px-4 pt-4"><SkeletonList rows={4} /></div>}
      {b && (
        <div className="space-y-3 px-4 pt-3">
          <PlantCard b={b} />
          <Quests b={b} />
          <Streaks b={b} />
          {b.jar && <Jar b={b} />}
          <Milestones b={b} fresh={fresh} />
          <Together b={b} />
          <Wishes b={b} />
        </div>
      )}
    </div>
  )
}

function Box({ id, title, right, children }: { id?: string; title: string; right?: React.ReactNode; children: React.ReactNode }) {
  return (
    <section id={id} className="scroll-mt-16 rounded-2xl bg-surface p-4 shadow-sm">
      <div className="mb-2 flex items-baseline justify-between gap-2">
        <h2 className="text-sm font-bold text-ink-2">{title}</h2>
        {right}
      </div>
      {children}
    </section>
  )
}

// 입체 막대: 홈 파인 트랙 + 위쪽이 밝은 채움.
const Bar = ({ value, max, color = 'bg-emerald-500' }: { value: number; max: number; color?: string }) => (
  <div className="h-2 overflow-hidden rounded-full bg-line">
    <div className={`relative h-full rounded-full ${color} transition-all`} style={{ width: `${max > 0 ? Math.min(100, (value / max) * 100) : 100}%` }}>
    </div>
  </div>
)

const HERO: Record<number, [string, string]> = {
  1: ['땅속에서', '꿈꾸는 콩이'],
  2: ['드디어', '싹이 텄어요'],
  3: ['떡잎이', '쑥쑥 자라요'],
  4: ['곧 꽃이', '필 것 같아요'],
  5: ['콩이를', '다 키웠어요'],
}

// 맨 위: 금색 머리말 → 두 줄 헤드라인 → 설명 → 받침대 위 콩이 → 진행 → 크림 버튼 → 작은 안내.
function PlantCard({ b }: { b: FamilyBoard }) {
  const p = b.plant
  const [why, setWhy] = useState(false)
  const [naming, setNaming] = useState<string | null>(null)
  const shown = p.stage
  const keys = [['family']]
  const rename = useInvalidating((name: string) => api.put('/api/family/plant-name', { name }), keys)
  const next = useInvalidating(() => api.post('/api/family/grow-next'), keys)
  const prev = b.stages[p.stage - 1].drops
  const [l1, l2] = HERO[shown] ?? HERO[1]
  return (
    <section className="family-theme dark relative overflow-hidden rounded-3xl bg-canvas px-5 pb-6 pt-5 text-ink shadow-[0_14px_32px_-18px_rgba(16,28,52,0.9)]">
      <button type="button" onClick={() => setNaming(p.name)} className="group flex items-center gap-1.5 text-xs font-bold tracking-wide text-[#c9a64a]">
        <span>우리 가족 · {p.name || '콩이'}{p.count > 0 && ` · ${p.count + 1}번째`}</span>
        <svg viewBox="0 0 24 24" className="h-3.5 w-3.5 opacity-60 group-hover:opacity-100" fill="none" stroke="currentColor" strokeWidth="2.2" strokeLinecap="round" strokeLinejoin="round"><path d="M12 20h9" /><path d="M16.5 3.5a2.1 2.1 0 0 1 3 3L7 19l-4 1 1-4z" /></svg>
      </button>
      <h1 className="hero-title mt-4 text-[34px] font-black leading-[1.18] tracking-tight">{l1}<br />{l2}</h1>
      <p className="mt-4 text-[15px] leading-relaxed text-muted">
        {p.ready ? <>함께 해낸 덕분이에요.<br />다음 콩을 심으면 🎟️ 구매권이 생겨요.</> : <>💧{p.next - p.drops}개 더 모으면<br />콩이가 한 뼘 더 자라요.</>}
      </p>

      <div className="relative mt-6 flex justify-center">
        <PokablePlant stage={shown} size={250} happy={p.ready || p.today > 0}
          hint={p.ready ? '다음 콩을 심어요 🌱' : `💧${p.next - p.drops}개만 더 주세요!`} />
      </div>

      {/* 진행: 금빛 막대 + 다섯 단계(누르면 미리보기) */}
      <div className="mt-4">
        <div className="mb-2 flex items-baseline justify-between text-xs">
          <span className="font-bold text-ink">💧 {p.drops}<span className="font-normal text-faint"> / {p.ready ? b.stages[4].drops : p.next}</span></span>
          <span className="text-faint">{p.today > 0 ? `오늘 +${p.today}` : '오늘은 아직'}</span>
        </div>
        <div className="h-2 overflow-hidden rounded-full bg-[#1d2c47]">
          <div className="h-full rounded-full bg-gradient-to-r from-[#d9b24a] to-[#f3dc98] transition-all" style={{ width: `${p.ready ? 100 : Math.min(100, ((p.drops - prev) / (p.next - prev)) * 100)}%` }} />
        </div>
      </div>

      {p.ready ? (
        <button type="button" disabled={next.isPending} onClick={() => next.mutate(undefined)}
          className="mt-6 h-14 w-full rounded-2xl bg-[#f3ebdd] text-base font-bold text-[#22334f] shadow-[0_10px_24px_-12px_rgba(0,0,0,0.6)] disabled:opacity-60">🌱 다음 콩 심기</button>
      ) : (
        <button type="button" onClick={() => document.getElementById('family-quests')?.scrollIntoView({ behavior: 'smooth', block: 'start' })}
          className="mt-6 h-14 w-full rounded-2xl bg-[#f3ebdd] text-base font-bold text-[#22334f] shadow-[0_10px_24px_-12px_rgba(0,0,0,0.6)]">이번 주 퀘스트 보기</button>
      )}
      <p className="mt-3 text-center text-xs text-faint">
        {p.credits > 0 ? <span className="font-semibold text-[#e2b84a]">🎟️ 구매권 {p.credits}장 · 위시리스트에서 사요</span> : '콩이를 누르면 반응해요'}
        {' · '}<button type="button" onClick={() => setWhy(!why)} className="underline">{why ? '접기' : '물방울 모으는 법'}</button>
      </p>
      {why && (
        <div className="mt-3 space-y-2 rounded-2xl bg-surface p-4 text-xs text-muted">
          <ul className="space-y-1">
            <li>🔥 연속 기록을 해낸 날마다 기록 하나에 💧1</li>
            <li>🎯 이번 주 가족 퀘스트 하나에 💧3, 다 채우면 보너스 💧5</li>
            <li>✅ 할 일을 처음 정한 마감까지 끝내면 💧2 (넘기면 없어요)</li>
            <li>🏅 마일스톤을 이루면 💧5</li>
            <li>🌸 다 키우면(💧{b.stages[4].drops}) 🎟️ 구매권 — 위시리스트에서 하나를 사요</li>
          </ul>
          {p.recent.length > 0 && (
            <ul className="divide-y divide-line">
              {p.recent.map((r, i) => (
                <li key={i} className="flex justify-between gap-2 py-1.5">
                  <span className="min-w-0 truncate">{r.day.slice(5).replace('-', '/')} · {r.label}</span>
                  <span className="shrink-0 font-semibold text-[#e2b84a]">+{r.amount}</span>
                </li>
              ))}
            </ul>
          )}
        </div>
      )}
      <Sheet open={naming !== null} onClose={() => setNaming(null)} title="콩이 이름">
        {naming !== null && (
          <form className="space-y-3" onSubmit={async (e) => { e.preventDefault(); await rename.mutateAsync(naming.trim()); setNaming(null) }}>
            <Input autoFocus value={naming} maxLength={12} placeholder="예: 콩콩이" onChange={(e) => setNaming(e.target.value)} />
            <p className="text-[11px] text-faint">언제든 바꿀 수 있어요. 다음 콩도 이 이름을 이어받아요.</p>
            <div className="flex justify-end"><Button type="submit" disabled={rename.isPending}>저장</Button></div>
          </form>
        )}
      </Sheet>
    </section>
  )
}

const byLine = (by: Record<string, number> | null) => {
  const xs = Object.entries(by ?? {}).filter(([, n]) => n > 0)
  return xs.length ? xs.map(([n, v]) => `${n} ${v}`).join(' · ') : ''
}

function Quests({ b }: { b: FamilyBoard }) {
  const done = b.quests.filter((q) => q.done).length
  return (
    <Box id="family-quests" title="이번 주 가족 퀘스트" right={<span className="text-xs text-muted">{done}/{b.quests.length} · {b.week.slice(5).replace('-', '/')}~{b.week_end.slice(5).replace('-', '/')}</span>}>
      <ul className="space-y-3">
        {b.quests.map((q) => (
          <li key={q.key}>
            <Link to={q.url} className="block">
              <div className="flex items-baseline justify-between gap-2">
                <span className={`text-sm font-medium ${q.done ? 'text-emerald-700 dark:text-emerald-300' : ''}`}>{q.done ? '✅ ' : '🎯 '}{q.label}</span>
                <span className="shrink-0 text-xs tabular-nums text-muted">
                  {q.unit === '원' ? `${fmtWon(q.value)} / ${fmtWon(q.target)}` : q.key === 'tags' ? `${q.value}${q.unit} 남음` : `${q.value}/${q.target}${q.unit}`}
                </span>
              </div>
              <div className="mt-1">
                {q.key === 'tags' ? <Bar value={q.value === 0 ? 1 : 0} max={1} /> : q.unit === '원'
                  ? <Bar value={q.value} max={q.target} color={q.value > q.target ? 'bg-rose-400' : 'bg-emerald-500'} />
                  : <Bar value={q.value} max={q.target} />}
              </div>
              <p className="mt-0.5 text-[11px] text-faint">{q.help}{byLine(q.by) && ` · 고마워요 ${byLine(q.by)}`}</p>
            </Link>
          </li>
        ))}
      </ul>
      <p className="mt-3 text-[11px] text-faint">매주 월요일에 바뀌어요. 누가 하든 같이 채워요 — 하나에 💧3, 다 채우면 💧5 더.</p>
    </Box>
  )
}

function Streaks({ b }: { b: FamilyBoard }) {
  return (
    <Box title="연속 기록">
      <div className="grid grid-cols-2 gap-2">
        {b.streaks.map((s) => (
          <Link key={s.key} to={s.url} className="rounded-xl bg-gradient-to-b from-surface to-surface-2 p-3 shadow-[0_3px_8px_-4px_rgba(15,23,42,0.3)] ring-1 ring-line/70">
            <p className="text-xs text-muted">{s.label}</p>
            <p className="mt-0.5 text-xl font-bold tabular-nums">{s.current > 0 ? '🔥' : '🌱'} {s.current}<span className="text-xs font-normal text-muted">일</span></p>
            <p className="text-[11px] text-faint">{s.today_done ? '오늘 완료' : '오늘 아직'} · 이번 달 {s.month_done}/{s.month_days}일</p>
          </Link>
        ))}
      </div>
      <p className="mt-2 text-[11px] text-faint">끊겨도 괜찮아요 — 이번 달 해낸 날도 같이 세요. 해낸 날마다 💧1.</p>
    </Box>
  )
}

function Jar({ b }: { b: FamilyBoard }) {
  const j = b.jar!
  const over = j.pace > j.spendable
  return (
    <Box title="저금통" right={<Link to="/finance" className="text-xs text-muted underline">자산</Link>}>
      <div className="flex items-end justify-between">
        <div>
          <p className="text-xs text-muted">이번 달 남은 생활비</p>
          <p className={`text-xl font-bold tabular-nums ${j.month_left < 0 ? 'text-rose-500' : ''}`}>{fmtWon(j.month_left)}</p>
        </div>
        <span className="text-4xl">🐷</span>
      </div>
      <div className="mt-2"><Bar value={Math.max(0, j.month_left)} max={j.spendable} color={j.month_left < 0 ? 'bg-rose-400' : 'bg-amber-400'} /></div>
      <p className={`mt-1 text-[11px] ${over ? 'text-rose-500' : 'text-faint'}`}>
        이 속도면 월말에 {over ? `${fmtWon(j.pace - j.spendable)} 넘어요` : `${fmtWon(j.spendable - j.pace)} 저금통에 들어가요`}
        {j.incentive > 0 && ` · 지역화폐 적립 보너스 +${fmtWon(j.incentive)}`}
      </p>
      <div className="mt-3 rounded-xl bg-surface-2 p-3 text-sm">
        <div className="flex justify-between"><span>지금까지 모은 돈 <span className="text-[11px] text-faint">({j.from.slice(0, 4)}년 {Number(j.from.slice(5))}월부터)</span></span><b className="tabular-nums">{fmtWon(j.total)}</b></div>
        {j.goal_name && <div className="mt-1.5"><p className="mb-0.5 text-[11px] text-muted">목표 '{j.goal_name}'</p><Bar value={Math.max(0, j.total)} max={j.goal_target} color="bg-amber-400" /></div>}
        {j.months.length > 0 && (
          <ul className="mt-2 space-y-0.5 text-xs">
            {j.months.slice().reverse().map((m) => (
              <li key={m.month} className="flex justify-between"><span className="text-muted">{Number(m.month.slice(5))}월</span><span className={`tabular-nums ${m.saved < 0 ? 'text-rose-500' : 'text-emerald-700'}`}>{m.saved >= 0 ? '+' : ''}{fmtWon(m.saved)}</span></li>
            ))}
          </ul>
        )}
        {j.months.length === 0 && <p className="mt-1 text-[11px] text-faint">달이 끝나면 '한 달에 쓸 수 있는 돈'에서 남은 만큼 쌓여요.</p>}
      </div>
    </Box>
  )
}

function Milestones({ b, fresh }: { b: FamilyBoard; fresh: string[] }) {
  const done = b.milestones.filter((m) => m.done).sort((x, y) => y.at.localeCompare(x.at))
  const next = b.milestones.filter((m) => !m.done)
  const [all, setAll] = useState(false)
  const shown = all ? done : done.slice(0, 4)
  return (
    <Box title="성장 마일스톤">
      {fresh.length > 0 && (
        <div className="mb-3 rounded-xl bg-amber-50 p-3 text-center dark:bg-amber-950/40">
          <p className="text-2xl">🎉</p>
          {b.milestones.filter((m) => fresh.includes(m.key)).map((m) => <p key={m.key} className="text-sm font-semibold">{m.label}</p>)}
          <p className="mt-0.5 text-[11px] text-muted">축하해요! 💧5씩 받았어요.</p>
        </div>
      )}
      {next.length > 0 && (
        <>
          <p className="mb-1 text-xs font-semibold text-muted">다음 목표</p>
          <ul className="mb-3 space-y-2">
            {next.map((m) => (
              <li key={m.key}>
                <Link to={m.url} className="block">
                  <div className="flex justify-between text-sm"><span>{m.label}</span><span className="text-xs tabular-nums text-muted">{m.group === '성장' ? `${m.at.slice(5).replace('-', '/')}` : `${m.value}/${m.target}`}</span></div>
                  <div className="mt-0.5"><Bar value={m.value} max={m.target} color="bg-violet-400" /></div>
                </Link>
              </li>
            ))}
          </ul>
        </>
      )}
      {done.length > 0 && (
        <>
          <p className="mb-1 text-xs font-semibold text-muted">이룬 것 {done.length}</p>
          <ul className="space-y-1">
            {shown.map((m) => (
              <li key={m.key} className="flex justify-between text-sm"><span>🏅 {m.label}</span><span className="text-xs text-faint">{m.at && `${m.at.slice(2).replaceAll('-', '.')}`}</span></li>
            ))}
          </ul>
          {done.length > 4 && <button onClick={() => setAll(!all)} className="mt-1 text-xs text-muted underline">{all ? '접기' : `${done.length}개 모두 보기`}</button>}
        </>
      )}
    </Box>
  )
}

function Together({ b }: { b: FamilyBoard }) {
  const xs = Object.entries(b.together).filter(([, n]) => n > 0)
  if (xs.length === 0) return null
  return (
    <Box title="이번 주 함께한 기록">
      <div className="flex flex-wrap gap-2">
        {xs.map(([n, v]) => (
          <span key={n} className="rounded-xl bg-surface-2 px-3 py-2 text-sm"><b>{n}</b> <span className="tabular-nums text-muted">{v}번</span></span>
        ))}
      </div>
      <p className="mt-2 text-[11px] text-faint">육아 기록·루틴·일기·할 일을 더한 수예요. 순위가 아니라 서로 고마운 만큼이에요 💛</p>
    </Box>
  )
}

function Wishes({ b }: { b: FamilyBoard }) {
  const confirm = useConfirm()
  const keys = [['family']]
  const save = useInvalidating((w: { id?: number; title: string; price: number; note: string }) =>
    w.id ? api.patch(`/api/family/wishes/${w.id}`, w) : api.post('/api/family/wishes', w), keys)
  const del = useInvalidating((id: number) => api.del(`/api/family/wishes/${id}`), keys)
  const [edit, setEdit] = useState<{ id?: number; title: string; price: string; note: string } | null>(null)
  const [err, setErr] = useState<string | null>(null)
  const buy = useInvalidating((id: number) => api.post(`/api/family/wishes/${id}/buy`), keys)
  const open = b.wishes.filter((w) => !w.bought_at)
  const bought = b.wishes.filter((w) => w.bought_at)
  const credits = b.plant.credits
  return (
    <Box title="위시리스트" right={<button onClick={() => setEdit({ title: '', price: '', note: '' })} className="text-xs text-muted underline">+ 추가</button>}>
      {credits > 0 && open.length > 0 && <p className="mb-2 rounded-xl bg-amber-50 px-3 py-2 text-xs font-medium text-amber-800 ring-1 ring-amber-200 dark:bg-amber-950/40 dark:text-amber-300 dark:ring-amber-800">🎟️ 구매권 {credits}장 — 갖고 싶은 걸 하나 골라 사세요!</p>}
      {open.length === 0 && <p className="text-xs text-faint">{credits > 0 ? '🎟️ 구매권이 있어요! 갖고 싶은 걸 넣고 사세요.' : '식물을 다 키우면 여기서 하나를 사요. 갖고 싶은 걸 언제든 넣어두세요.'}</p>}
      <ul className="space-y-1.5">
        {open.map((w) => (
          <li key={w.id} className={`flex items-center gap-2 rounded-xl px-2 py-1.5 ${credits > 0 ? 'bg-gradient-to-r from-amber-50 to-surface ring-1 ring-amber-200 shadow-[0_4px_12px_-8px_rgba(180,83,9,0.6)] dark:from-amber-950/40 dark:ring-amber-800' : ''}`}>
            <button type="button" onClick={() => setEdit({ id: w.id, title: w.title, price: w.price ? String(w.price) : '', note: w.note })} className="flex min-w-0 flex-1 items-center justify-between gap-2 text-left">
              <span className="min-w-0">
                <span className="flex items-center gap-1.5 truncate text-sm">🎁 {w.title}{credits > 0 && <span className="shrink-0 rounded-full bg-amber-400 px-1.5 py-px text-[10px] font-bold text-white">구매 가능</span>}</span>
                {w.note && <span className="block truncate text-[11px] text-faint">{w.note}</span>}
              </span>
              {w.price > 0 && <span className="shrink-0 text-sm tabular-nums text-muted">{fmtWon(w.price)}</span>}
            </button>
            {credits > 0 && (
              <Button className="shrink-0 px-3 py-1.5 text-xs" disabled={buy.isPending} onClick={async () => {
                if (await confirm({ title: `'${w.title}'을(를) 살까요?`, body: '구매권 한 장을 써요.', confirmLabel: '살게요' })) buy.mutate(w.id)
              }}>사기</Button>
            )}
          </li>
        ))}
      </ul>
      {bought.length > 0 && (
        <div className="mt-2 border-t border-line pt-2">
          <p className="mb-1 text-xs text-muted">식물로 산 것</p>
          <ul className="space-y-0.5">
            {bought.map((w) => (
              <li key={w.id} className="flex justify-between text-xs text-faint"><span>🌸 {w.title}</span><span>{new Date(w.bought_at! * 1000).toLocaleDateString('ko-KR')}</span></li>
            ))}
          </ul>
        </div>
      )}
      <Sheet open={edit !== null} onClose={() => { setEdit(null); setErr(null) }} title={edit?.id ? '위시 고치기' : '위시 추가'}>
        {edit && (
          <form className="space-y-3" onSubmit={async (e) => {
            e.preventDefault(); setErr(null)
            try { await save.mutateAsync({ id: edit.id, title: edit.title.trim(), price: Number(edit.price) || 0, note: edit.note.trim() }); setEdit(null) } catch (ex) { setErr(ex instanceof Error ? ex.message : '저장하지 못했어요') }
          }}>
            <Field label="갖고 싶은 것"><Input autoFocus value={edit.title} maxLength={60} placeholder="예: 아기 띠, 커피 머신" onChange={(e) => setEdit({ ...edit, title: e.target.value })} /></Field>
            <Field label="가격 (원, 선택)"><Input inputMode="numeric" value={edit.price} onChange={(e) => setEdit({ ...edit, price: e.target.value.replace(/[^0-9]/g, '') })} /></Field>
            <Field label="메모 (선택)"><Input value={edit.note} maxLength={200} placeholder="링크나 색상 등" onChange={(e) => setEdit({ ...edit, note: e.target.value })} /></Field>
            {err && <p className="text-xs text-rose-500">{err}</p>}
            <div className="flex gap-2">
              {edit.id && <Button type="button" variant="danger" onClick={async () => {
                if (await confirm({ title: '위시를 지울까요?', confirmLabel: '삭제', danger: true })) { del.mutate(edit.id!); setEdit(null) }
              }}>삭제</Button>}
              <div className="flex-1" />
              <Button type="submit" disabled={!edit.title.trim() || save.isPending}>저장</Button>
            </div>
          </form>
        )}
      </Sheet>
    </Box>
  )
}
