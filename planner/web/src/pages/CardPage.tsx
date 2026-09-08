import { useEffect, useRef, useState } from 'react'
import { useNavigate, useParams } from 'react-router'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useConfirm } from '../components/Confirm'
import { api, type Card, type Column } from '../api'
import { useUsers } from '../lib/hooks'
import { dueLabel, hasTime, today } from '../lib/date'
import Byline from '../components/Byline'
import BlockEditor from '../components/BlockEditor'
import { StarPicker } from '../components/SortToggle'
import { SkeletonList } from '../components/ui'

// 카드 상세(전체 페이지). 저장 버튼 없이 800ms 뒤 자동 저장.
export default function CardPage() {
  const confirm = useConfirm()
  const cardId = Number(useParams().id)
  const nav = useNavigate()
  const qc = useQueryClient()
  const users = useUsers()

  const q = useQuery({
    queryKey: ['card', cardId],
    queryFn: () => api.get<{ card: Card; board: { id: number; name: string }; columns: Column[] }>(`/api/cards/${cardId}`),
  })

  const [saving, setSaving] = useState<'idle' | 'saving' | 'saved' | 'error'>('idle')
  const timer = useRef<number | undefined>(undefined)
  const pending = useRef<Record<string, unknown>>({})

  // 속성은 저장을 기다리지 않고 캐시에 먼저 반영한다. 본문(content)은 빼야 한다 — 편집기가 다시 그려지면 커서가 튄다.
  function applyLocal(patch: Record<string, unknown>) {
    const { content: _content, ...rest } = patch
    if (Object.keys(rest).length === 0) return
    qc.setQueryData(['card', cardId], (old: typeof q.data) => (old ? { ...old, card: { ...old.card, ...rest } } : old))
  }

  function queueSave(patch: Record<string, unknown>) {
    applyLocal(patch)
    pending.current = { ...pending.current, ...patch }
    setSaving('saving')
    window.clearTimeout(timer.current)
    timer.current = window.setTimeout(async () => {
      const body = pending.current
      pending.current = {}
      try {
        const saved = await api.patch<Card>(`/api/cards/${cardId}`, body)
        // 서버가 계산하는 값(반복 설명 등)으로 맞춘다.
        if (saved && typeof saved === 'object' && 'id' in saved) {
          const { content: _c, ...fields } = saved
          // 보관·완료 시각은 비면 응답에서 빠진다(omitempty) — 빠진 걸 null 로 채워야 보관 해제가 반영된다.
          applyLocal({ ...fields, archived_at: saved.archived_at ?? null, done_at: saved.done_at ?? null } as Record<string, unknown>)
        }
        setSaving('saved')
        // 이 페이지 캐시는 건드리지 않는다(편집 중 리셋되면 커서가 튄다).
        qc.invalidateQueries({ queryKey: ['board'] })
        qc.invalidateQueries({ queryKey: ['today'] })
        qc.invalidateQueries({ queryKey: ['calendar'] })
      } catch {
        setSaving('error')
      }
    }, 800)
  }

  useEffect(() => () => window.clearTimeout(timer.current), [])

  const [archiving, setArchiving] = useState(false)
  async function setArchived(on: boolean) {
    setArchiving(true)
    try {
      const saved = on ? await api.post<Card>(`/api/cards/${cardId}/archive`) : await api.del<Card>(`/api/cards/${cardId}/archive`)
      if (saved) applyLocal({ archived_at: saved.archived_at ?? null, done_at: saved.done_at ?? null })
      qc.invalidateQueries({ queryKey: ['board'] })
      qc.invalidateQueries({ queryKey: ['archive'] })
    } catch {
      setSaving('error')
    } finally {
      setArchiving(false)
    }
  }

  if (q.isPending) {
    return (
      <div className="mx-auto max-w-2xl space-y-4 p-4">
        <div className="h-8 w-2/3 animate-pulse rounded bg-line" />
        <SkeletonList rows={3} />
        <div className="h-32 animate-pulse rounded-xl bg-surface-2" />
      </div>
    )
  }
  if (q.isError || !q.data) return <div className="p-6 text-rose-500">카드를 불러올 수 없어요</div>

  const { card, board, columns } = q.data
  const col = columns.find((c) => c.id === card.column_id)
  const isDone = columns.length > 0 && card.column_id === columns[columns.length - 1].id

  return (
    <div className="mx-auto flex h-full max-w-2xl flex-col">
      <header className="sticky top-0 z-10 flex items-center gap-2 bg-canvas/90 px-3 py-2 backdrop-blur">
        <button onClick={() => nav(-1)} aria-label="뒤로" className="rounded-lg p-1 text-muted active:bg-line">
          <svg viewBox="0 0 24 24" className="h-6 w-6" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><path d="M15 18l-6-6 6-6" /></svg>
        </button>
        <span className="min-w-0 flex-1 truncate text-xs text-faint">
          {board.name} <span className="mx-1">›</span> {col?.name}
        </span>
        <SaveState state={saving} />
        <button
          onClick={async () => { if (await confirm({ title: '카드를 삭제할까요?', body: '되돌릴 수 없어요.', confirmLabel: '삭제', danger: true })) { await api.del(`/api/cards/${cardId}`); qc.invalidateQueries({ queryKey: ['board'] }); qc.invalidateQueries({ queryKey: ['today'] }); nav(-1) } }}
          aria-label="삭제"
          className="rounded-lg p-1 text-faint active:bg-line"
        >
          <svg viewBox="0 0 24 24" className="h-5 w-5" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round"><path d="M3 6h18M8 6V4h8v2M19 6l-1 14H6L5 6" /></svg>
        </button>
      </header>

      <div className="flex-1 overflow-y-auto px-4 pb-24">
        <TitleInput initial={card.title} onChange={(v) => queueSave({ title: v })} />

        <dl className="mt-3 space-y-0.5 border-b border-line pb-3">
          <Prop icon={<IconStatus />} label="상태">
            <select
              defaultValue={card.column_id}
              onChange={(e) => queueSave({ column_id: Number(e.target.value), position: 0 })}
              className="w-full rounded-md bg-transparent px-1.5 py-1 text-sm font-medium outline-none active:bg-surface-2"
            >
              {columns.map((c) => <option key={c.id} value={c.id}>{c.name}</option>)}
            </select>
          </Prop>
          <Prop icon={<IconUser />} label="담당">
            <select
              defaultValue={card.assignee_id ?? 0}
              onChange={(e) => queueSave({ assignee_id: Number(e.target.value) })}
              className="w-full rounded-md bg-transparent px-1.5 py-1 text-sm outline-none active:bg-surface-2"
            >
              <option value={0}>비어 있음</option>
              {users.data?.map((u) => <option key={u.id} value={u.id}>{u.name}</option>)}
            </select>
          </Prop>
          <Prop icon={<IconStar />} label="중요도">
            <StarPicker value={card.priority} onChange={(n) => queueSave({ priority: n })} />
          </Prop>
          <Prop icon={<IconCalendar />} label="마감">
            <DueInput initial={card.due_at} onChange={(v) => queueSave({ due_at: v })} />
          </Prop>
          <Prop icon={<IconRepeat />} label="반복">
            <RecurPicker
              value={card.recur}
              label={card.recur_label}
              hasDue={!!card.due_at}
              onChange={(v) => queueSave({ recur: v })}
            />
          </Prop>
        </dl>
        {card.archived_at ? (
          <div className="mt-2 flex items-center gap-2 rounded-lg bg-surface-2 px-2.5 py-1.5">
            <p className="min-w-0 flex-1 text-[11px] text-muted">보관된 카드예요. 보드의 완료 칸에는 안 보여요.</p>
            <button onClick={() => setArchived(false)} disabled={archiving} className="shrink-0 rounded-md bg-surface px-2 py-1 text-xs font-medium text-ink-2 active:bg-line">
              보관 풀기
            </button>
          </div>
        ) : isDone && (
          <div className="mt-2 flex justify-end">
            <button onClick={() => setArchived(true)} disabled={archiving} className="rounded-md bg-surface-2 px-2.5 py-1 text-xs font-medium text-muted active:bg-line">
              보관하기
            </button>
          </div>
        )}
        {card.recur_parent_id && (
          <p className="mt-2 text-[11px] text-faint">이전 회차에서 이어진 카드예요</p>
        )}
        <Byline className="mt-2" by={card.created_by} at={card.created_at} />

        <div className="mt-4">
          <BlockEditor initial={card.content} onChange={(json) => queueSave({ content: json })} />
        </div>
      </div>
    </div>
  )
}

