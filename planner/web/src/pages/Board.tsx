import { useEffect, useState } from 'react'
import { useNavigate, useParams } from 'react-router'
import { useQueryClient } from '@tanstack/react-query'
import {
  DndContext, DragOverlay, PointerSensor, useSensor, useSensors, useDroppable, closestCorners,
  type DragEndEvent, type DragStartEvent,
} from '@dnd-kit/core'
import { SortableContext, useSortable, verticalListSortingStrategy, arrayMove } from '@dnd-kit/sortable'
import { CSS } from '@dnd-kit/utilities'
import { api, type BoardDetail, type Card, type Column } from '../api'
import { useBoard, useBoards, useInvalidating, useUsers } from '../lib/hooks'
import { dueLabel, today } from '../lib/date'
import { Avatar, Button, Field, Input, PageHeader, Sheet, Textarea } from '../components/ui'

const LAST_BOARD_KEY = 'planner.lastBoard'
const VIEW_KEY = 'planner.boardView'
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

type CardBody = { title: string; description: string; due_date: string; assignee_id: number }

function Kanban({ id, onSwitch }: { id: number; onSwitch: (id: number) => void }) {
  const qc = useQueryClient()
  const boards = useBoards()
  const board = useBoard(id)
  const users = useUsers()
  const [editing, setEditing] = useState<Card | null>(null)
  const [adding, setAdding] = useState<Column | null>(null)
  const [menu, setMenu] = useState(false)
  const [switcher, setSwitcher] = useState(false)
  const [dragging, setDragging] = useState<Card | null>(null)
  const [view, setViewState] = useState<View>(() => {
    try { return (localStorage.getItem(VIEW_KEY) as View) || 'kanban' } catch { return 'kanban' }
  })
  const setView = (v: View) => { setViewState(v); try { localStorage.setItem(VIEW_KEY, v) } catch { /* ignore */ } }

  const keys = [['board', id], ['boards'], ['calendar'], ['today']]
  const addCard = useInvalidating(({ col, ...body }: { col: number } & CardBody) => api.post(`/api/columns/${col}/cards`, body), keys)
  const patchCard = useInvalidating(({ cardId, ...body }: { cardId: number } & Record<string, unknown>) => api.patch(`/api/cards/${cardId}`, body), keys)
  const delCard = useInvalidating((cardId: number) => api.del(`/api/cards/${cardId}`), keys)
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
    if (toCol.id === src.col.id && toIndex === src.index) return

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

      {/* 칸반 / 백로그 전환 */}
      <div className="px-4 pt-2">
        <div className="flex rounded-xl bg-slate-200 p-0.5 text-sm font-medium">
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
      </div>

      {view === 'kanban' ? (
        <DndContext sensors={sensors} collisionDetection={closestCorners} onDragStart={onDragStart} onDragEnd={onDragEnd} onDragCancel={() => setDragging(null)}>
          {/* 컬럼을 한 화면에 나란히. 3개면 3분할, 5개 이상이면 가로 스크롤. */}
          <div className="flex flex-1 gap-2 overflow-x-auto px-2 py-3">
            {columns.map((col) => (
              <ColumnView
                key={col.id}
                col={col}
                narrow={columns.length >= 3}
                userName={userName}
                onAdd={() => setAdding(col)}
                onOpen={setEditing}
              />
            ))}
          </div>
          <DragOverlay dropAnimation={null}>
            {dragging && <CardTile card={dragging} userName={userName} lifted />}
          </DragOverlay>
        </DndContext>
      ) : (
        <Backlog columns={columns} userName={userName} onAdd={() => setAdding(columns[0])} onOpen={setEditing} />
      )}

      {/* 카드 추가 */}
      <Sheet open={!!adding} onClose={() => setAdding(null)} title={adding ? `${adding.name}에 추가` : ''}>
        {adding && (
          <CardForm
            key={`new-${adding.id}`}
            users={users.data ?? []}
            onSave={(body) => { addCard.mutate({ col: adding.id, ...body }); setAdding(null) }}
            onClose={() => setAdding(null)}
          />
        )}
      </Sheet>

      {/* 카드 편집 */}
      <Sheet open={!!editing} onClose={() => setEditing(null)}>
        {editing && (
          <CardForm
            key={editing.id}
            card={editing}
            columns={columns}
            users={users.data ?? []}
            onSave={(body) => { patchCard.mutate({ cardId: editing.id, ...body }); setEditing(null) }}
            onMove={(colId, pos) => { patchCard.mutate({ cardId: editing.id, column_id: colId, position: pos }); setEditing(null) }}
            onDelete={() => { if (confirm('카드를 삭제할까요?')) { delCard.mutate(editing.id); setEditing(null) } }}
            onClose={() => setEditing(null)}
          />
        )}
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
    const ad = a.c.due_date ?? '9999', bd = b.c.due_date ?? '9999'
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
            {c.due_date && <DueBadge date={c.due_date} />}
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

function ColumnView({ col, narrow, userName, onAdd, onOpen }: { col: Column; narrow: boolean; userName: (id: number | null) => string | undefined; onAdd: () => void; onOpen: (c: Card) => void }) {
  const { setNodeRef, isOver } = useDroppable({ id: `col-${col.id}` })
  return (
    <section
      ref={setNodeRef}
      className={`flex min-w-0 flex-1 flex-col rounded-2xl transition-colors ${narrow ? 'min-w-[30%]' : 'min-w-[45%]'} sm:min-w-56 ${isOver ? 'bg-slate-200' : 'bg-slate-100'}`}
    >
      <header className="flex items-center justify-between px-2.5 pt-2.5 pb-1.5">
        <h2 className="truncate text-xs font-bold text-slate-700">{col.name}</h2>
        <span className="ml-1 shrink-0 text-[11px] text-slate-400">{col.cards.length}</span>
      </header>
      <SortableContext items={col.cards.map((c) => `card-${c.id}`)} strategy={verticalListSortingStrategy}>
        <ul className="flex-1 space-y-1.5 overflow-y-auto px-1.5 pb-1">
          {col.cards.map((c) => (
            <SortableCard key={c.id} card={c} userName={userName} onOpen={onOpen} />
          ))}
          {col.cards.length === 0 && <li className="h-10" />}
        </ul>
      </SortableContext>
      <button onClick={onAdd} className="m-1.5 rounded-lg py-1.5 text-[13px] font-medium text-slate-500 active:bg-white">+ 추가</button>
    </section>
  )
}

function SortableCard({ card, userName, onOpen }: { card: Card; userName: (id: number | null) => string | undefined; onOpen: (c: Card) => void }) {
  const { attributes, listeners, setNodeRef, setActivatorNodeRef, transform, transition, isDragging } = useSortable({ id: `card-${card.id}` })
  return (
    <li ref={setNodeRef} style={{ transform: CSS.Transform.toString(transform), transition }} className={isDragging ? 'opacity-30' : ''}>
      <CardTile
        card={card}
        userName={userName}
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

function CardTile({ card, userName, onClick, lifted, grip }: { card: Card; userName: (id: number | null) => string | undefined; onClick?: () => void; lifted?: boolean; grip?: React.ReactNode }) {
  const name = card.assignee_id ? userName(card.assignee_id) : undefined
  return (
    <div className={`flex w-full select-none rounded-lg bg-white p-2 ${lifted ? 'rotate-2 shadow-xl ring-2 ring-slate-900/10' : 'shadow-sm'}`}>
      <button onClick={onClick} className="min-w-0 flex-1 text-left active:opacity-70">
        <p className="text-[13px] font-medium leading-snug break-words">{card.title}</p>
        {(card.due_date || name) && (
          <div className="mt-1.5 flex flex-wrap items-center gap-1">
            {card.due_date && <DueBadge date={card.due_date} />}
            {name && <Avatar name={name} />}
          </div>
        )}
      </button>
      {grip}
    </div>
  )
}

function DueBadge({ date }: { date: string }) {
  const overdue = date < today()
  const isToday = date === today()
  const cls = overdue ? 'bg-rose-100 text-rose-600' : isToday ? 'bg-amber-100 text-amber-700' : 'bg-slate-100 text-slate-500'
  return <span className={`rounded px-1 py-0.5 text-[10px] font-medium ${cls}`}>{dueLabel(date)}</span>
}

// 추가(card 없음)와 편집(card 있음) 겸용 폼.
function CardForm({ card, columns, users, onSave, onMove, onDelete, onClose }: {
  card?: Card
  columns?: Column[]
  users: { id: number; name: string }[]
  onSave: (body: CardBody) => void
  onMove?: (colId: number, pos: number) => void
  onDelete?: () => void
  onClose: () => void
}) {
  const [title, setTitle] = useState(card?.title ?? '')
  const [desc, setDesc] = useState(card?.description ?? '')
  const [due, setDue] = useState(card?.due_date ?? '')
  const [assignee, setAssignee] = useState(card?.assignee_id ?? 0)
  const dirty = !card || title !== card.title || desc !== card.description || due !== (card.due_date ?? '') || assignee !== (card.assignee_id ?? 0)

  return (
    <form
      onSubmit={(e) => { e.preventDefault(); if (title.trim()) onSave({ title: title.trim(), description: desc, due_date: due, assignee_id: assignee }) }}
      className="space-y-3"
    >
      <Input autoFocus={!card} placeholder="할 일" value={title} onChange={(e) => setTitle(e.target.value)} className="text-lg font-semibold" />
      <Textarea rows={3} placeholder="설명 (선택)" value={desc} onChange={(e) => setDesc(e.target.value)} />
      {/* 세로로 쌓는다 — iOS date input은 폭이 고정이라 2열 그리드에서 옆 칸을 침범한다 */}
      <Field label="마감일">
        <Input type="date" value={due} onChange={(e) => setDue(e.target.value)} />
      </Field>
      <Field label="담당">
        <select value={assignee} onChange={(e) => setAssignee(Number(e.target.value))} className="w-full rounded-xl border border-slate-200 bg-white px-3 py-2.5 text-base">
          <option value={0}>없음</option>
          {users.map((u) => <option key={u.id} value={u.id}>{u.name}</option>)}
        </select>
      </Field>

      {card && columns && onMove && (
        <Field label="이동">
          <div className="flex flex-wrap gap-2">
            {columns.map((c) => (
              <button
                key={c.id}
                type="button"
                disabled={c.id === card.column_id}
                onClick={() => onMove(c.id, c.cards.length)}
                className="rounded-lg bg-slate-100 px-3 py-1.5 text-sm font-medium text-slate-700 disabled:bg-slate-900 disabled:text-white"
              >
                {c.name}
              </button>
            ))}
          </div>
        </Field>
      )}

      <div className="flex gap-2 pt-1">
        {onDelete && <Button type="button" variant="danger" onClick={onDelete}>삭제</Button>}
        <div className="flex-1" />
        <Button type="button" variant="ghost" onClick={onClose}>닫기</Button>
        <Button type="submit" disabled={!dirty || !title.trim()}>{card ? '저장' : '추가'}</Button>
      </div>
    </form>
  )
}
