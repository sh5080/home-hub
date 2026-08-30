import { useEffect, useState } from 'react'
import { useNavigate, useSearchParams } from 'react-router'
import { useSearch } from '../lib/hooks'
import { fmtDue } from '../lib/date'
import { Empty, Input, PageHeader, SkeletonList } from '../components/ui'
import { Stars } from '../components/SortToggle'

// 카드 검색.
//
// 주소에 ?q= 를 남긴다 — 결과에서 카드로 들어갔다가 뒤로 오면 검색어가
// 그대로 있어야 한다. 매 글자마다 주소를 갈아끼우면 뒤로가기가 한 글자씩
// 되짚으므로 replace 로 바꾼다.
export default function Search() {
  const nav = useNavigate()
  const [params, setParams] = useSearchParams()
  const [q, setQ] = useState(() => params.get('q') ?? '')
  const [debounced, setDebounced] = useState(q)

  useEffect(() => {
    const t = setTimeout(() => {
      setDebounced(q)
      setParams(q ? { q } : {}, { replace: true })
    }, 200)
    return () => clearTimeout(t)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [q])

  const res = useSearch(debounced)
  const hits = res.data ?? []

  return (
    <div className="mx-auto max-w-lg">
      <PageHeader title="검색" back={() => nav(-1)} />

      <div className="px-4 pt-3">
        <Input
          autoFocus
          value={q}
          onChange={(e) => setQ(e.target.value)}
          placeholder="제목과 본문에서 찾아요"
          autoComplete="off"
          // iOS 키보드의 '완료'가 폼을 제출하지 않도록
          type="search"
        />
      </div>

      <div className="px-4 py-3">
        {debounced.trim() === '' && (
          <p className="py-10 text-center text-sm text-slate-400">찾을 말을 입력해주세요</p>
        )}
        {debounced.trim() !== '' && res.isPending && <SkeletonList rows={4} />}
        {debounced.trim() !== '' && !res.isPending && hits.length === 0 && (
          <Empty>“{debounced}” 에 맞는 카드가 없어요</Empty>
        )}
        <ul className="space-y-1.5">
          {hits.map((h) => (
            <li key={h.id}>
              <button
                onClick={() => nav(`/cards/${h.id}`)}
                className="flex w-full items-start gap-3 rounded-xl bg-white p-3 text-left shadow-sm active:bg-slate-50"
              >
                <div className="min-w-0 flex-1">
                  <p className="truncate text-sm font-medium">{h.title}</p>
                  {h.snippet && <p className="mt-0.5 truncate text-xs text-slate-500">{h.snippet}</p>}
                  <p className="mt-1 flex items-center gap-1.5 text-[11px] text-slate-400">
                    <span className="truncate">{h.board_name} · {h.column_name}</span>
                    {h.due_at && <span className="shrink-0">· {fmtDue(h.due_at)}</span>}
                  </p>
                </div>
                {h.priority > 0 && <Stars n={h.priority} />}
              </button>
            </li>
          ))}
        </ul>
        {hits.length >= 50 && (
          <p className="pt-3 text-center text-[11px] text-slate-400">
            결과가 많아요 — 앞의 50개만 보여줍니다
          </p>
        )}
      </div>
    </div>
  )
}