/** 반복 설정. 고를 수 있는 것만 두고 값(요일·날짜)은 마감에서 끌어온다. */
function RecurPicker({ value, label, hasDue, onChange }: {
  value: string | null
  label: string
  hasDue: boolean
  onChange: (v: string) => void
}) {
  if (!hasDue) {
    return <span className="text-sm text-faint">마감을 먼저 정해주세요</span>
  }
  const options = [
    { v: '', t: '안 함' },
    { v: 'daily', t: '매일' },
    { v: 'weekly', t: '매주' },
    { v: 'monthly', t: '매월' },
    { v: 'yearly', t: '매년' },
  ]
  const kind = value ? value.split(':')[0] : ''
  return (
    <div className="flex flex-wrap items-center gap-1">
      {options.map((o) => (
        <button
          key={o.v}
          onClick={() => onChange(o.v)}
          className={`rounded-lg px-2 py-1 text-xs font-medium ${
            kind === o.v || (o.v === '' && !value) ? 'bg-accent text-accent-ink' : 'bg-surface-2 text-muted'
          }`}
        >
          {o.t}
        </button>
      ))}
      {label && <span className="ml-1 text-xs text-faint">{label}</span>}
    </div>
  )
}

function IconRepeat() {
  return (
    <svg viewBox="0 0 24 24" className="h-4 w-4" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round">
      <path d="M17 2l4 4-4 4" /><path d="M3 11V9a4 4 0 0 1 4-4h14" />
      <path d="M7 22l-4-4 4-4" /><path d="M21 13v2a4 4 0 0 1-4 4H3" />
    </svg>
  )
}

