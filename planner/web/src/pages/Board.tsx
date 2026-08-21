import { useEffect, useState } from 'react'
import { useNavigate, useParams } from 'react-router'
import { useQueryClient } from '@tanstack/react-query'
import {
  DndContext, DragOverlay, PointerSensor, useSensor, useSensors, useDroppable,
  pointerWithin, rectIntersection, getFirstCollision,
  type CollisionDetection, type DragEndEvent, type DragStartEvent,
} from '@dnd-kit/core'
import { SortableContext, useSortable, verticalListSortingStrategy, arrayMove } from '@dnd-kit/sortable'
import { CSS } from '@dnd-kit/utilities'
import { api, type BoardDetail, type Card, type Column } from '../api'
import { useBoard, useBoards, useInvalidating, useUsers } from '../lib/hooks'
import { fmtDue, today } from '../lib/date'
import { Avatar, Button, Input, PageHeader, Sheet } from '../components/ui'
import SortToggle, { Stars } from '../components/SortToggle'
import type { SortMode } from '../lib/hooks'

// 손가락(포인터)이 실제로 어느 영역 안에 있는지로 판정한다.
// closestCorners는 끌고 있는 카드의 모서리와 후보들의 거리를 재는데, 컬럼이
// 세로로 같은 높이라 손가락이 왼쪽 컬럼 위에 있어도 다른 컬럼이 더 가깝게
// 계산되는 일이 생긴다. pointerWithin이 이 배치에 맞다.
// 포인터가 어느 영역에도 안 걸칠 때(컬럼 사이 여백)만 사각형 교차로 떨어진다.
const collisionDetection: CollisionDetection = (args) => {
  const pointer = pointerWithin(args)
  if (getFirstCollision(pointer)) return pointer
  return rectIntersection(args)
}

const LAST_BOARD_KEY = 'planner.lastBoard'
const VIEW_KEY = 'planner.boardView'
const SORT_KEY = 'planner.boardSort'
type View = 'kanban' | 'backlog'

// 할 일 탭은 목록이 아니라 바로 칸반/백로그다. /boards 는 마지막에 본 보드(없으면 첫 보드).
export default function Board() {
  const params = useParams()
  const nav = useNavigate()
  const boards = useBoards()

  let id = params.id ? Number(params.id) : NaN
  if (!id) {
    let last = NaN
    try { last = Number(localStorage.getItem(LAST_BOARD_KEY)) } catch { /* private mode 등 */ }
    id = boards.data?.some((b) => b.id === last) ? last : (boards.data?.[0]?.id ?? NaN)
  }
  useEffect(() => {
    if (id) try { localStorage.setItem(LAST_BOARD_KEY, String(id)) } catch { /* ignore */ }
  }, [id])

  if (boards.isPending) return <div className="p-6 text-slate-400">불러오는 중…</div>
  if (!id) return <NoBoards />
  return <Kanban id={id} onSwitch={(bid) => nav(`/boards/${bid}`)} />
}

function NoBoards() {
  const create = useInvalidating((n: string) => api.post('/api/boards', { name: n }), [['boards']])
  return (
    <div className="mx-auto max-w-lg">
      <PageHeader title="할 일" />
      <div className="p-6 text-center">
        <p className="text-slate-400">보드가 없어요</p>
        <Button className="mt-4" onClick={() => create.mutate('할 일')}>할 일 보드 만들기</Button>
      </div>
    </div>
  )
}

