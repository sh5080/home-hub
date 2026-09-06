import { useRef, useState } from 'react'
import type { BFFood } from '../api'
import { Input } from './ui'

// 베이스·토핑·간식 공용 재료 고르개(자유 입력은 '단호박'/'단호박 ' 처럼 재료가 갈라진다).
// 목록에 없는 건 '+ 이름'으로 만든다.
export default function FoodPicker({ value, foods, multi, prefer, src, onChange }: {
  value: string[]
  foods: BFFood[]
  /** 여러 개 담을 수 있는가 (토핑) */
  multi?: boolean
  /** 검색어가 없을 때 앞에 둘 종류 */
  prefer?: 'base' | 'cube' | 'dish'
  /** 이 끼니의 원래 값. 검색어가 없을 때 맨 앞에 둔다 */
  src: string[]
  onChange: (next: string[]) => void
}) {
  const [q, setQ] = useState('')
  const [open, setOpen] = useState(false)
  const box = useRef<HTMLInputElement>(null)
  const kw = q.trim()

  // 후보를 누르면 포커스가 잠깐 빠진다 — 바로 닫으면 클릭이 빈 곳에 떨어진다. 정말 떠났을 때만 닫는다.
  const closeSoon = () => window.setTimeout(() => {
    if (document.activeElement !== box.current) setOpen(false)
  }, 150)

  const add = (name: string) => {
    const v = name.trim()
    if (!v) return
    if (multi) {
      if (!value.includes(v)) onChange([...value, v])
      // 토핑은 담고 나서도 계속 고르게 포커스를 둔다.
      box.current?.focus()
    } else {
      onChange([v])
      setOpen(false)
    }
    setQ('')
  }

  // 검색어가 없으면 이 끼니의 원래 값, 그다음 자주 쓰는 것부터.
  const picks = foods
    .filter((f) => !value.includes(f.name) && (!kw || f.name.includes(kw)))
    .sort((a, b) => {
      if (kw) {
        return Number(!a.name.startsWith(kw)) - Number(!b.name.startsWith(kw)) || a.name.localeCompare(b.name)
      }
      const rank = (f: BFFood) => (src.includes(f.name) ? 0 : f.kind === prefer ? 1 : 2)
      return rank(a) - rank(b) || b.uses - a.uses
    })
    .slice(0, 24)
  const isNew = kw !== '' && !foods.some((f) => f.name === kw) && !value.includes(kw)

  return (
    <>
      {value.length > 0 && (
        <div className="mb-1.5 flex flex-wrap gap-1.5">
          {value.map((t) => (
            <button
              key={t}
              type="button"
              onClick={() => { if (!open) onChange(value.filter((x) => x !== t)) }}
              className="flex items-center gap-1 rounded-lg bg-surface-2 py-1 pl-2 pr-1.5 text-sm text-ink-2"
            >
              {t}
              <svg viewBox="0 0 24 24" className="h-3.5 w-3.5 text-faint" fill="none" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round"><path d="M6 6l12 12M18 6L6 18" /></svg>
            </button>
          ))}
        </div>
      )}
      {/* 후보가 떠 있는 동안 뒤를 덮는 막 — 없으면 창 밖 탭이 밑의 칩(선택된 재료)을 눌러 빠뜨린다. */}
      {open && (
        <div
          className="fixed inset-0 z-20"
          onPointerDown={(e) => { e.preventDefault(); setOpen(false); box.current?.blur() }}
          aria-hidden
        />
      )}
      <div className={`relative ${open ? 'z-30' : ''}`}>
        <Input
          ref={box}
          value={q}
          onFocus={() => setOpen(true)}
          onBlur={closeSoon}
          onChange={(e) => { setQ(e.target.value); setOpen(true) }}
          onKeyDown={(e) => {
            if (e.key === 'Escape') { setOpen(false); return }
            // 엔터는 맨 앞 후보를, 후보가 없으면 친 그대로 담는다.
            if (e.key === 'Enter') { e.preventDefault(); if (kw) add(picks[0]?.name ?? kw) }
          }}
          placeholder="재료 찾기"
        />

        {/* 후보는 칸 위로 뜬다(아래면 키보드에 가리고, 흐름에 넣으면 양식이 출렁인다). */}
        {open && (
          <div className="absolute inset-x-0 bottom-full z-20 mb-1.5 max-h-52 overflow-y-auto overscroll-contain rounded-xl border border-line bg-surface p-2 shadow-lg">
            <div className="flex flex-wrap gap-1.5">
              {picks.map((f) => (
                <button
                  key={f.name}
                  type="button"
                  onClick={() => add(f.name)}
                  className={`rounded-lg px-2 py-1 text-sm ${
                    f.reaction
                ? 'bg-rose-100 text-rose-700'
                : f.disliked
                  ? 'bg-sky-100 text-sky-700'
                  : f.liked
                    ? 'bg-amber-100 text-amber-800'
                    : 'bg-canvas text-muted'
                  }`}
                >
                  {f.name}
                </button>
              ))}
              {isNew && (
                <button
                  type="button"
                  onClick={() => add(kw)}
                  className="rounded-lg border border-dashed border-line px-2 py-1 text-sm text-muted"
                >
                  + {kw}
                </button>
              )}
              {picks.length === 0 && !isNew && (
                <p className="px-1 py-0.5 text-xs text-faint">{kw ? '찾는 재료가 없어요' : '고를 재료가 없어요'}</p>
              )}
            </div>
          </div>
        )}
      </div>
    </>
  )
}