// 마감 입력. 날짜만/시각까지 토글(전환해도 날짜는 유지).
function DueInput({ initial, onChange }: { initial: string | null; onChange: (v: string) => void }) {
  const [timed, setTimed] = useState(!!initial && hasTime(initial))
  const [v, setV] = useState(initial ?? '')

  const set = (next: string) => { setV(next); onChange(next) }
  const toggle = (withTime: boolean) => {
    setTimed(withTime)
    if (!v) return
    set(withTime ? `${v.slice(0, 10)}T09:00` : v.slice(0, 10))
  }

  return (
    <div className="flex flex-wrap items-center gap-2">
      <input
        type={timed ? 'datetime-local' : 'date'}
        value={v}
        onChange={(e) => set(e.target.value)}
        className="min-w-0 flex-1 rounded-md bg-transparent px-1.5 py-1 text-sm outline-none active:bg-surface-2"
      />
      {v && (
        <>
          <span className={`shrink-0 rounded px-1.5 py-0.5 text-[11px] font-medium ${v.slice(0, 10) < today() ? 'bg-rose-100 text-rose-600' : v.slice(0, 10) === today() ? 'bg-amber-100 text-amber-700' : 'bg-surface-2 text-muted'}`}>
            {dueLabel(v.slice(0, 10))}
          </span>
          <button
            type="button"
            onClick={() => toggle(!timed)}
            className="shrink-0 rounded-md px-1.5 py-1 text-[11px] text-muted active:bg-surface-2"
          >
            {timed ? '시간 빼기' : '+ 시간'}
          </button>
          <button type="button" onClick={() => set('')} className="shrink-0 text-[11px] text-faint">지우기</button>
        </>
      )}
    </div>
  )
}

function TitleInput({ initial, onChange }: { initial: string; onChange: (v: string) => void }) {
  const [v, setV] = useState(initial)
  return (
    <textarea
      value={v}
      rows={1}
      placeholder="제목 없음"
      onChange={(e) => {
        setV(e.target.value)
        e.target.style.height = 'auto'
        e.target.style.height = `${e.target.scrollHeight}px`
        if (e.target.value.trim()) onChange(e.target.value.trim())
      }}
      className="mt-2 w-full resize-none bg-transparent text-2xl font-bold leading-tight outline-none placeholder:text-ghost"
    />
  )
}

function Prop({ icon, label, children }: { icon: React.ReactNode; label: string; children: React.ReactNode }) {
  return (
    <div className="flex items-center gap-2">
      <dt className="flex w-24 shrink-0 items-center gap-1.5 text-xs text-faint">
        {icon}
        {label}
      </dt>
      <dd className="min-w-0 flex-1">{children}</dd>
    </div>
  )
}

function SaveState({ state }: { state: 'idle' | 'saving' | 'saved' | 'error' }) {
  if (state === 'idle') return null
  const text = state === 'saving' ? '저장 중…' : state === 'saved' ? '저장됨' : '저장 실패'
  const cls = state === 'error' ? 'text-rose-500' : 'text-faint'
  return <span className={`shrink-0 text-[11px] ${cls}`}>{text}</span>
}

const iconCls = 'h-3.5 w-3.5'
function IconStatus() {
  return <svg viewBox="0 0 24 24" className={iconCls} fill="none" stroke="currentColor" strokeWidth="2"><circle cx="12" cy="12" r="9" /><path d="M12 7v5l3 2" strokeLinecap="round" /></svg>
}
function IconUser() {
  return <svg viewBox="0 0 24 24" className={iconCls} fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round"><circle cx="12" cy="8" r="3.5" /><path d="M5 20c0-3.3 3.1-6 7-6s7 2.7 7 6" /></svg>
}
function IconStar() {
  return <svg viewBox="0 0 24 24" className={iconCls} fill="none" stroke="currentColor" strokeWidth="2" strokeLinejoin="round"><path d="M12 3l2.6 5.6 6 .8-4.4 4.2 1.1 6-5.3-2.9L6.7 19.6l1.1-6L3.4 9.4l6-.8z" /></svg>
}
function IconCalendar() {
  return <svg viewBox="0 0 24 24" className={iconCls} fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round"><rect x="3" y="5" width="18" height="16" rx="2" /><path d="M3 10h18M8 3v4M16 3v4" /></svg>
}

