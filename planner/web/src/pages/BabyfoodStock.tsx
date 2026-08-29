import { useState } from 'react'
import { useNavigate } from 'react-router'
import { api, type BFStock, type Card } from '../api'
import { useBFStock, useInvalidating } from '../lib/hooks'
import { fmtDate } from '../lib/date'
import { Button, Field, Input, PageHeader, Sheet, SkeletonList } from '../components/ui'

// 재고 화면.
//
// 재고를 숫자 하나로 직접 고치게 두면 종이로 적던 때와 똑같이 틀어진다.
// 그래서 여기서는 세 가지만 한다: **실사**(지금 센 개수로 맞추기),
// **제조 기록**(+), 그리고 그 둘로 계산된 값 보여주기. 어떻게 그 숫자가
// 나왔는지를 줄마다 같이 보여줘서 조용히 어긋나지 않게 한다.
export default function BabyfoodStock() {
  const nav = useNavigate()
  const [onlyNeeded, setOnlyNeeded] = useState(true)
  const [target, setTarget] = useState<BFStock | null>(null)
  const q = useBFStock()

  const count = useInvalidating(
    (body: { name: string; qty: number }) => api.post('/api/babyfood/stock/count', body),
    [['babyfood-stock']],
  )
  const batch = useInvalidating(
    (body: { name: string; qty: number; note: string }) => api.post('/api/babyfood/stock/batch', body),
    [['babyfood-stock']],
  )
  // 재고 화면에서 끝나지 않고 칸반으로 넘긴다. 본문(체크리스트)은 서버가 만든다.
  const makeCard = useInvalidating(
    () => api.post<Card>('/api/babyfood/stock/shopping', {}),
    [['boards'], ['board'], ['today'], ['calendar']],
  )
  const [cardErr, setCardErr] = useState<string | null>(null)

  const items = (q.data?.items ?? []).filter((i) => i.need > 0 || i.stock != null)
  const shown = onlyNeeded ? items.filter((i) => i.make > 0) : items
  const groups: { key: BFStock['kind']; label: string; hint?: string }[] = [
    { key: 'cube', label: '토핑 큐브' },
    { key: 'base', label: '베이스' },
    { key: 'dish', label: '그날 만드는 요리', hint: '큐브가 아니라 그때 조리하는 메뉴예요' },
  ]

  return (
    <div className="mx-auto max-w-lg">
      <PageHeader
        title="재고"
        back={() => nav('/babyfood')}
        right={<Button variant="ghost" onClick={() => nav('/babyfood/foods')}>음식</Button>}
      />

      <div className="px-4 pt-2">
        {q.isError && (
          <div className="rounded-xl bg-white p-4 text-center shadow-sm">
            <p className="text-sm text-slate-500">{(q.error as Error).message}</p>
            <Button className="mt-3" onClick={() => nav('/?settings=1')}>설정 열기</Button>
          </div>
        )}
        {q.data && (
          <>
            <p className="text-xs text-slate-400">
              {fmtDate(q.data.from)} ~ {fmtDate(q.data.to)} · {q.data.horizon_days}일치
            </p>
            <label className="mt-2 flex items-center gap-2 text-sm">
              <input type="checkbox" className="h-4 w-4" checked={onlyNeeded} onChange={(e) => setOnlyNeeded(e.target.checked)} />
              만들어야 할 것만 보기
            </label>
          </>
        )}
      </div>

      {q.isPending && <div className="px-4 pt-4"><SkeletonList rows={6} /></div>}

      {q.data &&
        groups.map((g) => {
          const rows = shown.filter((i) => i.kind === g.key)
          if (rows.length === 0) return null
          return (
            <section key={g.key} className="px-4 pt-4">
              <h2 className="text-sm font-bold text-slate-700">{g.label}</h2>
              {g.hint && <p className="text-[11px] text-slate-400">{g.hint}</p>}
              <ul className="mt-2 space-y-1.5">
                {rows.map((it) => (
                  <li key={it.name}>
                    <button
                      onClick={() => setTarget(it)}
                      className="flex w-full items-center gap-3 rounded-xl bg-white p-3 text-left shadow-sm active:bg-slate-50"
                    >
                      <div className="min-w-0 flex-1">
                        <p className="truncate text-sm font-medium">{it.name}</p>
                        <p className="text-[11px] text-slate-400">
                          필요 {it.need} · 재고 {it.stock == null ? '아직 안 셈' : it.stock}
                          {it.stock != null && (it.used > 0 || it.made > 0) && ` (실사 ${it.count_qty} − 사용 ${it.used} + 제조 ${it.made})`}
                        </p>
                      </div>
                      {it.make > 0 ? (
                        <span className="shrink-0 rounded-lg bg-rose-50 px-2 py-1 text-sm font-bold text-rose-600">
                          +{it.make}
                        </span>
                      ) : (
                        <span className="shrink-0 text-xs text-emerald-600">충분</span>
                      )}
                    </button>
                  </li>
                ))}
              </ul>
            </section>
          )
        })}

      {q.data && shown.length === 0 && (
        <p className="px-4 py-10 text-center text-sm text-slate-400">
          {onlyNeeded ? '만들어야 할 게 없어요' : '재료가 없어요'}
        </p>
      )}

      {q.data && shown.some((i) => i.make > 0) && (
        <div className="px-4 pt-6">
          <Button
            className="w-full"
            disabled={makeCard.isPending}
            onClick={async () => {
              setCardErr(null)
              try {
                const card = await makeCard.mutateAsync(undefined)
                nav(`/cards/${card.id}`)
              } catch (e) {
                setCardErr(e instanceof Error ? e.message : '만들지 못했어요')
              }
            }}
          >
            {makeCard.isPending ? '만드는 중…' : '장보기 카드 만들기'}
          </Button>
          <p className="mt-1.5 text-center text-[11px] text-slate-400">
            만들어야 할 것들이 체크리스트로 들어간 할 일 카드가 생겨요
          </p>
          {cardErr && <p className="mt-1 text-center text-xs text-rose-500">{cardErr}</p>}
        </div>
      )}

      <Sheet open={!!target} onClose={() => setTarget(null)} title={target?.name}>
        {target && (
          <StockForm
            item={target}
            onCount={(qty) => { count.mutate({ name: target.name, qty }); setTarget(null) }}
            onBatch={(qty, note) => { batch.mutate({ name: target.name, qty, note }); setTarget(null) }}
            onClose={() => setTarget(null)}
          />
        )}
      </Sheet>
    </div>
  )
}

