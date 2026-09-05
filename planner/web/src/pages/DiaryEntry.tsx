import { useCallback, useEffect, useRef, useState } from 'react'
import { useNavigate, useParams, useSearchParams } from 'react-router'
import { useQueryClient } from '@tanstack/react-query'
import { api, uploadMedia, type DiaryEntry, type DiaryRef } from '../api'
import { useDiaryEntry, useSearch } from '../lib/hooks'
import { fmtDate, today } from '../lib/date'
import { useConfirm } from '../components/Confirm'
import { Input, Sheet, SkeletonList } from '../components/ui'
import BlockEditor, { type Editor } from '../components/BlockEditor'
import Byline from '../components/Byline'

/** 일기 편집. 저장 버튼 없이 800ms 뒤 자동 저장. 참조는 본문 위 칩(diary_refs). */
export default function DiaryEntry() {
  // /diary/new 는 처음 무언가 적을 때 서버에 만든다(빈 글이 쌓이지 않게).
  const param = useParams().id
  const [search] = useSearchParams()
  const nav = useNavigate()
  const qc = useQueryClient()
  const confirm = useConfirm()
  const [id, setId] = useState<number | null>(param === 'new' ? null : Number(param))
  const idRef = useRef(id)
  const createdHere = useRef(false)
  const newDate = search.get('date') && /^\d{4}-\d{2}-\d{2}$/.test(search.get('date')!) ? search.get('date')! : today()
  const q = useDiaryEntry(id ?? 0, id !== null)

  const [saving, setSaving] = useState<'idle' | 'saving' | 'saved' | 'error'>('idle')
  const [picking, setPicking] = useState(false)
  const [uploading, setUploading] = useState(0)
  const [uploadErr, setUploadErr] = useState<string | null>(null)
  const editorRef = useRef<Editor | null>(null)
  const onReady = useCallback((ed: Editor) => { editorRef.current = ed }, [])
  const titleRef = useRef('')
  const refsRef = useRef(0)
  const recheckEmpty = () => {
    const doc = editorRef.current?.document ?? []
    const hasBody = doc.some((b) => b.type === 'image' || (Array.isArray(b.content) && b.content.some((c) => 'text' in c && String(c.text).trim() !== '')))
    isEmptyRef.current = titleRef.current.trim() === '' && !hasBody && refsRef.current === 0
  }
  const fileInput = useRef<HTMLInputElement>(null)

  // 사진은 고르는 즉시 blob: 주소로 보이고, 줄이기·전송은 뒤에서 한 장씩 한다.
  // 끝나면 그 블록의 주소만 서버 것으로 바꾼다. blob: 은 저장하지 않는다(stripPending).
  // 화면을 떠나도 올리기는 끝까지 가고, 끝난 사진은 서버의 글 끝에 붙인다(finishDetached).
  const queue = useRef<Promise<void>>(Promise.resolve())
  function addPhotos(files: FileList | null) {
    const ed = editorRef.current
    if (!ed || !files || files.length === 0) return
    setUploadErr(null)
    for (const f of Array.from(files)) {
      const local = URL.createObjectURL(f)
      const last = ed.document[ed.document.length - 1]
      const [block] = ed.insertBlocks([{ type: 'image', props: { url: local } }], last, 'after')
      pendingPhotos.add(local)
      setUploading((n) => n + 1)
      queue.current = queue.current.then(async () => {
        try {
          const m = await uploadMedia(f)
          const live = editorRef.current
          if (live && live.getBlock(block.id)) {
            live.updateBlock(block.id, { props: { url: m.url } })
          } else {
            await finishDetached(m.url)
          }
        } catch (ex) {
          setUploadErr(ex instanceof Error ? ex.message : '사진을 올리지 못했어요')
          const live = editorRef.current
          if (live && live.getBlock(block.id)) live.removeBlocks([block.id])
        } finally {
          pendingPhotos.delete(local)
          URL.revokeObjectURL(local)
          setUploading((n) => n - 1)
        }
      })
    }
    recheckEmpty()
    if (fileInput.current) fileInput.current.value = ''
  }

  async function finishDetached(url: string) {
    const cur = idRef.current
    if (cur === null) return
    const fresh = await api.get<DiaryEntry>(`/api/diary/${cur}`)
    let blocks: unknown[] = []
    try { blocks = fresh.content ? JSON.parse(fresh.content) : [] } catch { /* 빈 글로 */ }
    blocks.push({ type: 'image', props: { url }, children: [] })
    await api.patch(`/api/diary/${cur}`, { content: JSON.stringify(blocks) })
    qc.invalidateQueries({ queryKey: ['diary'] })
  }

  const timer = useRef<number | undefined>(undefined)
  const pending = useRef<Record<string, unknown>>({})

  // 만드는 요청은 한 번만 — 첫 저장이 느린 사이 다음 저장이 오면 글이 둘 생긴다.
  const creating = useRef<Promise<number> | null>(null)
  const ensureID = useCallback(() => {
    if (idRef.current !== null) return Promise.resolve(idRef.current)
    if (!creating.current) {
      creating.current = api.post<DiaryEntry>('/api/diary', { date: newDate }).then((created) => {
        idRef.current = created.id
        createdHere.current = true
        // 서버 답을 캐시에 먼저 넣는다. 안 넣으면 id 가 생기는 순간 조회가 시작돼 편집기가 다시 마운트되고 쓰던 글이 날아간다.
        qc.setQueryData(['diary-entry', created.id], created)
        setId(created.id)
        nav(`/diary/${created.id}`, { replace: true })
        return created.id
      }).finally(() => { creating.current = null })
    }
    return creating.current
  }, [nav, newDate, qc])

  const flush = useCallback(async () => {
    if (Object.keys(pending.current).length === 0) return
    try {
      const cur = await ensureID()
      const body = pending.current
      pending.current = {}
      const saved = await api.patch<DiaryEntry>(`/api/diary/${cur}`, body)
      // 편집기는 initial 만 보고 제목은 uncontrolled 라 캐시를 갱신해도 되돌아가지 않는다.
      qc.setQueryData(['diary-entry', cur], saved)
      setSaving('saved')
      qc.invalidateQueries({ queryKey: ['diary'] })
      qc.invalidateQueries({ queryKey: ['calendar'] })
    } catch {
      setSaving('error')
    }
  }, [ensureID, qc])

  function queueSave(patch: Record<string, unknown>) {
    pending.current = { ...pending.current, ...patch }
    setSaving('saving')
    window.clearTimeout(timer.current)
    timer.current = window.setTimeout(flush, 800)
  }

  // 나갈 때: 남은 변경은 바로 보내고, 이 화면에서 만든 글이 비어 있으면 지운다.
  const isEmptyRef = useRef(true)
  // flush 를 의존성에 넣으면 렌더 사이(주소가 /diary/new → /diary/N 으로 바뀔 때)에도 정리가 돌아
  // 방금 만든 글을 지운다. ref + 빈 의존성으로 진짜 언마운트에만.
  const flushRef = useRef(flush)
  flushRef.current = flush
  useEffect(() => () => {
    window.clearTimeout(timer.current)
    const cur = idRef.current
    if (cur !== null && createdHere.current && isEmptyRef.current && Object.keys(pending.current).length === 0 && creating.current === null) {
      api.del(`/api/diary/${cur}`).then(() => {
        qc.invalidateQueries({ queryKey: ['diary'] })
        qc.invalidateQueries({ queryKey: ['calendar'] })
      }).catch(() => { /* 빈 글 하나 남는 게 최악이다 */ })
      return
    }
    flushRef.current()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  if (id !== null && q.isPending) {
    return (
      <div className="mx-auto max-w-2xl space-y-4 p-4">
        <div className="h-7 w-2/3 animate-pulse rounded bg-line" />
        <SkeletonList rows={3} />
      </div>
    )
  }
  if (id !== null && (q.isError || !q.data)) return <div className="p-6 text-rose-500">일기를 불러올 수 없어요</div>
  const e: DiaryEntry = q.data ?? {
    id: 0, date: newDate, title: '', content: null, plain: '', photos: [], source: null,
    created_by: null, created_at: 0, updated_at: 0, refs: [],
  }
  titleRef.current = titleRef.current || e.title
  refsRef.current = e.refs.length

  const [localRefs, setLocalRefs] = useState<DiaryRef[] | null>(null)
  const refs = localRefs ?? e.refs
  const setRefs = (next: DiaryRef[]) => {
    setLocalRefs(next)
    refsRef.current = next.length
    recheckEmpty()
    if (id !== null) qc.setQueryData(['diary-entry', id], { ...e, refs: next })
    queueSave({ refs: next })
  }
  const linkOf = (r: DiaryRef) => {
    if (r.gone) return null
    if (r.kind === 'card') return `/cards/${r.ref_id}`
    if (r.kind === 'routine') return `/routines?edit=${r.ref_id}`
    return `/babyfood?date=${r.ref_date}${r.child_id ? `&child=${r.child_id}` : ''}`
  }

  return (
    <div className="mx-auto flex h-full max-w-2xl flex-col">
      <header className="sticky top-0 z-10 flex items-center gap-2 bg-canvas/90 px-3 py-2 backdrop-blur">
        <button onClick={() => nav(-1)} aria-label="뒤로" className="rounded-lg p-1 text-muted active:bg-line">
          <svg viewBox="0 0 24 24" className="h-6 w-6" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><path d="M15 18l-6-6 6-6" /></svg>
        </button>
        <span className="min-w-0 flex-1 truncate text-xs text-faint">
          다이어리 <span className="mx-1">›</span> {fmtDate(e.date)}
        </span>
        <span className="shrink-0 text-[11px] text-faint">
          {saving === 'saving' ? '저장 중…' : saving === 'saved' ? '저장됨' : saving === 'error' ? '저장 실패' : ''}
        </span>
        <button
          onClick={async () => {
            if (id === null) { nav(-1); return }
            if (await confirm({ title: '이 일기를 지울까요?', body: '되돌릴 수 없어요.', confirmLabel: '지우기', danger: true })) {
              window.clearTimeout(timer.current)
              pending.current = {}
              createdHere.current = false
              await api.del(`/api/diary/${id}`)
              qc.invalidateQueries({ queryKey: ['diary'] })
              qc.invalidateQueries({ queryKey: ['calendar'] })
              nav('/diary', { replace: true })
            }
          }}
          aria-label="지우기"
          className="rounded-lg p-1 text-muted active:bg-line"
        >
          <svg viewBox="0 0 24 24" className="h-5 w-5" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><path d="M5 7h14M10 11v6M14 11v6M6 7l1 13h10l1-13M9 7V4h6v3" /></svg>
        </button>
      </header>

      <div className="px-4 pb-2">
        <input
          defaultValue={e.title}
          onChange={(ev) => { titleRef.current = ev.target.value; recheckEmpty(); queueSave({ title: ev.target.value }) }}
          placeholder="제목 (없어도 괜찮아요)"
          className="w-full bg-transparent text-2xl font-bold outline-none placeholder:text-ghost"
        />
        <div className="mt-1.5 flex items-center gap-2">
          <input
            type="date"
            defaultValue={e.date}
            onChange={(ev) => { if (ev.target.value) queueSave({ date: ev.target.value }) }}
            className="rounded-lg border border-line bg-surface px-2 py-1 text-xs"
          />
          <Byline by={e.created_by} at={e.created_at} />
        </div>

        <div className="mt-2 flex flex-wrap items-center gap-1.5">
          {refs.map((r, i) => {
            const to = linkOf(r)
            return (
              <span
                key={i}
                className={`flex items-center gap-1 rounded-lg py-1 pl-2 pr-1.5 text-xs ${
                  r.gone ? 'bg-surface-2 text-faint line-through' : 'bg-surface-2 text-ink-2'
                }`}
              >
                <button onClick={() => to && nav(to)} disabled={!to} className="max-w-[12rem] truncate">
                  {r.label}
                </button>
                <button
                  onClick={() => setRefs(refs.filter((_, k) => k !== i))}
                  aria-label="참조 빼기"
                  className="text-faint"
                >
                  <svg viewBox="0 0 24 24" className="h-3.5 w-3.5" fill="none" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round"><path d="M6 6l12 12M18 6L6 18" /></svg>
                </button>
              </span>
            )
          })}
          <button
            onClick={() => setPicking(true)}
            className="rounded-lg border border-dashed border-line px-2 py-1 text-xs text-muted"
          >
            + 참조
          </button>
          <button
            onClick={() => fileInput.current?.click()}
            className="rounded-lg border border-dashed border-line px-2 py-1 text-xs text-muted disabled:opacity-50"
          >
            {uploading > 0 ? `+ 사진 · 올리는 중 ${uploading}장` : '+ 사진'}
          </button>
          <input
            ref={fileInput}
            type="file"
            accept="image/*"
            multiple
            hidden
            onChange={(ev) => addPhotos(ev.target.files)}
          />
          {uploadErr && <span className="text-xs text-rose-500">{uploadErr}</span>}
        </div>
      </div>

      <div className="min-h-0 flex-1 px-4">
        <BlockEditor initial={e.content} onChange={(json) => { recheckEmpty(); queueSave({ content: stripPending(json) }) }} onReady={onReady} />
      </div>

      <RefPicker
        open={picking}
        date={e.date}
        onClose={() => setPicking(false)}
        onPick={(r) => { setRefs([...refs, r]); setPicking(false) }}
      />
    </div>
  )
}

/** 참조 고르기 — 검색으로 할 일·루틴을 찾고, 그 날의 이유식은 바로 건다. */
function RefPicker({ open, date, onClose, onPick }: {
  open: boolean
  date: string
  onClose: () => void
  onPick: (r: DiaryRef) => void
}) {
  const [q, setQ] = useState('')
  const found = useSearch(q)
  const blank: Omit<DiaryRef, 'kind' | 'label'> = { ref_id: null, ref_date: '', child_id: null, gone: false }

  return (
    <Sheet open={open} onClose={onClose} title="무엇을 걸까요">
      <div className="space-y-3">
        <button
          onClick={() => onPick({ ...blank, kind: 'babyfood', ref_date: date, label: `이유식 · ${fmtDate(date)}` })}
          className="w-full rounded-xl bg-canvas p-3 text-left text-sm"
        >
          이 날의 이유식 <span className="text-faint">· {fmtDate(date)}</span>
        </button>

        <Input autoFocus value={q} onChange={(ev) => setQ(ev.target.value)} placeholder="할 일 · 루틴 찾기" />

        {found.data && (
          <ul className="space-y-1">
            {found.data.cards.map((c) => (
              <li key={`c${c.id}`}>
                <button
                  onClick={() => onPick({ ...blank, kind: 'card', ref_id: c.id, label: c.title })}
                  className="w-full rounded-xl bg-canvas p-2.5 text-left"
                >
                  <span className="text-sm">{c.title}</span>
                  <span className="ml-1.5 text-[11px] text-faint">{c.board_name} · {c.column_name}</span>
                </button>
              </li>
            ))}
            {found.data.routines.map((r) => (
              <li key={`r${r.id}`}>
                <button
                  onClick={() => onPick({ ...blank, kind: 'routine', ref_id: r.id, label: r.title })}
                  className="w-full rounded-xl bg-canvas p-2.5 text-left"
                >
                  <span className="text-sm">{r.title}</span>
                  <span className="ml-1.5 text-[11px] text-faint">루틴</span>
                </button>
              </li>
            ))}
            {q.trim() !== '' && found.data.cards.length === 0 && found.data.routines.length === 0 && (
              <li className="py-4 text-center text-xs text-faint">찾는 게 없어요</li>
            )}
          </ul>
        )}
      </div>
    </Sheet>
  )
}

// 올리는 중인 사진의 임시 주소. 화면을 떠나도 올리기가 이어져 컴포넌트 밖에 둔다.
const pendingPhotos = new Set<string>()

/** 저장할 본문에서 아직 올라가지 않은(blob:) 사진 블록을 뺀다. */
function stripPending(json: string): string {
  if (!json.includes('blob:')) return json
  try {
    const walk = (bs: { type?: string; props?: { url?: string }; children?: unknown[] }[]): unknown[] =>
      bs.filter((b) => !(b.type === 'image' && String(b.props?.url ?? '').startsWith('blob:')))
        .map((b) => (Array.isArray(b.children) ? { ...b, children: walk(b.children as never) } : b))
    return JSON.stringify(walk(JSON.parse(json)))
  } catch { return json }
}
