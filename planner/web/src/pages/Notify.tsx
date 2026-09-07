import { useEffect, useState } from 'react'
import { useNavigate } from 'react-router'
import { useQuery } from '@tanstack/react-query'
import { api } from '../api'
import { useBFChildren, useCareKinds, useInvalidating, useUsers } from '../lib/hooks'
import { disablePush, enablePush, pushState, syncPush, testPush, type PushState } from '../lib/push'
import { Button, Field, Input, PageHeader, Sheet, SkeletonList, TimeField, Switch } from '../components/ui'
import { useConfirm } from '../components/Confirm'

/** 알림 설정. 위는 이 기기의 구독, 아래는 규칙. */

// 알림 = 무엇을(대상) + 어떤 조건에 + 언제 확인할지. 대상마다 쓸 수 있는 조건은 서버가 알려준다.
interface Source { key: string; label: string; help: string; conds: string[] }
interface Kinds { sources: Source[]; conds: Record<string, string> }
type Cond = 'pending' | 'none_today' | 'since' | 'always'
interface Rule {
  id: number
  kind: string
  title: string
  days_mask: number
  times: string[]
  params: {
    cond?: Cond; amount?: number; unit?: 'hours' | 'days'; watch?: boolean
    child_id?: number; care_kinds?: string[]; mine_only?: boolean; message?: string
  }
  recipients: number[]
  enabled: boolean
}

const DAY_NAMES = ['월', '화', '수', '목', '금', '토', '일'] // bit0=월
const PRESETS = [
  { label: '매일', mask: 0b1111111 },
  { label: '평일', mask: 0b0011111 },
  { label: '주말', mask: 0b1100000 },
]

function daysLabel(mask: number) {
  const p = PRESETS.find((x) => x.mask === mask)
  if (p) return p.label
  return DAY_NAMES.filter((_, i) => mask & (1 << i)).join('·')
}

export default function Notify() {
  const nav = useNavigate()
  const confirm = useConfirm()
  const kinds = useQuery({ queryKey: ['notify-kinds2'], queryFn: () => api.get<Kinds>('/api/notify/kinds'), staleTime: Infinity })
  const rules = useQuery({ queryKey: ['notify-rules'], queryFn: () => api.get<Rule[]>('/api/notify/rules') })
  const users = useUsers()
  const careKinds = useCareKinds()
  const [editing, setEditing] = useState<Rule | null>(null)

  const save = useInvalidating((r: Rule) => api.post<Rule>('/api/notify/rules', r), [['notify-rules']])
  const del = useInvalidating((id: number) => api.del(`/api/notify/rules/${id}`), [['notify-rules']])

  const kindLabel = (k: string) => kinds.data?.sources.find((x) => x.key === k)?.label ?? k
  const who = (ids: number[]) =>
    ids.length === 0 ? '가족 모두' : ids.map((id) => users.data?.find((u) => u.id === id)?.name ?? '?').join(', ')
  const detail = (r: Rule) => {
    const what = r.kind === 'care'
      ? (r.params.care_kinds ?? []).map((k) => careKinds.data?.find((c) => c.key === k)?.label ?? k).join('·') + ' 기록'
      : kindLabel(r.kind)
    const cond = r.params.cond === 'since'
      ? `마지막 이후 ${r.params.amount}${r.params.unit === 'days' ? '일' : '시간'} 지나면`
      : r.params.cond === 'always' ? '' : kinds.data?.conds[r.params.cond ?? ''] ?? ''
    const when = r.params.watch ? `${r.times[0]}~${r.times[1]} 계속` : `${daysLabel(r.days_mask)} ${r.times.join(', ')}`
    return [r.kind === 'custom' ? '' : `${what} ${cond}`, when].filter(Boolean).join(' · ')
  }

  return (
    <div className="mx-auto max-w-lg">
      <PageHeader title="알림" back={() => nav(-1)} />

      <DeviceCard />

      <section className="px-4 pt-5">
        <div className="flex items-center justify-between">
          <h2 className="text-sm font-bold text-ink-2">알림 목록</h2>
          <Button variant="ghost" onClick={() => setEditing(blank())}>+ 추가</Button>
        </div>
        {rules.isPending && <SkeletonList rows={2} />}
        {rules.data?.length === 0 && (
          <p className="py-6 text-center text-sm text-faint">아직 알림이 없어요. '+ 추가'로 만들어요.</p>
        )}
        <ul className="mt-2 space-y-2">
          {rules.data?.map((r) => (
            <li key={r.id} className="flex items-center gap-3 rounded-2xl bg-surface p-3.5 shadow-sm">
              <button onClick={() => setEditing(r)} className="min-w-0 flex-1 text-left">
                <p className={`truncate text-sm font-semibold ${r.enabled ? '' : 'text-faint'}`}>{r.title || (r.kind === 'custom' ? r.params.message : kindLabel(r.kind))}</p>
                <p className="truncate text-xs text-muted">{detail(r)}</p>
                <p className="truncate text-[11px] text-faint">{who(r.recipients)}</p>
              </button>
              <Switch on={r.enabled} onChange={(v) => save.mutate({ ...r, enabled: v })} />
            </li>
          ))}
        </ul>
      </section>

      <Sheet open={!!editing} onClose={() => setEditing(null)} title={editing?.id ? '알림 고치기' : '새 알림'}>
        {editing && kinds.data && (
          <RuleForm
            rule={editing}
            kinds={kinds.data}
            onSave={async (r) => { await save.mutateAsync(r); setEditing(null) }}
            onDelete={editing.id ? async () => {
              if (await confirm({ title: '이 알림을 지울까요?', confirmLabel: '지우기', danger: true })) { del.mutate(editing.id); setEditing(null) }
            } : undefined}
            onClose={() => setEditing(null)}
          />
        )}
      </Sheet>
    </div>
  )
}

