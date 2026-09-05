import { useEffect, useRef, useState } from 'react'
import { useNavigate, useSearchParams } from 'react-router'
import { useQueryClient } from '@tanstack/react-query'
import { api, uploadMedia } from '../api'
import { useBFChildren, useDiaryEntry, useUsers } from '../lib/hooks'
import { fmtDate, lifeDayOf } from '../lib/date'
import Byline from './Byline'

/** 일기 읽기 창. 어느 글인지는 ?view=<id> 가 든다 — 뒤로 가기(스와이프)가 창을 닫는다. */
export function useDiaryViewer() {
  const [params, setParams] = useSearchParams()
  const nav = useNavigate()
  const id = Number(params.get('view')) || null
  const open = (entryID: number) => {
    const next = new URLSearchParams(params)
    next.set('view', String(entryID))
    setParams(next)
  }
  const close = () => {
    // 우리가 push 한 항목이면 뒤로(히스토리에 남지 않게), 링크로 들어왔으면 주소만 지운다.
    const idx = (window.history.state as { idx?: number } | null)?.idx ?? 0
    if (idx > 0) nav(-1)
    else {
      const next = new URLSearchParams(params)
      next.delete('view')
      setParams(next, { replace: true })
    }
  }
  return { id, open, close }
}

export default function DiaryViewer() {
  const { id, close } = useDiaryViewer()
  const nav = useNavigate()
  const q = useDiaryEntry(id ?? 0, id !== null)
  const users = useUsers()
  const kids = useBFChildren()
  const birth = kids.data?.length === 1 ? kids.data[0].birth_date : ''
  const [lightbox, setLightbox] = useState<number | null>(null)
  const qc = useQueryClient()

  useEffect(() => {
    if (id === null) return
    const onKey = (e: KeyboardEvent) => { if (e.key === 'Escape') (lightbox !== null ? setLightbox(null) : close()) }
    document.addEventListener('keydown', onKey)
    document.body.style.overflow = 'hidden'
    return () => { document.removeEventListener('keydown', onKey); document.body.style.overflow = '' }
  })

  if (id === null) return null
  const e = q.data
  const blocks = e?.content ? parseBlocks(e.content) : []
  const photos = collectPhotos(blocks)

  return (
    <div className="fixed inset-0 z-40 flex items-end justify-center sm:items-center" onClick={close}>
      <div className="absolute inset-0 bg-black/50" />
      <div
        role="dialog"
        onClick={(ev) => ev.stopPropagation()}
        className="relative flex max-h-[92dvh] w-full max-w-lg flex-col rounded-t-3xl bg-surface shadow-xl sm:max-h-[85vh] sm:rounded-2xl"
        style={{ paddingBottom: 'env(safe-area-inset-bottom)' }}
      >
        <div className="mx-auto mt-2 h-1 w-10 shrink-0 rounded-full bg-line sm:hidden" />
        <header className="flex items-center gap-2 px-4 pb-2 pt-3">
          <div className="min-w-0 flex-1">
            {e && (
              <p className="text-xs text-faint">
                {fmtDate(e.date)}
                {birth && birth <= e.date && <span className="ml-1.5 font-medium text-amber-700">{lifeDayOf(birth, e.date)}</span>}
              </p>
            )}
            {e?.title && <h2 className="truncate text-lg font-bold">{e.title}</h2>}
          </div>
          <button onClick={() => nav(`/diary/${id}`)} className="rounded-lg px-2 py-1 text-sm font-medium text-muted active:bg-line">수정</button>
          <button onClick={close} aria-label="닫기" className="rounded-lg p-1 text-muted active:bg-line">
            <svg viewBox="0 0 24 24" className="h-5 w-5" fill="none" stroke="currentColor" strokeWidth="2.2" strokeLinecap="round"><path d="M6 6l12 12M18 6L6 18" /></svg>
          </button>
        </header>

        <div className="min-h-0 flex-1 overflow-y-auto px-4 pb-4">
          {q.isPending && <div className="space-y-2 pt-2"><div className="h-4 w-3/4 animate-pulse rounded bg-line" /><div className="h-4 w-1/2 animate-pulse rounded bg-line" /></div>}
          {q.isError && <p className="py-6 text-center text-sm text-rose-500">일기를 불러올 수 없어요</p>}
          {e && (
            <>
              <Blocks blocks={blocks} onPhoto={(url) => setLightbox(Math.max(0, photos.indexOf(url)))} />
              {blocks.length === 0 && <p className="py-6 text-center text-sm text-faint">아직 아무것도 안 적었어요</p>}
              {e.refs.length > 0 && (
                <div className="mt-3 flex flex-wrap gap-1.5 border-t border-line pt-3">
                  {e.refs.map((r, i) => (
                    <span key={i} className={`rounded-lg bg-surface-2 px-2 py-1 text-xs ${r.gone ? 'text-faint line-through' : 'text-ink-2'}`}>{r.label}</span>
                  ))}
                </div>
              )}
              <Byline by={e.created_by} at={e.created_at} className="mt-3" />
              {users.data && e.source && <p className="mt-1 text-[11px] text-faint">다른 앱에서 가져온 글</p>}
            </>
          )}
        </div>
      </div>

      {lightbox !== null && photos.length > 0 && (
        <Lightbox
          photos={photos}
          start={lightbox}
          onClose={() => setLightbox(null)}
          onReplace={async (oldURL, newURL) => {
            if (!e?.content) return
            // 본문의 사진 주소를 바꿔 저장하고 옛 파일을 지운다(다른 글이 쓰면 서버가 409 로 남긴다).
            const content = e.content.split(oldURL).join(newURL)
            await api.patch(`/api/diary/${e.id}`, { content })
            qc.setQueryData(['diary-entry', e.id], { ...e, content })
            qc.invalidateQueries({ queryKey: ['diary'] })
            api.del(`/api/media/${oldURL.slice('/media/'.length)}`).catch(() => { /* 쓰는 곳이 남았으면 둔다 */ })
          }}
        />
      )}
    </div>
  )
}

