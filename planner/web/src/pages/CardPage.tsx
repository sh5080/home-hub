import { useEffect, useRef, useState } from 'react'
import { useNavigate, useParams } from 'react-router'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { api, type Card, type Column } from '../api'
import { useUsers } from '../lib/hooks'
import { dueLabel, hasTime, today } from '../lib/date'
import BlockEditor from '../components/BlockEditor'
import { StarPicker } from '../components/SortToggle'
import { SkeletonList } from '../components/ui'

// 카드 상세 = 전체 페이지(노션 방식). 바텀시트가 아니라 라우트라서 뒤로가기,
// 링크 공유, 스크롤이 자연스럽다.
//
// 저장 버튼이 없다. 제목·속성·본문 모두 바뀌면 800ms 뒤 자동 저장하고,
// 상단에 상태만 표시한다.
export default function CardPage() {
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

  // 변경분을 모아 한 번에 보낸다. 타이핑 중 요청이 쌓이지 않게.
  function queueSave(patch: Record<string, unknown>) {
    pending.current = { ...pending.current, ...patch }
    setSaving('saving')
    window.clearTimeout(timer.current)
    timer.current = window.setTimeout(async () => {
      const body = pending.current
      pending.current = {}
      try {
        await api.patch(`/api/cards/${cardId}`, body)
        setSaving('saved')
        // 목록 쪽 캐시는 무효화하되 이 페이지는 건드리지 않는다
        // (편집 중 초기 내용이 리셋되면 커서가 튄다).
        qc.invalidateQueries({ queryKey: ['board'] })
        qc.invalidateQueries({ queryKey: ['today'] })
        qc.invalidateQueries({ queryKey: ['calendar'] })
      } catch {
        setSaving('error')
      }
    }, 800)
  }

  useEffect(() => () => window.clearTimeout(timer.current), [])

  if (q.isPending) {
    return (
      <div className="mx-auto max-w-2xl space-y-4 p-4">
        <div className="h-8 w-2/3 animate-pulse rounded bg-slate-200" />
        <SkeletonList rows={3} />
        <div className="h-32 animate-pulse rounded-xl bg-slate-100" />
      </div>
    )
  }
  if (q.isError || !q.data) return <div className="p-6 text-rose-500">카드를 불러올 수 없어요</div>

  const { card, board, columns } = q.data
  const col = columns.find((c) => c.id === card.column_id)

  return (
    <div className="mx-auto flex h-full max-w-2xl flex-col">
      {/* 상단 바: 뒤로 + 브레드크럼 + 저장 상태 */}
      <header className="sticky top-0 z-10 flex items-center gap-2 bg-slate-50/90 px-3 py-2 backdrop-blur">
        <button onClick={() => nav(-1)} aria-label="뒤로" className="rounded-lg p-1 text-slate-500 active:bg-slate-200">
          <svg viewBox="0 0 24 24" className="h-6 w-6" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><path d="M15 18l-6-6 6-6" /></svg>
        </button>
        <span className="min-w-0 flex-1 truncate text-xs text-slate-400">
          {board.name} <span className="mx-1">›</span> {col?.name}
        </span>
        <SaveState state={saving} />
        <button
          onClick={async () => { if (confirm('카드를 삭제할까요?')) { await api.del(`/api/cards/${cardId}`); qc.invalidateQueries({ queryKey: ['board'] }); qc.invalidateQueries({ queryKey: ['today'] }); nav(-1) } }}
          aria-label="삭제"
          className="rounded-lg p-1 text-slate-400 active:bg-slate-200"
        >
          <svg viewBox="0 0 24 24" className="h-5 w-5" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round"><path d="M3 6h18M8 6V4h8v2M19 6l-1 14H6L5 6" /></svg>
        </button>
      </header>

      <div className="flex-1 overflow-y-auto px-4 pb-24">
        {/* 제목 = H1 */}
        <TitleInput initial={card.title} onChange={(v) => queueSave({ title: v })} />

        {/* 속성 */}
        <dl className="mt-3 space-y-0.5 border-b border-slate-200 pb-3">
          <Prop icon={<IconStatus />} label="상태">
            <select
              defaultValue={card.column_id}
              onChange={(e) => queueSave({ column_id: Number(e.target.value), position: 0 })}
              className="w-full rounded-md bg-transparent px-1.5 py-1 text-sm font-medium outline-none active:bg-slate-100"
            >
              {columns.map((c) => <option key={c.id} value={c.id}>{c.name}</option>)}
            </select>
          </Prop>
          <Prop icon={<IconUser />} label="담당">
            <select
              defaultValue={card.assignee_id ?? 0}
              onChange={(e) => queueSave({ assignee_id: Number(e.target.value) })}
              className="w-full rounded-md bg-transparent px-1.5 py-1 text-sm outline-none active:bg-slate-100"
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
        </dl>

        {/* 본문 */}
        <div className="mt-4">
          <BlockEditor initial={card.content} onChange={(json) => queueSave({ content: json })} />
        </div>
      </div>
    </div>
  )
}

// 마감 입력. 날짜만 쓸지 시각까지 쓸지 토글한다 — 전환해도 날짜는 유지한다.
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
        className="min-w-0 flex-1 rounded-md bg-transparent px-1.5 py-1 text-sm outline-none active:bg-slate-100"
      />
      {v && (
        <>
          <span className={`shrink-0 rounded px-1.5 py-0.5 text-[11px] font-medium ${v.slice(0, 10) < today() ? 'bg-rose-100 text-rose-600' : v.slice(0, 10) === today() ? 'bg-amber-100 text-amber-700' : 'bg-slate-100 text-slate-500'}`}>
            {dueLabel(v.slice(0, 10))}
          </span>
          <button
            type="button"
            onClick={() => toggle(!timed)}
            className="shrink-0 rounded-md px-1.5 py-1 text-[11px] text-slate-500 active:bg-slate-100"
          >
            {timed ? '시간 빼기' : '+ 시간'}
          </button>
          <button type="button" onClick={() => set('')} className="shrink-0 text-[11px] text-slate-400">지우기</button>
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
      className="mt-2 w-full resize-none bg-transparent text-2xl font-bold leading-tight outline-none placeholder:text-slate-300"
    />
  )
}

function Prop({ icon, label, children }: { icon: React.ReactNode; label: string; children: React.ReactNode }) {
  return (
    <div className="flex items-center gap-2">
      <dt className="flex w-24 shrink-0 items-center gap-1.5 text-xs text-slate-400">
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
  const cls = state === 'error' ? 'text-rose-500' : 'text-slate-400'
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