function StockForm({ item, onCount, onBatch, onClose }: {
  item: BFStock
  onCount: (qty: number) => void
  onBatch: (qty: number, note: string) => void
  onClose: () => void
}) {
  const [counted, setCounted] = useState(item.stock?.toString() ?? '')
  const [made, setMade] = useState('')
  const [note, setNote] = useState('')

  return (
    <div className="space-y-5">
      <div className="rounded-xl bg-slate-50 p-3 text-xs text-slate-500">
        <p>필요 <b className="text-slate-800">{item.need}</b> · 제조 필요 <b className="text-slate-800">{item.make}</b></p>
        {item.stock == null ? (
          <p className="mt-1">아직 세어본 적이 없어서 재고를 모르는 상태예요. 지금 세서 적어두면 이후로는 자동으로 맞춰져요.</p>
        ) : (
          <p className="mt-1">실사 {item.count_qty} − 그 뒤 사용 {item.used} + 그 뒤 제조 {item.made} = <b className="text-slate-800">{item.stock}</b></p>
        )}
      </div>

      <form
        onSubmit={(e) => { e.preventDefault(); onCount(Number(counted)) }}
        className="space-y-2"
      >
        <Field label="지금 센 개수 (실사)">
          <Input inputMode="numeric" value={counted} onChange={(e) => setCounted(e.target.value)} placeholder="냉동실에 있는 개수" />
        </Field>
        <p className="text-[11px] text-slate-400">이 값으로 기준을 다시 잡아요. 어긋났다 싶으면 언제든 다시 세면 됩니다.</p>
        <Button type="submit" disabled={counted.trim() === ''} className="w-full">실사로 맞추기</Button>
      </form>

      <form
        onSubmit={(e) => { e.preventDefault(); onBatch(Number(made), note) }}
        className="space-y-2 border-t border-slate-100 pt-4"
      >
        <Field label="방금 만든 개수">
          <Input inputMode="numeric" value={made} onChange={(e) => setMade(e.target.value)} placeholder="예: 12" />
        </Field>
        <Field label="메모">
          <Input value={note} onChange={(e) => setNote(e.target.value)} placeholder="선택" />
        </Field>
        <Button type="submit" variant="ghost" disabled={!made.trim() || Number(made) <= 0} className="w-full">제조 기록 추가</Button>
      </form>

      <div className="flex justify-end">
        <Button type="button" variant="ghost" onClick={onClose}>닫기</Button>
      </div>
    </div>
  )
}