function blank(): Rule {
  return { id: 0, kind: 'tasks', title: '', days_mask: 0b1111111, times: ['21:00'], params: { cond: 'pending' }, recipients: [], enabled: true }
}

function DeviceCard() {
  const [state, setState] = useState<PushState | null>(null)
  const [busy, setBusy] = useState(false)
  const [msg, setMsg] = useState<string | null>(null)
  // 열 때마다 이 기기의 구독을 서버와 맞춘다(서버가 지웠을 수 있다).
  useEffect(() => { syncPush().catch(() => {}).finally(() => pushState().then(setState)) }, [])

  const run = async (fn: () => Promise<unknown>, ok?: string) => {
    setBusy(true); setMsg(null)
    try { await fn(); setState(await pushState()); if (ok) setMsg(ok) }
    catch (e) { setMsg(e instanceof Error ? e.message : '실패했어요') }
    finally { setBusy(false) }
  }

  return (
    <section className="mx-4 mt-3 rounded-2xl bg-surface p-4 shadow-sm">
      <p className="text-sm font-semibold">이 기기에서 받기</p>
      {state === null && <p className="mt-1 text-xs text-faint">확인 중…</p>}
      {state === 'needs-install' && (
        <p className="mt-1 text-xs text-muted">
          아이폰은 <b>홈 화면에 추가한 앱</b>에서만 알림을 받을 수 있어요. 사파리 공유 버튼 → '홈 화면에 추가' 후 거기서 열어주세요.
        </p>
      )}
      {state === 'unsupported' && <p className="mt-1 text-xs text-muted">이 브라우저는 알림을 받을 수 없어요.</p>}
      {state === 'denied' && (
        <p className="mt-1 text-xs text-muted">알림이 막혀 있어요. 아이폰 설정 → 알림 → 플래너에서 허용해 주세요.</p>
      )}
      {(state === 'off' || state === 'on') && (
        <div className="mt-2 flex flex-wrap items-center gap-2">
          {state === 'off' ? (
            <Button disabled={busy} onClick={() => run(() => enablePush(), '이 기기에서 알림을 받아요')}>알림 켜기</Button>
          ) : (
            <>
              <span className="text-xs font-medium text-emerald-600">받는 중</span>
              <Button variant="ghost" disabled={busy} onClick={() => run(testPush, '시험 알림을 보냈어요')}>시험 알림</Button>
              <Button variant="ghost" disabled={busy} onClick={() => run(disablePush, '이 기기에서는 안 받아요')}>끄기</Button>
            </>
          )}
        </div>
      )}
      {msg && <p className="mt-2 text-xs text-muted">{msg}</p>}
    </section>
  )
}

