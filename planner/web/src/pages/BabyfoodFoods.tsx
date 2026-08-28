import { useMemo, useState } from 'react'
import { useNavigate } from 'react-router'
import { api, type BFFood } from '../api'
import { useBFFoods, useInvalidating } from '../lib/hooks'
import { fmtDate } from '../lib/date'
import { Button, Input, PageHeader, SkeletonList } from '../components/ui'

// 먹어본 음식을 한눈에 본다.
//
// 반응과 좋아함은 **서로 독립**이다. 반응이 있으면서 잘 먹을 수도 있고(그래도
// 빼야 한다), 반응은 없는데 안 먹을 수도 있다. 그래서 한 줄에 두 개를 따로 둔다.
export default function BabyfoodFoods() {
  const nav = useNavigate()
  const [q, setQ] = useState('')
  const foods = useBFFoods()

  const tag = useInvalidating(
    (body: { name: string; reaction?: boolean; liked?: boolean }) =>
      api.post<BFFood>('/api/babyfood/foods/tag', body),
    [['babyfood'], ['babyfood-foods']],
  )

  const { reacted, liked, rest } = useMemo(() => {
    const kw = q.trim()
    const all = (foods.data ?? []).filter((f) => !kw || f.name.includes(kw))
    return {
      reacted: all.filter((f) => f.reaction),
      liked: all.filter((f) => f.liked && !f.reaction),
      rest: all.filter((f) => !f.reaction && !f.liked),
    }
  }, [foods.data, q])

  const row = (f: BFFood) => (
    <li key={f.name} className="flex items-center gap-2 rounded-xl bg-white p-2.5 pl-3 shadow-sm">
      <div className="min-w-0 flex-1">
        <p className="truncate text-sm font-medium">{f.name}</p>
        <p className="text-[11px] text-slate-400">
          {f.first_date ? `${fmtDate(f.first_date)}부터` : '식단에 없음'}
          {f.uses > 0 && ` · ${f.uses}번`}
        </p>
      </div>
      <button
        onClick={() => tag.mutate({ name: f.name, reaction: !f.reaction })}
        aria-pressed={f.reaction}
        className={`h-8 rounded-lg px-2.5 text-xs font-semibold ${
          f.reaction ? 'bg-rose-500 text-white' : 'bg-slate-100 text-slate-400'
        }`}
      >
        반응
      </button>
      <button
        onClick={() => tag.mutate({ name: f.name, liked: !f.liked })}
        aria-pressed={f.liked}
        className={`h-8 rounded-lg px-2.5 text-xs font-semibold ${
          f.liked ? 'bg-amber-400 text-white' : 'bg-slate-100 text-slate-400'
        }`}
      >
        좋아함
      </button>
    </li>
  )

  return (
    <div className="mx-auto max-w-lg">
      <PageHeader
        title="먹어본 음식"
        back={() => nav('/babyfood')}
        right={<Button variant="ghost" onClick={() => nav('/babyfood/stock')}>재고</Button>}
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
  tone?: 'rose' | 'amber'
  count: number
  empty?: string
  children: React.ReactNode
}) {
  const dot = tone === 'rose' ? 'bg-rose-500' : tone === 'amber' ? 'bg-amber-400' : 'bg-slate-300'
  return (
    <section className="px-4 pt-5">
      <h2 className="flex items-center gap-1.5 text-sm font-bold text-slate-700">
        <i className={`h-2 w-2 rounded-full ${dot}`} />
        {title}
        <span className="font-normal text-slate-400">{count}</span>
      </h2>
      {count === 0 ? (
        empty && <p className="mt-1 text-xs text-slate-400">{empty}</p>
      ) : (
        <ul className="mt-2 space-y-1.5">{children}</ul>
      )}
    </section>
  )
}
