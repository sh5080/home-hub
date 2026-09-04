import { useMemo, useState } from 'react'
import { useNavigate, useSearchParams } from 'react-router'
import { type BFFood } from '../api'
import { useBFFoods } from '../lib/hooks'
import { fmtDate } from '../lib/date'
import { Button, Input, PageHeader, SkeletonList } from '../components/ui'

// 먹어본 음식 요약. 여기서는 고치지 않는다(기록은 날짜에 달린다) — 날짜를 누르면 그날로 간다.
export default function BabyfoodFoods() {
  const nav = useNavigate()
  // ?q=<이름> 이면 그 재료만.
  const [params] = useSearchParams()
  const [q, setQ] = useState(() => params.get('q') ?? '')
  const childID = params.get('child') ? Number(params.get('child')) : undefined
  const suffix = childID ? `?child=${childID}` : ''
  const foods = useBFFoods(childID)

  const { reacted, liked, disliked, rest } = useMemo(() => {
    const kw = q.trim()
    const all = (foods.data ?? []).filter((f) => !kw || f.name.includes(kw))
    return {
      reacted: all.filter((f) => f.reaction),
      liked: all.filter((f) => f.liked && !f.reaction),
      disliked: all.filter((f) => f.disliked && !f.reaction && !f.liked),
      rest: all.filter((f) => !f.reaction && !f.liked && !f.disliked),
    }
  }, [foods.data, q])

  const goDay = (date: string) => nav(`/babyfood?date=${date}${childID ? `&child=${childID}` : ''}`)

  const dates = (list: string[], tone: 'rose' | 'amber' | 'sky') => (
    <span className="flex flex-wrap gap-1">
      {list.map((d) => (
        <button
          key={d}
          onClick={() => goDay(d)}
          className={`rounded-md px-1.5 py-0.5 text-[11px] font-medium ${
            tone === 'rose' ? 'bg-rose-100 text-rose-700'
              : tone === 'sky' ? 'bg-sky-100 text-sky-700'
                : 'bg-amber-100 text-amber-800'
          }`}
        >
          {d.slice(5).replace('-', '/')}
        </button>
      ))}
    </span>
  )

  const row = (f: BFFood) => (
    <li key={f.name} className="rounded-2xl bg-surface p-3 shadow-sm">
      <div className="flex items-baseline gap-2">
        <p className="min-w-0 flex-1 truncate text-sm font-medium">{f.name}</p>
        <p className="shrink-0 text-[11px] text-faint">
          {f.first_date ? `${fmtDate(f.first_date)}부터` : '식단에 없음'}
          {f.uses > 0 && ` · ${f.uses}번`}
        </p>
      </div>
      {f.reaction_dates.length > 0 && (
        <div className="mt-1.5 flex items-center gap-1.5">
          <span className="shrink-0 text-[11px] text-muted">반응</span>
          {dates(f.reaction_dates, 'rose')}
        </div>
      )}
      {f.liked_dates.length > 0 && (
        <div className="mt-1 flex items-center gap-1.5">
          <span className="shrink-0 text-[11px] text-muted">좋아함</span>
          {dates(f.liked_dates, 'amber')}
        </div>
      )}
      {f.disliked_dates.length > 0 && (
        <div className="mt-1 flex items-center gap-1.5">
          <span className="shrink-0 text-[11px] text-muted">싫어함</span>
          {dates(f.disliked_dates, 'sky')}
        </div>
      )}
    </li>
  )

  return (
    <div className="mx-auto max-w-lg">
      <PageHeader
        title="먹어본 음식"
        back={() => nav(`/babyfood${suffix}`)}
        right={<Button variant="ghost" onClick={() => nav(`/babyfood/stock${suffix}`)}>재고</Button>}
      />

      <div className="px-4 pt-2">
        <Input value={q} onChange={(e) => setQ(e.target.value)} placeholder="재료 찾기" />
      </div>

      {foods.isPending && <div className="px-4 pt-4"><SkeletonList rows={6} /></div>}

      {foods.data && (
        <>
          <Section title="알레르기 반응이 있었어요" tone="rose" count={reacted.length} empty="아직 없어요">
            {reacted.map(row)}
          </Section>
          <Section title="잘 먹어요" tone="amber" count={liked.length} empty="표시한 게 없어요">
            {liked.map(row)}
          </Section>
          <Section title="잘 안 먹어요" tone="sky" count={disliked.length} empty="표시한 게 없어요">
            {disliked.map(row)}
          </Section>
          <Section title="그 밖에" count={rest.length}>
            {rest.map(row)}
          </Section>
        </>
      )}
    </div>
  )
}

function Section({ title, tone, count, empty, children }: {
  title: string
  tone?: 'rose' | 'amber' | 'sky'
  count: number
  empty?: string
  children: React.ReactNode
}) {
  const dot = tone === 'rose' ? 'bg-rose-500' : tone === 'amber' ? 'bg-amber-400' : tone === 'sky' ? 'bg-sky-500' : 'bg-line'
  return (
    <section className="px-4 pt-5">
      <h2 className="flex items-center gap-1.5 text-sm font-bold text-ink-2">
        <i className={`h-2 w-2 rounded-full ${dot}`} />
        {title}
        <span className="font-normal text-faint">{count}</span>
      </h2>
      {count === 0 ? (
        empty && <p className="mt-1 text-xs text-faint">{empty}</p>
      ) : (
        <ul className="mt-2 space-y-1.5">{children}</ul>
      )}
    </section>
  )
}
