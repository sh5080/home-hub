import { useEffect, useState } from 'react'
import { api, type FinTag } from '../api'
import { useInvalidating } from '../lib/hooks'
import { useConfirm } from './Confirm'
import { Button, Input, Sheet } from './ui'

// Tailwind 가 찾을 수 있게 클래스는 통째로 적는다. 서버의 finTagColors 와 같은 목록.
export const TAG_STYLE: Record<string, { chip: string; dot: string }> = {
  emerald: { chip: 'bg-emerald-100 text-emerald-800', dot: 'bg-emerald-500' },
  amber: { chip: 'bg-amber-100 text-amber-800', dot: 'bg-amber-500' },
  sky: { chip: 'bg-sky-100 text-sky-800', dot: 'bg-sky-500' },
  indigo: { chip: 'bg-indigo-100 text-indigo-800', dot: 'bg-indigo-500' },
  violet: { chip: 'bg-violet-100 text-violet-800', dot: 'bg-violet-500' },
  pink: { chip: 'bg-pink-100 text-pink-800', dot: 'bg-pink-500' },
  rose: { chip: 'bg-rose-100 text-rose-800', dot: 'bg-rose-500' },
  cyan: { chip: 'bg-cyan-100 text-cyan-800', dot: 'bg-cyan-500' },
  orange: { chip: 'bg-orange-100 text-orange-800', dot: 'bg-orange-500' },
  lime: { chip: 'bg-lime-100 text-lime-800', dot: 'bg-lime-500' },
  fuchsia: { chip: 'bg-fuchsia-100 text-fuchsia-800', dot: 'bg-fuchsia-500' },
  slate: { chip: 'bg-slate-200 text-slate-700', dot: 'bg-slate-500' },
  teal: { chip: 'bg-teal-100 text-teal-800', dot: 'bg-teal-500' },
  yellow: { chip: 'bg-yellow-100 text-yellow-800', dot: 'bg-yellow-500' },
}
const COLORS = Object.keys(TAG_STYLE)
const style = (c: string) => TAG_STYLE[c] ?? TAG_STYLE.slate

export function TagChip({ tag, className = '' }: { tag: FinTag; className?: string }) {
  return <span className={`inline-block rounded px-1.5 py-px text-[10px] font-medium ${style(tag.color).chip} ${className}`}>{tag.name}</span>
}

/** 항목의 태그들. 없으면 '+ 태그' 로 누를 곳을 보인다. */
export function TagChips({ ids, tags, onEdit }: { ids: number[] | undefined; tags: FinTag[]; onEdit: () => void }) {
  const list = (ids ?? []).map((id) => tags.find((t) => t.id === id)).filter((t): t is FinTag => !!t)
  return (
    <button type="button" onClick={onEdit} className="mt-0.5 flex flex-wrap items-center gap-1 text-left">
      {list.map((t) => <TagChip key={t.id} tag={t} />)}
      <span className="rounded border border-dashed border-line px-1.5 py-px text-[10px] text-faint">{list.length ? '태그' : '+ 태그'}</span>
    </button>
  )
}

/** 항목(key) 하나의 태그를 고르고, 태그를 만들거나 지운다. */
/**
 * 태그를 붙일 대상. refs 면 거래 한 건 한 건(평소·예상 외), keys 면 항목(고정 — 매달 같은 것).
 * cur 는 지금 붙은 태그, suggest 는 같은 이름의 다른 거래에 썼던 태그(누를 때만 붙는다).
 */
export type TagTarget = { label: string; keys?: string[]; refs?: string[]; cur: number[]; suggest?: number[] }