function RuleForm({ rule, kinds, onSave, onDelete, onClose }: {
  rule: Rule
  kinds: Kinds
  onSave: (r: Rule) => Promise<void>
  onDelete?: () => void
  onClose: () => void
}) {
  const users = useUsers()
  const children = useBFChildren()
  const careKinds = useCareKinds()
  const [r, setR] = useState<Rule>(rule)
  const [err, setErr] = useState<string | null>(null)
  const set = (patch: Partial<Rule>) => setR({ ...r, ...patch })
  const setP = (patch: Partial<Rule['params']>) => setR({ ...r, params: { ...r.params, ...patch } })
  const src = kinds.sources.find((x) => x.key === r.kind)
  const cond = (r.params.cond ?? src?.conds[0]) as Cond | undefined
  const since = cond === 'since'
  const watch = since && !!r.params.watch

  // 대상을 바꾸면 그 대상의 첫 조건과 알맞은 기본값으로.
  const pickSource = (k: string) => {
    const s2 = kinds.sources.find((x) => x.key === k)!
    const c = s2.conds[0] as Cond
    const base: Rule['params'] = { cond: c }
    if (k === 'care') Object.assign(base, { care_kinds: ['formula', 'breast'], child_id: children.data?.[0]?.user_id, amount: 3, unit: 'hours', watch: true })
    if (k === 'finance') Object.assign(base, { amount: 30, unit: 'days' })
    if (k === 'diary' && c === 'since') Object.assign(base, { amount: 3, unit: 'days' })
    setR({ ...r, kind: k, params: base, times: base.watch ? ['08:00', '22:00'] : r.params.watch ? ['21:00'] : r.times })
  }
  const pickCond = (c: Cond) => {
    const p: Rule['params'] = { ...r.params, cond: c }
    if (c === 'since' && !p.amount) Object.assign(p, r.kind === 'care' ? { amount: 3, unit: 'hours' } : { amount: r.kind === 'finance' ? 30 : 3, unit: 'days' })
    if (c !== 'since') p.watch = false
    setR({ ...r, params: p, times: r.params.watch && !p.watch ? ['21:00'] : r.times })
  }
  const setWatch = (w: boolean) => setR({ ...r, params: { ...r.params, watch: w }, times: w ? ['08:00', '22:00'] : ['09:00'] })

  return (
    <form
      onSubmit={async (e) => {
        e.preventDefault(); setErr(null)
        try { await onSave({ ...r, params: { ...r.params, cond } }) } catch (ex) { setErr(ex instanceof Error ? ex.message : '저장하지 못했어요') }
      }}
      className="max-h-[70dvh] space-y-4 overflow-y-auto"
    >
      <Field label="무엇을 볼까요">
        <div className="flex flex-wrap gap-1.5">
          {kinds.sources.map((k) => <Chip key={k.key} on={r.kind === k.key} onClick={() => pickSource(k.key)}>{k.label}</Chip>)}
        </div>
        {src && <p className="mt-1 text-[11px] text-faint">{src.help}</p>}
      </Field>

      {r.kind === 'care' && (
        <>
          {children.data && children.data.length > 1 && (
            <Field label="누구">
              <div className="flex gap-1.5">
                {children.data.map((c) => (
                  <Chip key={c.user_id} on={r.params.child_id === c.user_id} onClick={() => setP({ child_id: c.user_id })}>{c.name}</Chip>
                ))}
              </div>
            </Field>
          )}
          <Field label="어떤 기록 (하나라도 있으면 된 걸로 봐요)">
            <div className="flex flex-wrap gap-1.5">
              {careKinds.data?.map((k) => {
                const on = r.params.care_kinds?.includes(k.key) ?? false
                return (
                  <Chip key={k.key} on={on} onClick={() => setP({
                    care_kinds: on ? (r.params.care_kinds ?? []).filter((x) => x !== k.key) : [...(r.params.care_kinds ?? []), k.key],
                  })}>{k.label}</Chip>
                )
              })}
            </div>
          </Field>
        </>
      )}

      {src && r.kind !== 'custom' && (
        <Field label="어떤 조건에">
          <div className="flex flex-wrap gap-1.5">
            {src.conds.map((c) => <Chip key={c} on={cond === c} onClick={() => pickCond(c as Cond)}>{kinds.conds[c]}</Chip>)}
          </div>
          {since && (
            <div className="mt-2 flex flex-nowrap items-center gap-2 whitespace-nowrap">
              <span className="shrink-0 text-sm text-muted">마지막 이후</span>
              {/* 공용 Input 은 w-full 이라 여기선 좁은 칸을 직접 쓴다(두 자리면 충분). */}
              <input inputMode="numeric" maxLength={2} value={String(r.params.amount ?? '')}
                onChange={(e) => setP({ amount: Number(e.target.value.replace(/[^0-9]/g, '')) || 0 })}
                className="w-12 shrink-0 rounded-xl border border-line bg-surface px-2 py-2.5 text-center text-base outline-none focus:border-ghost" />
              <select value={r.params.unit ?? 'hours'} onChange={(e) => setP({ unit: e.target.value as 'hours' | 'days' })}
                className="shrink-0 rounded-xl border border-line bg-surface px-2 py-2.5 text-base">
                <option value="hours">시간</option>
                <option value="days">일</option>
              </select>
              <span className="shrink-0 text-sm text-muted">지나면</span>
            </div>
          )}
          {since && <p className="mt-1 text-[11px] text-faint">풀릴 때까지(새로 기록·업로드할 때까지) 확인할 때마다 알려요.</p>}
          {r.kind === 'tasks' && r.recipients.length > 0 && (
            <label className="mt-2 flex items-center gap-2 text-xs text-muted">
              <input type="checkbox" checked={!!r.params.mine_only} onChange={(e) => setP({ mine_only: e.target.checked })} />
              받는 사람이 담당인 할 일만 세기
            </label>
          )}
        </Field>
      )}

      {r.kind === 'custom' ? (
        <Field label="보낼 문구"><Input value={r.params.message ?? ''} onChange={(e) => setP({ message: e.target.value })} placeholder="예: 비타민D 먹이기" /></Field>
      ) : (
        <Field label="이름 (알림 제목, 비우면 기본 문구)"><Input value={r.title} onChange={(e) => set({ title: e.target.value })} /></Field>
      )}

      <Field label="언제 확인할까요">
        {since && (
          <div className="mb-2 flex gap-1.5">
            <Chip on={!watch} onClick={() => setWatch(false)}>정한 시각에</Chip>
            <Chip on={watch} onClick={() => setWatch(true)}>시간대 동안 계속</Chip>
          </div>
        )}
        <div className="flex gap-1.5">
          {PRESETS.map((p) => <Chip key={p.label} on={r.days_mask === p.mask} onClick={() => set({ days_mask: p.mask })}>{p.label}</Chip>)}
        </div>
        <div className="mt-1.5 flex gap-1">
          {DAY_NAMES.map((d, i) => {
            const on = (r.days_mask & (1 << i)) !== 0
            return (
              <button key={d} type="button" onClick={() => set({ days_mask: r.days_mask ^ (1 << i) })}
                className={`h-9 flex-1 rounded-lg text-sm font-medium ${on ? 'bg-accent text-accent-ink' : 'bg-surface-2 text-faint'}`}>
                {d}
              </button>
            )
          })}
        </div>
        {watch ? (
          <div className="mt-2 space-y-1.5">
            <p className="text-[11px] text-faint">이 시간대에만 지켜봐요(밤에는 안 울리게). 같은 공백엔 한 번만 알려요.</p>
            <TimeField value={r.times[0]} onChange={(v) => set({ times: [v, r.times[1]] })} />
            <TimeField value={r.times[1]} onChange={(v) => set({ times: [r.times[0], v] })} />
          </div>
        ) : (
          <div className="mt-2 space-y-1.5">
            {r.times.map((t, i) => (
              <div key={i} className="flex items-center gap-1.5">
                <div className="min-w-0 flex-1"><TimeField value={t} onChange={(v) => set({ times: r.times.map((x, k) => (k === i ? v : x)) })} /></div>
                {r.times.length > 1 && (
                  <button type="button" onClick={() => set({ times: r.times.filter((_, k) => k !== i) })} className="rounded-lg px-2 text-faint" aria-label="시각 빼기">×</button>
                )}
              </div>
            ))}
            {r.times.length < 12 && (
              <button type="button" onClick={() => set({ times: [...r.times, '12:00'] })} className="text-xs font-medium text-muted">+ 시각 더하기</button>
            )}
          </div>
        )}
      </Field>

      <Field label={r.kind === 'finance' ? '받는 사람 (비우면 파일 주인 각자에게)' : '받는 사람 (아무도 안 고르면 가족 모두)'}>
        <div className="flex flex-wrap gap-1.5">
          {users.data?.map((u) => {
            const on = r.recipients.includes(u.id)
            return <Chip key={u.id} on={on} onClick={() => set({ recipients: on ? r.recipients.filter((x) => x !== u.id) : [...r.recipients, u.id] })}>{u.name}</Chip>
          })}
        </div>
      </Field>

      {err && <p className="text-xs text-rose-500">{err}</p>}
      <div className="flex items-center justify-between pt-1">
        {onDelete ? <Button type="button" variant="ghost" onClick={onDelete} className="text-rose-500">지우기</Button> : <span />}
        <div className="flex gap-2">
          <Button type="button" variant="ghost" onClick={onClose}>취소</Button>
          <Button type="submit">저장</Button>
        </div>
      </div>
    </form>
  )
}

function Chip({ on, onClick, children }: { on: boolean; onClick: () => void; children: React.ReactNode }) {
  return (
    <button type="button" onClick={onClick}
      className={`rounded-lg px-3 py-1.5 text-sm font-medium ${on ? 'bg-accent text-accent-ink' : 'bg-surface-2 text-muted'}`}>
      {children}
    </button>
  )
}