// --- 본문 그리기 ---
// 편집기 없이 저장된 블록을 그린다. 모르는 블록은 글자만 뽑아 단락으로.

interface Inline { type: string; text?: string; href?: string; content?: Inline[]; styles?: Record<string, unknown> }
interface Block { type: string; props?: Record<string, unknown>; content?: Inline[] | { rows?: { cells: Inline[][] }[] }; children?: Block[] }

function parseBlocks(raw: string): Block[] {
  try {
    const v = JSON.parse(raw)
    return Array.isArray(v) ? v : []
  } catch { return [] }
}

function collectPhotos(blocks: Block[]): string[] {
  const out: string[] = []
  const walk = (bs: Block[]) => {
    for (const b of bs) {
      const url = b.type === 'image' ? String(b.props?.url ?? '') : ''
      if (url) out.push(url)
      if (b.children) walk(b.children)
    }
  }
  walk(blocks)
  return out
}

function InlineText({ items }: { items?: Inline[] }) {
  if (!items) return null
  return (
    <>
      {items.map((it, i) => {
        if (it.type === 'link') {
          return <a key={i} href={it.href} target="_blank" rel="noreferrer" className="text-sky-600 underline"><InlineText items={it.content} /></a>
        }
        const st = it.styles ?? {}
        let node: React.ReactNode = it.text ?? ''
        if (st.bold) node = <b>{node}</b>
        if (st.italic) node = <i>{node}</i>
        if (st.underline) node = <u>{node}</u>
        if (st.strike) node = <s>{node}</s>
        if (st.code) node = <code className="rounded bg-surface-2 px-1 text-[0.9em]">{node}</code>
        return <span key={i}>{node}</span>
      })}
    </>
  )
}