export function TagPicker({ target, tags, onClose }: {
  target: TagTarget | null
  tags: FinTag[]
  onClose: () => void
}) {
  const confirm = useConfirm()
  const keys = [['finance']]
  // 묶인 항목(같은 이름으로 고친 것)은 key 마다 같은 태그를 단다.
  const setLinks = useInvalidating((b: { keys: string[]; tag_ids: number[] }) =>
    Promise.all(b.keys.map((key) => api.put('/api/finance/tag-links', { key, tag_ids: b.tag_ids }))), keys)
  const setTx = useInvalidating((b: { refs: string[]; tag_ids: number[] }) => api.put('/api/finance/tx-tags', b), keys)
  const create = useInvalidating((b: { name: string; color: string }) => api.post<FinTag>('/api/finance/tags', b), keys)
  const del = useInvalidating((id: number) => api.del(`/api/finance/tags/${id}`), keys)
  const update = useInvalidating(({ id, ...b }: { id: number; name: string; color: string }) => api.patch(`/api/finance/tags/${id}`, b), keys)
  const [edit, setEdit] = useState<FinTag | null>(null) // 관리 모드에서 고르는 중인 태그
  const [name, setName] = useState('')
  const [color, setColor] = useState('emerald')
  const [editing, setEditing] = useState(false)
  const [err, setErr] = useState<string | null>(null)
  // 누르면 바로 저장하고, 화면은 서버 응답을 기다리지 않고 따라간다(미분류 보기의 다른 달 데이터일 수도 있다).
  const [cur, setCur] = useState<number[]>([])
  useEffect(() => { setCur(target?.cur ?? []) }, [target])
  const apply = (ids: number[]) => {
    if (!target) return
    setCur(ids)
    if (target.refs) setTx.mutate({ refs: target.refs, tag_ids: ids })
    else if (target.keys) setLinks.mutate({ keys: target.keys, tag_ids: ids })
  }
  const toggle = (id: number) => apply(cur.includes(id) ? cur.filter((x) => x !== id) : [...cur, id])
  const suggest = (target?.suggest ?? []).filter((id) => !cur.includes(id)).map((id) => tags.find((t) => t.id === id)).filter((t): t is FinTag => !!t)
  const add = async () => {
    if (!target || !name.trim()) return
    setErr(null)
    try {
      const t = await create.mutateAsync({ name: name.trim(), color })
      apply([...cur, t.id])
      setName('')
    } catch (e) { setErr(e instanceof Error ? e.message : '만들지 못했어요') }
  }
  const remove = async (t: FinTag) => {
    if (await confirm({ title: `'${t.name}' 태그를 지울까요?`, body: '붙어 있던 항목들에서도 빠져요.', confirmLabel: '삭제', danger: true })) {
      del.mutate(t.id)
      setEdit(null)
    }
  }
  const saveEdit = async () => {
    if (!edit || !edit.name.trim()) return
    setErr(null)
    try { await update.mutateAsync({ id: edit.id, name: edit.name.trim(), color: edit.color }); setEdit(null) } catch (e) { setErr(e instanceof Error ? e.message : '고치지 못했어요') }
  }

  return (
    <Sheet open={target !== null} onClose={() => { setEditing(false); setEdit(null); setErr(null); onClose() }} title={target?.label}>
      <div className="space-y-4">
        <div>
          <div className="mb-1.5 flex items-center justify-between">
            <p className="text-xs text-muted">{editing ? '고칠 태그를 누르세요' : target?.refs ? `이 거래${target.refs.length > 1 ? ` ${target.refs.length}건` : ''}에만 붙어요` : '이 항목(매달 같은 것)에 붙어요'}</p>
            <button onClick={() => { setEditing(!editing); setEdit(null); setErr(null) }} className="text-xs text-muted underline">{editing ? '완료' : '태그 관리'}</button>
          </div>
          {!editing && suggest.length > 0 && (
            <div className="mb-2 flex flex-wrap items-center gap-1.5">
              <span className="text-[11px] text-faint">이전에 쓴 태그</span>
              {suggest.map((t) => (
                <button key={t.id} type="button" onClick={() => toggle(t.id)} className="rounded-lg border border-dashed border-line px-2 py-1 text-xs text-muted hover:bg-surface-2">+ {t.name}</button>
              ))}
            </div>
          )}
          <div className="flex flex-wrap gap-1.5">
            {tags.map((t) => {
              const on = cur.includes(t.id)
              return (
                <button key={t.id} type="button" onClick={() => (editing ? setEdit({ ...t }) : toggle(t.id))}
                  className={`flex items-center gap-1 rounded-lg px-2.5 py-1.5 text-sm font-medium ${editing ? (edit?.id === t.id ? style(t.color).chip + ' ring-2 ring-ink/40' : 'bg-surface-2 text-ink-2') : on ? style(t.color).chip + ' ring-2 ring-current/30' : 'bg-surface-2 text-muted'}`}>
                  <i className={`inline-block h-2 w-2 rounded-full ${style(t.color).dot}`} />
                  {t.name}
                  {editing && <span className="ml-0.5 text-faint">✎</span>}
                </button>
              )
            })}
          </div>
        </div>
        {editing && edit && (
          <div className="space-y-2 rounded-xl bg-surface-2 p-3">
            <p className="text-xs font-semibold text-ink-2">'{tags.find((t) => t.id === edit.id)?.name}' 고치기</p>
            <Input value={edit.name} maxLength={12} onChange={(e) => setEdit({ ...edit, name: e.target.value })}
              onKeyDown={(e) => { if (e.key === 'Enter' && !e.nativeEvent.isComposing) saveEdit() }} />
            <div className="flex flex-wrap gap-1.5">
              {COLORS.map((c) => (
                <button key={c} type="button" aria-label={c} onClick={() => setEdit({ ...edit, color: c })}
                  className={`h-6 w-6 rounded-full ${style(c).dot} ${edit.color === c ? 'ring-2 ring-ink ring-offset-2 ring-offset-surface-2' : ''}`} />
              ))}
            </div>
            <div className="flex gap-2">
              <Button variant="danger" onClick={() => { const t = tags.find((x) => x.id === edit.id); if (t) remove(t) }}>삭제</Button>
              <div className="flex-1" />
              <Button variant="ghost" onClick={() => setEdit(null)}>취소</Button>
              <Button onClick={saveEdit} disabled={!edit.name.trim() || update.isPending}>저장</Button>
            </div>
          </div>
        )}
        {!editing && (
        <div className="space-y-2 border-t border-line pt-3">
          <p className="text-xs font-semibold text-ink-2">새 태그</p>
          <div className="flex gap-2">
            <Input value={name} maxLength={12} placeholder="예: 반려동물" onChange={(e) => setName(e.target.value)}
              onKeyDown={(e) => { if (e.key === 'Enter' && !e.nativeEvent.isComposing) add() }} />
            <Button onClick={add} disabled={!name.trim() || create.isPending}>추가</Button>
          </div>
          <div className="flex flex-wrap gap-1.5">
            {COLORS.map((c) => (
              <button key={c} type="button" aria-label={c} onClick={() => setColor(c)}
                className={`h-6 w-6 rounded-full ${style(c).dot} ${color === c ? 'ring-2 ring-ink ring-offset-2 ring-offset-surface' : ''}`} />
            ))}
          </div>
        </div>
        )}
        {err && <p className="text-xs text-rose-500">{err}</p>}
      </div>
    </Sheet>
  )
}
