import { useEffect, useRef, useState } from 'react'
import { useNavigate } from 'react-router'
import { type DiaryEntry } from '../api'
import { useBFChildren, useDiaryFeed, useUsers } from '../lib/hooks'
import { fmtDate, lifeDayOf, WEEKDAYS, weekdayIndex } from '../lib/date'
import { Button, Empty, PageHeader, SkeletonList } from '../components/ui'
import { useDiaryViewer } from '../components/DiaryViewer'

/** 다이어리 목록(시간순). 날짜로 짚는 건 캘린더가 맡는다. */
export default function Diary() {
  const nav = useNavigate()
  const viewer = useDiaryViewer()
  // 달 제목을 헤더 바로 밑에 붙이려고 헤더 높이를 잰다(헤더도 sticky 라 감싸지 않고 찾아서).
  const [headerH, setHeaderH] = useState(0)
  useEffect(() => {
    const h = document.querySelector('main header') as HTMLElement | null
    if (!h) return
    const ro = new ResizeObserver(() => setHeaderH(h.offsetHeight))
    ro.observe(h)
    setHeaderH(h.offsetHeight)
    return () => ro.disconnect()
  }, [])
  const feed = useDiaryFeed()
  const entries = feed.data?.pages.flat() ?? []

  // 보초(sentinel)가 화면에 들어오면 다음 장을 받는다.
  const sentinel = useRef<HTMLDivElement>(null)
  const { hasNextPage, isFetchingNextPage, fetchNextPage } = feed
  useEffect(() => {
    const el = sentinel.current
    if (!el || !hasNextPage) return
    const io = new IntersectionObserver((es) => {
      if (es.some((e) => e.isIntersecting) && !isFetchingNextPage) fetchNextPage()
    }, { rootMargin: '600px 0px' }) // 한 화면쯤 미리 — 닿기 전에 받아둔다
    io.observe(el)
    return () => io.disconnect()
  }, [hasNextPage, isFetchingNextPage, fetchNextPage])
  const users = useUsers()
  // 아이가 하나일 때만 생후 일수를 적는다.
  const kids = useBFChildren()
  const birth = kids.data?.length === 1 ? kids.data[0].birth_date : ''

  // 서버에 먼저 만들지 않는다(편집 화면이 처음 적을 때 만든다).
  const write = () => nav('/diary/new')

  const byMonth = new Map<string, DiaryEntry[]>()
  for (const e of entries) {
    const k = e.date.slice(0, 7)
    byMonth.set(k, [...(byMonth.get(k) ?? []), e])
  }

  return (
    <div className="mx-auto max-w-lg">
      <PageHeader title="다이어리" right={<Button variant="ghost" onClick={write}>+ 오늘 쓰기</Button>} />

      {feed.isPending && <div className="px-4 pt-4"><SkeletonList rows={4} /></div>}

      {feed.data && entries.length === 0 && (
        <div className="px-4 pt-10">
          <Empty>
            아직 쓴 일기가 없어요.
            <br />
            오늘 있었던 일을 한 줄만 남겨도 돼요.
          </Empty>
          <Button className="mx-auto mt-4 block" onClick={write}>오늘 쓰기</Button>
        </div>
      )}

      {[...byMonth.entries()].map(([month, entries]) => (
        <section key={month} className="px-4">
          {/* sticky 는 부모(section) 안에서만 붙어 다음 달 section 이 올라오면 밀려난다. */}
          <h2
            className="sticky z-10 -mx-4 bg-canvas/90 px-4 pb-2 pt-4 text-sm font-bold text-ink-2 backdrop-blur"
            style={{ top: headerH }}
          >
            {Number(month.slice(0, 4))}년 {Number(month.slice(5))}월
          </h2>
          <ul className="space-y-2">
            {entries.map((e) => (
              <li key={e.id}>
                <button
                  onClick={() => viewer.open(e.id)}
                  className="w-full rounded-2xl bg-surface p-3.5 text-left shadow-sm active:bg-canvas"
                >
                  <div className="flex items-baseline gap-2">
                    <span className="shrink-0 text-sm font-bold">
                      {Number(e.date.slice(8))}일
                      <span className="ml-1 text-[11px] font-medium text-faint">
                        {WEEKDAYS[weekdayIndex(e.date)]}
                      </span>
                    </span>
                    {birth && birth <= e.date && (
                      <span className="shrink-0 text-[11px] font-medium text-amber-700">{lifeDayOf(birth, e.date)}</span>
                    )}
                    <span className="min-w-0 flex-1 truncate text-sm font-semibold">{e.title}</span>
                    {e.created_by && (
                      <span className="shrink-0 text-[11px] text-faint">
                        {users.data?.find((u) => u.id === e.created_by)?.name}
                      </span>
                    )}
                  </div>
                  {e.plain && <p className="mt-1 line-clamp-3 text-sm text-ink-2">{e.plain}</p>}
                  {e.photos.length > 0 && (
                    <div className={`mt-2 grid gap-1 ${e.photos.length === 1 ? 'grid-cols-1' : e.photos.length === 2 ? 'grid-cols-2' : 'grid-cols-3'}`}>
                      {e.photos.slice(0, 3).map((u) => (
                        <img key={u} src={u} alt="" loading="lazy" className="aspect-square w-full rounded-lg object-cover" />
                      ))}
                    </div>
                  )}
                  {e.refs.length > 0 && (
                    <div className="mt-1.5 flex flex-wrap gap-1">
                      {e.refs.map((r, i) => (
                        <span key={i} className="rounded-md bg-surface-2 px-1.5 py-0.5 text-[11px] text-ink-2">
                          {r.label}
                        </span>
                      ))}
                    </div>
                  )}
                </button>
              </li>
            ))}
          </ul>
        </section>
      ))}

      {/* 40장씩 받는다. */}
      {entries.length > 0 && (
        <div ref={sentinel} className="px-4 pb-4 pt-4 text-center">
          {feed.hasNextPage ? (
            feed.isError ? (
              <Button variant="ghost" onClick={() => feed.fetchNextPage()}>이어서 불러오기</Button>
            ) : (
              <SkeletonList rows={2} />
            )
          ) : (
            <p className="text-[11px] text-faint">{fmtDate(entries[entries.length - 1].date)}이 첫 일기예요</p>
          )}
        </div>
      )}
    </div>
  )
}