function Blocks({ blocks, onPhoto }: { blocks: Block[]; onPhoto: (url: string) => void }) {
  // 연달아 있는 사진은 격자로 묶는다.
  const groups: (Block | Block[])[] = []
  for (const b of blocks) {
    const last = groups[groups.length - 1]
    if (b.type === 'image') {
      if (Array.isArray(last)) last.push(b)
      else groups.push([b])
    } else groups.push(b)
  }
  let num = 0
  return (
    <div className="space-y-1.5 text-[15px] leading-relaxed">
      {groups.map((g, i) => {
        if (Array.isArray(g)) {
          const urls = g.map((b) => String(b.props?.url ?? '')).filter(Boolean)
          const cols = urls.length === 1 ? 'grid-cols-1' : urls.length === 2 || urls.length === 4 ? 'grid-cols-2' : 'grid-cols-3'
          return (
            <div key={i} className={`grid gap-1 pt-1 ${cols}`}>
              {urls.map((u, k) => (
                <button key={k} onClick={() => onPhoto(u)} className="overflow-hidden rounded-xl bg-surface-2 active:opacity-80">
                  <img src={u} alt="" loading="lazy" className={`w-full object-cover ${urls.length === 1 ? 'max-h-[70vh]' : 'aspect-square'}`} />
                </button>
              ))}
            </div>
          )
        }
        const b = g
        const inline = Array.isArray(b.content) ? b.content : undefined
        const kids = b.children && b.children.length > 0 ? <div className="pl-4"><Blocks blocks={b.children} onPhoto={onPhoto} /></div> : null
        if (b.type !== 'numberedListItem') num = 0
        switch (b.type) {
          case 'heading': {
            const lv = Number(b.props?.level ?? 2)
            const cls = lv === 1 ? 'text-xl font-bold pt-2' : lv === 2 ? 'text-lg font-bold pt-1.5' : 'text-base font-semibold pt-1'
            return <div key={i}><p className={cls}><InlineText items={inline} /></p>{kids}</div>
          }
          case 'bulletListItem':
            return <div key={i}><p className="flex gap-2"><span className="text-faint">•</span><span><InlineText items={inline} /></span></p>{kids}</div>
          case 'numberedListItem':
            num += 1
            return <div key={i}><p className="flex gap-2"><span className="text-faint">{num}.</span><span><InlineText items={inline} /></span></p>{kids}</div>
          case 'checkListItem':
            return <div key={i}><p className={`flex gap-2 ${b.props?.checked ? 'text-faint line-through' : ''}`}><span>{b.props?.checked ? '☑' : '☐'}</span><span><InlineText items={inline} /></span></p>{kids}</div>
          case 'quote':
            return <div key={i}><p className="border-l-2 border-ghost pl-3 text-ink-2"><InlineText items={inline} /></p>{kids}</div>
          case 'callout':
            return <div key={i}><p className="flex gap-2 rounded-lg bg-amber-50 px-3 py-2"><span>{String(b.props?.emoji ?? '💡')}</span><span><InlineText items={inline} /></span></p>{kids}</div>
          case 'divider':
            return <hr key={i} className="my-2 border-line" />
          case 'codeBlock':
            return <pre key={i} className="overflow-x-auto rounded-lg bg-surface-2 p-3 text-xs"><InlineText items={inline} /></pre>
          case 'table': {
            const rows = !Array.isArray(b.content) ? b.content?.rows ?? [] : []
            return (
              <div key={i} className="overflow-x-auto"><table className="text-sm"><tbody>
                {rows.map((r, ri) => <tr key={ri}>{r.cells.map((c, ci) => <td key={ci} className="border border-line px-2 py-1"><InlineText items={c} /></td>)}</tr>)}
              </tbody></table></div>
            )
          }
          default: {
            const empty = !inline || inline.length === 0
            return <div key={i}>{empty ? <p className="h-3" /> : <p><InlineText items={inline} /></p>}{kids}</div>
          }
        }
      })}
    </div>
  )
}

// --- 사진 크게 보기 ---
// 넘기기는 스크롤 스냅(기기 물리 그대로). 편집 모드의 회전·반전은 CSS 로 미리 보이고,
// 저장할 때만 폰 캔버스로 한 번 만들어 올린다(Pi 에서 돌리면 느리다).
interface Edit { rot: number; flip: boolean }