function Kanban({ id, onSwitch }: { id: number; onSwitch: (id: number) => void }) {
  const qc = useQueryClient()
  const boards = useBoards()
  const [sort, setSortState] = useState<SortMode>(() => {
    try { return (localStorage.getItem(SORT_KEY) as SortMode) || 'manual' } catch { return 'manual' }
  })
  const setSort = (m: SortMode) => { setSortState(m); try { localStorage.setItem(SORT_KEY, m) } catch { /* ignore */ } }
  // 정렬이 켜져 있으면 서버가 매번 다시 정렬하므로 드래그로 바꾼 순서가 남지 않는다.
  const canReorder = sort === 'manual'
  const board = useBoard(id, sort)
  const users = useUsers()
  const nav = useNavigate()
  const [adding, setAdding] = useState<Column | null>(null)
  const [menu, setMenu] = useState(false)
  const [switcher, setSwitcher] = useState(false)
  const [dragging, setDragging] = useState<Card | null>(null)
  // 펼친 컬럼. null이면 균등 분할. 좁은 화면에서 3분할은 카드가 답답해서
  // 한 컬럼만 크게 보는 모드를 둔다.
  const [expanded, setExpanded] = useState<number | null>(null)
  const [view, setViewState] = useState<View>(() => {
    try { return (localStorage.getItem(VIEW_KEY) as View) || 'kanban' } catch { return 'kanban' }
  })
  const setView = (v: View) => { setViewState(v); try { localStorage.setItem(VIEW_KEY, v) } catch { /* ignore */ } }

  const keys = [['board', id], ['boards'], ['calendar'], ['today']]
  const addCard = useInvalidating(({ col, title }: { col: number; title: string }) => api.post<Card>(`/api/columns/${col}/cards`, { title }), keys)
  const patchCard = useInvalidating(({ cardId, ...body }: { cardId: number } & Record<string, unknown>) => api.patch(`/api/cards/${cardId}`, body), keys)
  const addColumn = useInvalidating((name: string) => api.post(`/api/boards/${id}/columns`, { name }), keys)
  const renameBoard = useInvalidating((name: string) => api.patch(`/api/boards/${id}`, { name }), keys)
  const delBoard = useInvalidating(() => api.del(`/api/boards/${id}`), [['boards'], ['today']])
  const createBoard = useInvalidating((n: string) => api.post<{ id: number }>('/api/boards', { name: n }), [['boards']])

  // 드래그는 카드 오른쪽 그립에서만 시작한다(그립은 touch-action:none). 카드 본문은 스크롤/탭.
  // 길게 누르기 방식은 iOS에서 홀드 중 미세한 touchmove가 스크롤로 확정돼 드롭이 튀었다.
  const sensors = useSensors(useSensor(PointerSensor, { activationConstraint: { distance: 4 } }))

  if (board.isPending) return <div className="p-6 text-slate-400">불러오는 중…</div>
  if (board.isError || !board.data) return <div className="p-6 text-rose-500">보드를 불러올 수 없어요</div>

  const { columns } = board.data
  const userName = (uid: number | null) => users.data?.find((u) => u.id === uid)?.name
  const many = (boards.data?.length ?? 0) > 1

  const findCard = (cardId: number) => {
    for (const col of columns) {
      const i = col.cards.findIndex((c) => c.id === cardId)
      if (i >= 0) return { col, index: i, card: col.cards[i] }
    }
    return null
  }

  function onDragStart(e: DragStartEvent) {
    const found = findCard(Number(String(e.active.id).slice(5)))
    setDragging(found?.card ?? null)
  }

  function onDragEnd(e: DragEndEvent) {
    setDragging(null)
    const { active, over } = e
    if (!over) return
    const cardId = Number(String(active.id).slice(5))
    const src = findCard(cardId)
    if (!src) return

    // over는 카드('card-N')거나 컬럼('col-N')
    let toCol: Column | undefined
    let toIndex: number
    const overId = String(over.id)
    if (overId.startsWith('col-')) {
      toCol = columns.find((c) => c.id === Number(overId.slice(4)))
      toIndex = toCol?.cards.length ?? 0
    } else {
      const dst = findCard(Number(overId.slice(5)))
      if (!dst) return
      toCol = dst.col
      toIndex = dst.index
    }
    if (!toCol) return
    if (toCol.id === src.col.id) {
      // 정렬이 켜져 있으면 같은 컬럼 안 순서 변경은 남지 않는다 — 무시한다.
      if (!canReorder || toIndex === src.index) return
    }

    // 낙관적 갱신: 서버와 같은 splice 의미론으로 로컬 상태를 먼저 바꾼다.
    // 같은 컬럼이면 arrayMove가 곧 최종 인덱스 = 서버의 newPos.
    qc.setQueryData<BoardDetail>(['board', id], (old) => {
      if (!old) return old
      const cols = old.columns.map((c) => ({ ...c, cards: [...c.cards] }))
      const from = cols.find((c) => c.id === src.col.id)!
      const to = cols.find((c) => c.id === toCol!.id)!
      if (from === to) {
        to.cards = arrayMove(to.cards, src.index, toIndex)
      } else {
        const [moved] = from.cards.splice(src.index, 1)
        to.cards.splice(Math.min(toIndex, to.cards.length), 0, { ...moved, column_id: to.id })
      }
      return { ...old, columns: cols }
    })
    patchCard.mutate({ cardId, column_id: toCol.id, position: toIndex })
  }

  return (
    <div className="flex h-full flex-col">
      <PageHeader
        title={
          <button onClick={() => setSwitcher(true)} className="flex items-center gap-1 text-left">
            <span className="truncate">{board.data.board.name}</span>
            {many && <svg viewBox="0 0 24 24" className="h-4 w-4 shrink-0 text-slate-400" fill="none" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round"><path d="M6 9l6 6 6-6" /></svg>}
          </button>
        }
        right={
          <button onClick={() => setMenu(true)} aria-label="메뉴" className="rounded-lg p-1 text-slate-500 active:bg-slate-200">
            <svg viewBox="0 0 24 24" className="h-6 w-6" fill="currentColor"><circle cx="5" cy="12" r="2" /><circle cx="12" cy="12" r="2" /><circle cx="19" cy="12" r="2" /></svg>
          </button>
        }
      />

      {/* 칸반 / 백로그 + 정렬 */}
      <div className="flex items-center gap-2 px-4 pt-2">
        <div className="flex flex-1 rounded-xl bg-slate-200 p-0.5 text-sm font-medium">
          {(['kanban', 'backlog'] as const).map((v) => (
            <button
              key={v}
              onClick={() => setView(v)}
              className={`flex-1 rounded-[10px] py-1.5 transition ${view === v ? 'bg-white text-slate-900 shadow-sm' : 'text-slate-500'}`}
            >
              {v === 'kanban' ? '칸반' : '백로그'}
            </button>
          ))}
        </div>
        <SortToggle value={sort} onChange={setSort} />
      </div>
      {!canReorder && (
        <p className="px-4 pt-1.5 text-[11px] text-slate-400">정렬 중에는 카드를 끌어 순서를 바꿀 수 없어요 · 수동으로 바꾸면 가능</p>
      )}

      {view === 'kanban' ? (
        <DndContext sensors={sensors} collisionDetection={collisionDetection} onDragStart={onDragStart} onDragEnd={onDragEnd} onDragCancel={() => setDragging(null)}>
          {/* 컬럼을 한 화면에 나란히. 3개면 3분할, 5개 이상이면 가로 스크롤. */}
          <div className="flex flex-1 gap-2 overflow-x-auto px-2 py-3">
            {columns.map((col) => (
              <ColumnView
                key={col.id}
                col={col}
                narrow={columns.length >= 3}
                collapsed={expanded !== null && expanded !== col.id}
                expanded={expanded === col.id}
                onToggleExpand={() => setExpanded(expanded === col.id ? null : col.id)}
                userName={userName}
                onAdd={() => setAdding(col)}
                onOpen={(c) => nav(`/cards/${c.id}`)}
              />
            ))}
          </div>
          <DragOverlay dropAnimation={null}>
            {dragging && <CardTile card={dragging} userName={userName} lifted />}
          </DragOverlay>
        </DndContext>
      ) : (
        <Backlog columns={columns} userName={userName} onAdd={() => setAdding(columns[0])} onOpen={(c) => nav(`/cards/${c.id}`)} />
      )}

      {/* 카드 추가: 제목만 받고 바로 상세 페이지로 — 나머지는 거기서 채운다 */}
      <Sheet open={!!adding} onClose={() => setAdding(null)} title={adding ? `${adding.name}에 추가` : ''}>
        {adding && <QuickAdd colName={adding.name} onSubmit={async (title) => {
          const c = await addCard.mutateAsync({ col: adding.id, title })
          setAdding(null)
          nav(`/cards/${c.id}`)
        }} onClose={() => setAdding(null)} />}
      </Sheet>

      {/* 보드 전환 */}
      <Sheet open={switcher} onClose={() => setSwitcher(false)} title="보드">
        <ul className="space-y-1">
          {boards.data?.map((b) => (
            <li key={b.id}>
              <button
                onClick={() => { onSwitch(b.id); setSwitcher(false) }}
                className={`flex w-full items-center justify-between rounded-xl px-4 py-3 text-left ${b.id === id ? 'bg-slate-900 text-white' : 'bg-slate-100'}`}
              >
                <span className="font-medium">{b.name}</span>
                <span className={`text-xs ${b.id === id ? 'text-slate-300' : 'text-slate-400'}`}>{b.card_count ?? 0}개</span>
              </button>
            </li>
          ))}
        </ul>
        <Button
          variant="ghost"
          className="mt-3 w-full"
          onClick={async () => {
            const n = prompt('새 보드 이름')
            if (!n?.trim()) return
            const b = await createBoard.mutateAsync(n.trim())
            setSwitcher(false)
            onSwitch(b.id)
          }}
        >
          + 새 보드
        </Button>
      </Sheet>

      {/* 보드 메뉴 */}
      <Sheet open={menu} onClose={() => setMenu(false)} title={board.data.board.name}>
        <div className="space-y-2">
          <Button variant="ghost" className="w-full" onClick={() => { const n = prompt('컬럼 이름'); if (n?.trim()) addColumn.mutate(n.trim()); setMenu(false) }}>+ 컬럼 추가</Button>
          <Button variant="ghost" className="w-full" onClick={() => { const n = prompt('보드 이름', board.data!.board.name); if (n?.trim()) renameBoard.mutate(n.trim()); setMenu(false) }}>이름 바꾸기</Button>
          <Button variant="danger" className="w-full" onClick={() => {
            if (confirm(`"${board.data!.board.name}" 보드와 모든 카드를 삭제할까요?`)) {
              delBoard.mutate(undefined)
              try { localStorage.removeItem(LAST_BOARD_KEY) } catch { /* ignore */ }
              setMenu(false)
              onSwitch(boards.data?.find((b) => b.id !== id)?.id ?? 0)
            }
          }}>보드 삭제</Button>
        </div>
      </Sheet>
    </div>
  )
}

// 백로그: 보드의 카드를 세로 리스트로. 완료 컬럼은 접어둔다.
// 정렬: 마감 지난 것 → 마감 있는 것(가까운 순) → 마감 없는 것(컬럼 순).
function Backlog({ columns, userName, onAdd, onOpen }: { columns: Column[]; userName: (id: number | null) => string | undefined; onAdd: () => void; onOpen: (c: Card) => void }) {
  const last = columns[columns.length - 1]
  const open = columns.slice(0, -1).flatMap((col) => col.cards.map((c) => ({ c, col })))
  const done = last ? last.cards.map((c) => ({ c, col: last })) : []
  open.sort((a, b) => {
    const ad = a.c.due_at ?? '9999', bd = b.c.due_at ?? '9999'
    if (ad !== bd) return ad < bd ? -1 : 1
    return a.col.position - b.col.position || a.c.position - b.c.position
  })
  const Row = ({ c, col, muted }: { c: Card; col: Column; muted?: boolean }) => {
    const name = c.assignee_id ? userName(c.assignee_id) : undefined
    return (
      <button onClick={() => onOpen(c)} className={`flex w-full items-start gap-3 rounded-xl bg-white p-3 text-left shadow-sm active:bg-slate-50 ${muted ? 'opacity-60' : ''}`}>
        <div className="min-w-0 flex-1">
          <p className={`text-sm font-medium leading-snug ${muted ? 'line-through' : ''}`}>{c.title}</p>
          {c.description && <p className="mt-0.5 truncate text-xs text-slate-400">{c.description}</p>}
          <div className="mt-1.5 flex flex-wrap items-center gap-1.5">
            <span className="rounded bg-slate-100 px-1.5 py-0.5 text-[10px] font-medium text-slate-500">{col.name}</span>
            <Stars n={c.priority} />
            {c.due_at && <DueBadge due={c.due_at} />}
          </div>
        </div>
        {name && <Avatar name={name} />}
      </button>
    )
  }
  return (
    <div className="mx-auto w-full max-w-lg flex-1 overflow-y-auto px-4 py-3">
      <Button variant="ghost" className="mb-3 w-full" onClick={onAdd}>+ 할 일 추가</Button>
      <ul className="space-y-2">
        {open.map(({ c, col }) => <li key={c.id}><Row c={c} col={col} /></li>)}
        {open.length === 0 && <li className="py-8 text-center text-sm text-slate-400">남은 할 일이 없어요</li>}
      </ul>
      {done.length > 0 && (
        <details className="mt-4">
          <summary className="cursor-pointer text-xs font-medium text-slate-400">{last.name} {done.length}</summary>
          <ul className="mt-2 space-y-2">
            {done.map(({ c, col }) => <li key={c.id}><Row c={c} col={col} muted /></li>)}
          </ul>
        </details>
      )}
    </div>
  )
}

function ColumnView({ col, narrow, collapsed, expanded, onToggleExpand, userName, onAdd, onOpen }: {
  col: Column
  narrow: boolean
  collapsed: boolean
  expanded: boolean
  onToggleExpand: () => void
  userName: (id: number | null) => string | undefined
  onAdd: () => void
  onOpen: (c: Card) => void
}) {
  const { setNodeRef, isOver } = useDroppable({ id: `col-${col.id}` })

  // 접힌 컬럼도 드롭 대상으로 남긴다 — 펼친 상태에서 옆 컬럼으로 카드를
  // 끌어다 놓을 수 있어야 한다.
  if (collapsed) {
    return (
      <button
        ref={setNodeRef}
        onClick={onToggleExpand}
        className={`flex w-11 shrink-0 flex-col items-center gap-2 rounded-2xl py-3 transition-colors ${isOver ? 'bg-slate-300' : 'bg-slate-100'}`}
      >
        <span className="rounded-full bg-white px-1.5 text-[11px] font-semibold text-slate-500">{col.cards.length}</span>
        <span className="text-xs font-bold text-slate-600" style={{ writingMode: 'vertical-rl' }}>
          {col.name}
        </span>
      </button>
    )
  }

  const width = expanded ? 'flex-1' : narrow ? 'min-w-[30%] flex-1' : 'min-w-[45%] flex-1'
  return (
    <section
      ref={setNodeRef}
      className={`flex min-w-0 flex-col rounded-2xl transition-colors ${width} sm:min-w-56 ${isOver ? 'bg-slate-200' : 'bg-slate-100'}`}
    >
      {/* 헤더를 누르면 이 컬럼만 크게 — 다시 누르면 균등 분할로 */}
      <button
        onClick={onToggleExpand}
        className="flex items-center justify-between px-2.5 pt-2.5 pb-1.5 active:opacity-60"
      >
        <h2 className="truncate text-xs font-bold text-slate-700">{col.name}</h2>
        <span className="ml-1 flex shrink-0 items-center gap-1 text-[11px] text-slate-400">
          {col.cards.length}
          <svg viewBox="0 0 24 24" className="h-3 w-3" fill="none" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round">
            {expanded ? <path d="M9 4v6H3M15 20v-6h6" /> : <path d="M4 9V3h6M20 15v6h-6" />}
          </svg>
        </span>
      </button>
      <SortableContext items={col.cards.map((c) => `card-${c.id}`)} strategy={verticalListSortingStrategy}>
        <ul className="flex-1 space-y-1.5 overflow-y-auto px-1.5 pb-1">
          {col.cards.map((c) => (
            <SortableCard key={c.id} card={c} userName={userName} onOpen={onOpen} wide={expanded} />
          ))}
          {col.cards.length === 0 && <li className="h-10" />}
        </ul>
      </SortableContext>
      <button onClick={onAdd} className="m-1.5 rounded-lg py-1.5 text-[13px] font-medium text-slate-500 active:bg-white">+ 추가</button>
    </section>
  )
}

function SortableCard({ card, userName, onOpen, wide }: { card: Card; userName: (id: number | null) => string | undefined; onOpen: (c: Card) => void; wide?: boolean }) {
  const { attributes, listeners, setNodeRef, setActivatorNodeRef, transform, transition, isDragging } = useSortable({ id: `card-${card.id}` })
  return (
    <li ref={setNodeRef} style={{ transform: CSS.Transform.toString(transform), transition }} className={isDragging ? 'opacity-30' : ''}>
      <CardTile
        card={card}
        userName={userName}
        wide={wide}
        onClick={() => onOpen(card)}
        grip={
          <button
            ref={setActivatorNodeRef}
            {...attributes}
            {...listeners}
            aria-label="끌어서 이동"
            className="-my-2 -mr-2 flex w-7 shrink-0 cursor-grab items-center justify-center self-stretch rounded-r-lg text-slate-300 active:cursor-grabbing active:bg-slate-100"
            style={{ touchAction: 'none' }}
          >
            <svg viewBox="0 0 24 24" className="h-4 w-4" fill="currentColor"><circle cx="9" cy="6" r="1.6" /><circle cx="15" cy="6" r="1.6" /><circle cx="9" cy="12" r="1.6" /><circle cx="15" cy="12" r="1.6" /><circle cx="9" cy="18" r="1.6" /><circle cx="15" cy="18" r="1.6" /></svg>
          </button>
        }
      />
    </li>
  )
}

function CardTile({ card, userName, onClick, lifted, grip, wide }: { card: Card; userName: (id: number | null) => string | undefined; onClick?: () => void; lifted?: boolean; grip?: React.ReactNode; wide?: boolean }) {
  const name = card.assignee_id ? userName(card.assignee_id) : undefined
  return (
    <div className={`flex w-full select-none rounded-lg bg-white p-2 ${lifted ? 'rotate-2 shadow-xl ring-2 ring-slate-900/10' : 'shadow-sm'}`}>
      <button onClick={onClick} className="min-w-0 flex-1 text-left active:opacity-70">
        <p className={`font-medium leading-snug break-words ${wide ? 'text-sm' : 'text-[13px]'}`}>{card.title}</p>
        {wide && card.description && <p className="mt-0.5 line-clamp-2 text-xs text-slate-400">{card.description}</p>}
        {(card.due_at || name || card.priority > 0) && (
          <div className="mt-1.5 flex flex-wrap items-center gap-1">
            <Stars n={card.priority} />
            {card.due_at && <DueBadge due={card.due_at} />}
            {name && <Avatar name={name} />}
          </div>
        )}
      </button>
      {grip}
    </div>
  )
}

function DueBadge({ due }: { due: string }) {
  const day = due.slice(0, 10)
  const overdue = day < today()
  const isToday = day === today()
  const cls = overdue ? 'bg-rose-100 text-rose-600' : isToday ? 'bg-amber-100 text-amber-700' : 'bg-slate-100 text-slate-500'
  return <span className={`rounded px-1 py-0.5 text-[10px] font-medium ${cls}`}>{fmtDue(due)}</span>
}

// 제목만 받는 빠른 추가. 상세는 카드 페이지에서 채운다.
function QuickAdd({ colName, onSubmit, onClose }: { colName: string; onSubmit: (title: string) => void; onClose: () => void }) {
  const [title, setTitle] = useState('')
  const [busy, setBusy] = useState(false)
  return (
    <form
      onSubmit={(e) => { e.preventDefault(); if (title.trim()) { setBusy(true); onSubmit(title.trim()) } }}
      className="space-y-3"
    >
      <Input autoFocus placeholder={`${colName}에 추가할 일`} value={title} onChange={(e) => setTitle(e.target.value)} className="text-lg font-semibold" />
      <div className="flex justify-end gap-2">
        <Button type="button" variant="ghost" onClick={onClose}>취소</Button>
        <Button type="submit" disabled={!title.trim() || busy}>{busy ? '여는 중…' : '추가하고 열기'}</Button>
      </div>
    </form>
  )
}