function Lightbox({ photos, start, onClose, onReplace }: {
  photos: string[]
  start: number
  onClose: () => void
  onReplace: (oldURL: string, newURL: string) => Promise<void>
}) {
  const strip = useRef<HTMLDivElement>(null)
  const [idx, setIdx] = useState(start)
  const [edit, setEdit] = useState<Edit | null>(null)
  const [saving, setSaving] = useState(false)
  const [err, setErr] = useState<string | null>(null)

  useEffect(() => {
    const el = strip.current
    if (!el) return
    el.scrollTo({ left: el.clientWidth * start })
  }, [start])

  const onScroll = () => {
    const el = strip.current
    if (!el || edit) return
    setIdx(Math.round(el.scrollLeft / el.clientWidth))
  }
  const go = (n: number) => {
    const el = strip.current
    if (!el) return
    const next = Math.min(photos.length - 1, Math.max(0, n))
    el.scrollTo({ left: el.clientWidth * next, behavior: 'smooth' })
  }

  const touchY = useRef<number | null>(null)
  const current = photos[Math.min(idx, photos.length - 1)]
  const changed = !!edit && (edit.rot % 360 !== 0 || edit.flip)

  const save = async () => {
    if (!edit || !current) return
    if (!changed) { setEdit(null); return }
    setSaving(true); setErr(null)
    try {
      const blob = await transformImage(current, edit.rot, edit.flip)
      const m = await uploadMedia(new File([blob], 'photo.jpg', { type: 'image/jpeg' }))
      await onReplace(current, m.url)
      setEdit(null)
    } catch (ex) {
      setErr(ex instanceof Error ? ex.message : '저장하지 못했어요')
    } finally {
      setSaving(false)
    }
  }

  return (
    <div
      className="fixed inset-0 z-50 bg-black"
      onClick={(ev) => ev.stopPropagation()}
      onTouchStart={(ev) => { touchY.current = ev.touches[0].clientY }}
      onTouchEnd={(ev) => {
        const y0 = touchY.current
        touchY.current = null
        // 아래로 쓸어내리면 닫힌다. 편집 중엔 막는다.
        if (!edit && y0 !== null && ev.changedTouches[0].clientY - y0 > 90) onClose()
      }}
    >
      <div
        ref={strip}
        onScroll={onScroll}
        className={`no-scrollbar flex h-full w-full snap-x snap-mandatory ${edit ? 'overflow-x-hidden' : 'overflow-x-auto'}`}
      >
        {photos.map((u, i) => {
          const e = i === idx ? edit : null
          const sideways = !!e && e.rot % 180 !== 0
          return (
            <div key={u} className="flex h-full w-full shrink-0 snap-center items-center justify-center overflow-hidden" onClick={() => !edit && onClose()}>
              <img
                src={u}
                alt=""
                onClick={(ev) => ev.stopPropagation()}
                className="object-contain transition-transform duration-200"
                style={{
                  // 옆으로 누우면 가로·세로 한계를 바꾼다.
                  maxWidth: sideways ? '100vh' : '100%',
                  maxHeight: sideways ? '100vw' : '100%',
                  transform: e ? `rotate(${e.rot}deg) scaleX(${e.flip ? -1 : 1})` : undefined,
                }}
              />
            </div>
          )
        })}
      </div>

      <div className="absolute inset-x-0 top-0 z-10 flex items-center justify-between px-3 text-white" style={{ paddingTop: 'calc(env(safe-area-inset-top) + 8px)' }}>
        <span className="rounded-full bg-black/50 px-2.5 py-1 text-xs">{idx + 1} / {photos.length}</span>
        {!edit && (
          <button
            onClick={onClose}
            onTouchEnd={(ev) => { ev.preventDefault(); onClose() }}
            aria-label="닫기"
            className="touch-manipulation rounded-full bg-black/60 p-3"
          >
            <svg viewBox="0 0 24 24" className="h-6 w-6" fill="none" stroke="currentColor" strokeWidth="2.4" strokeLinecap="round"><path d="M6 6l12 12M18 6L6 18" /></svg>
          </button>
        )}
      </div>

      <div className="absolute inset-x-0 bottom-0 z-10 px-4 text-white" style={{ paddingBottom: 'calc(env(safe-area-inset-bottom) + 12px)' }}>
        {err && <p className="mb-2 text-center text-xs text-rose-300">{err}</p>}
        {!edit ? (
          current?.startsWith('/media/') && (
            <div className="flex justify-center">
              <button onClick={() => { setErr(null); setEdit({ rot: 0, flip: false }) }} className="rounded-full bg-black/60 px-4 py-2 text-sm font-medium">
                편집
              </button>
            </div>
          )
        ) : (
          <div className="flex items-center justify-between gap-2">
            <button onClick={() => { setEdit(null); setErr(null) }} disabled={saving} className="rounded-full px-3 py-2 text-sm text-white/80">취소</button>
            <div className="flex gap-2">
              <button
                onClick={() => setEdit({ ...edit, rot: edit.rot + 90 })}
                disabled={saving}
                className="flex flex-col items-center gap-0.5 rounded-xl bg-white/10 px-3 py-2 text-[11px]"
              >
                <svg viewBox="0 0 24 24" className="h-5 w-5" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><path d="M20 11a8 8 0 1 0-2.3 5.7" /><path d="M20 4v7h-7" /></svg>
                회전
              </button>
              <button
                onClick={() => setEdit({ ...edit, flip: !edit.flip })}
                disabled={saving}
                className="flex flex-col items-center gap-0.5 rounded-xl bg-white/10 px-3 py-2 text-[11px]"
              >
                <svg viewBox="0 0 24 24" className="h-5 w-5" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><path d="M12 3v18" /><path d="M8 7L3 12l5 5V7z" /><path d="M16 7l5 5-5 5V7z" /></svg>
                좌우 반전
              </button>
            </div>
            <button onClick={save} disabled={saving} className="rounded-full bg-white px-4 py-2 text-sm font-semibold text-black disabled:opacity-60">
              {saving ? '저장 중…' : '저장'}
            </button>
          </div>
        )}
      </div>

      {!edit && photos.length > 1 && (
        <div className="pointer-events-none absolute inset-y-0 left-0 right-0 hidden items-center justify-between px-2 sm:flex">
          <button onClick={() => go(idx - 1)} className="pointer-events-auto rounded-full bg-black/50 p-2 text-white" aria-label="이전">‹</button>
          <button onClick={() => go(idx + 1)} className="pointer-events-auto rounded-full bg-black/50 p-2 text-white" aria-label="다음">›</button>
        </div>
      )}
    </div>
  )
}

/** 돌리고(시계 방향) 뒤집어 JPEG 로. 미리보기 CSS 와 순서가 같아야 한다: 뒤집고 → 돌린다. */
async function transformImage(url: string, rot: number, flip: boolean): Promise<Blob> {
  const img = await new Promise<HTMLImageElement>((resolve, reject) => {
    const i = new Image()
    i.onload = () => resolve(i)
    i.onerror = () => reject(new Error('사진을 열지 못했어요'))
    i.src = url
  })
  const r = ((rot % 360) + 360) % 360
  const w = img.naturalWidth, h = img.naturalHeight
  const c = document.createElement('canvas')
  c.width = r % 180 === 0 ? w : h
  c.height = r % 180 === 0 ? h : w
  const ctx = c.getContext('2d')
  if (!ctx) throw new Error('사진을 편집하지 못했어요')
  ctx.translate(c.width / 2, c.height / 2)
  ctx.rotate((r * Math.PI) / 180)
  ctx.scale(flip ? -1 : 1, 1)
  ctx.drawImage(img, -w / 2, -h / 2)
  const blob = await new Promise<Blob | null>((res) => c.toBlob(res, 'image/jpeg', 0.9))
  if (!blob) throw new Error('사진을 만들지 못했어요')
  return blob
}
