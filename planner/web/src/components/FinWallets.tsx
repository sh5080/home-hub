import { useState } from 'react'
import { api, type FinOverview, type FinWallet } from '../api'
import { useInvalidating } from '../lib/hooks'
import { fmtWon } from '../lib/date'
import { useConfirm } from './Confirm'
import { Button, Field, Input, Sheet } from './ui'

// 충전식 지갑(지역화폐). 이용 내역을 받을 수 없어 쓴 건 직접 적고, 충전은 통장 내역에서 찾는다.
const nowLocal = () => {
  const d = new Date()
  d.setMinutes(d.getMinutes() - d.getTimezoneOffset())
  return d.toISOString().slice(0, 16)
}
const fmtAt = (at: string) => `${Number(at.slice(5, 7))}/${Number(at.slice(8, 10))} ${at.slice(11, 16)}`
const digits = (v: string) => v.replace(/[^0-9]/g, '')

type SpendEdit = { walletId: number; id?: number; at: string; title: string; amount: string }
type WalletEdit = { owner_id: number; cash: string; incentive: string }

export function Wallets({ d, owner, people, name, Card }: {
  d: FinOverview
  owner: number
  people: { id: number; name: string }[]
  name: (id: number) => string
  Card: (p: { id?: string; title: string; children: React.ReactNode; right?: React.ReactNode; summary?: React.ReactNode }) => React.ReactNode
}) {
  const confirm = useConfirm()
  const keys = [['finance']]
  const [spend, setSpend] = useState<SpendEdit | null>(null)
  const [wallet, setWallet] = useState<WalletEdit | null>(null)
  const [all, setAll] = useState<number | null>(null) // 사용 기록 전부 보는 지갑
  const [err, setErr] = useState<string | null>(null)
  const saveSpend = useInvalidating((x: SpendEdit) => {
    const body = { at: x.at, title: x.title.trim(), amount: Number(x.amount) }
    return x.id ? api.patch(`/api/finance/wallet-spends/${x.id}`, body) : api.post(`/api/finance/wallets/${x.walletId}/spends`, body)
  }, keys)
  const delSpend = useInvalidating((id: number) => api.del(`/api/finance/wallet-spends/${id}`), keys)
  // 추가와 잔액 맞추기는 같은 일이다 — 사람마다 하나, 없으면 만든다.
  const saveWallet = useInvalidating((x: WalletEdit) => api.put('/api/finance/wallets', { owner_id: x.owner_id, cash: Number(x.cash) || 0, incentive: Number(x.incentive) || 0 }), keys)
  const walletOf = (id: number) => d.wallets.find((w) => w.owner_id === id)
  const openBalance = (ownerID: number) => setWallet({ owner_id: ownerID, cash: '', incentive: '' })
  const delWallet = useInvalidating((id: number) => api.del(`/api/finance/wallets/${id}`), keys)

  const total = d.wallets.reduce((a, w) => a + w.balance, 0)
  const run = async (fn: () => Promise<unknown>, done: () => void) => {
    setErr(null)
    try { await fn(); done() } catch (e) { setErr(e instanceof Error ? e.message : '저장하지 못했어요') }
  }

  return (
    <Card id="wallets" title="지역화폐" summary={<span className="text-sm font-bold tabular-nums">{fmtWon(total)}</span>}
      right={<button onClick={() => openBalance(owner || people[0]?.id || 0)} className="text-xs text-muted underline">잔액 맞추기</button>}>
      {d.wallets.length === 0 && <p className="text-xs text-faint">'잔액 맞추기'로 지금 잔액을 넣으면 시작해요. 충전은 통장 내역에서 찾고, 쓴 건 직접 적어요.</p>}
      <div className="space-y-4">
        {d.wallets.map((w) => (
          <WalletView key={w.id} w={w} who={d.wallets.some((x) => x.owner_id !== w.owner_id) ? name(w.owner_id) : ''} showAll={all === w.id}
            onAll={() => setAll(all === w.id ? null : w.id)}
            onAdd={() => setSpend({ walletId: w.id, at: nowLocal(), title: '', amount: '' })}
            onEdit={(x) => setSpend({ walletId: w.id, id: x.id, at: x.at, title: x.title, amount: String(x.amount) })} />
        ))}
      </div>
      <p className="mt-3 text-[11px] text-faint">잔액 = 맞춘 잔액 + 그 뒤 충전(+인센티브) − 적은 사용. 충전은 지갑으로 옮긴 돈이고, 적은 사용이 쓴 돈으로 '한 달에 쓸 수 있는 돈'에서 빠져요. 여유자금엔 넣지 않아요.</p>

      <Sheet open={spend !== null} onClose={() => { setSpend(null); setErr(null) }} title={spend?.id ? '사용 기록 고치기' : '사용 기록'}>
        {spend && (
          <form className="space-y-3" onSubmit={(e) => { e.preventDefault(); run(() => saveSpend.mutateAsync(spend), () => setSpend(null)) }}>
            <Field label="제목"><Input autoFocus value={spend.title} maxLength={60} placeholder="예: 점심 김밥" onChange={(e) => setSpend({ ...spend, title: e.target.value })} /></Field>
            <Field label="금액 (원)"><Input inputMode="numeric" value={spend.amount} onChange={(e) => setSpend({ ...spend, amount: digits(e.target.value) })} /></Field>
            <Field label="날짜·시각"><Input type="datetime-local" value={spend.at} max={nowLocal()} onChange={(e) => setSpend({ ...spend, at: e.target.value })} /></Field>
            {err && <p className="text-xs text-rose-500">{err}</p>}
            <div className="flex gap-2">
              {spend.id && <Button type="button" variant="danger" onClick={async () => {
                if (await confirm({ title: '이 사용 기록을 지울까요?', confirmLabel: '삭제', danger: true })) { delSpend.mutate(spend.id!); setSpend(null) }
              }}>삭제</Button>}
              <div className="flex-1" />
              <Button type="submit" disabled={!spend.title.trim() || !Number(spend.amount) || saveSpend.isPending}>저장</Button>
            </div>
          </form>
        )}
      </Sheet>

      <Sheet open={wallet !== null} onClose={() => { setWallet(null); setErr(null) }} title="잔액 맞추기">
        {wallet && (
          <form className="space-y-3" onSubmit={(e) => { e.preventDefault(); run(() => saveWallet.mutateAsync(wallet), () => setWallet(null)) }}>
            {people.length > 1 && (
              <Field label="누구">
                <div className="flex gap-1.5">
                  {people.map((u) => (
                    <button key={u.id} type="button" onClick={() => openBalance(u.id)}
                      className={`rounded-lg px-3 py-1.5 text-sm font-medium ${wallet.owner_id === u.id ? 'bg-accent text-accent-ink' : 'bg-surface-2 text-muted'}`}>{u.name}</button>
                  ))}
                </div>
              </Field>
            )}
            <div className="grid grid-cols-2 gap-2">
              <Field label="충전 잔액 (원)"><Input autoFocus inputMode="numeric" value={wallet.cash} onChange={(e) => setWallet({ ...wallet, cash: digits(e.target.value) })} /></Field>
              <Field label="인센티브 잔액 (원)"><Input inputMode="numeric" value={wallet.incentive} onChange={(e) => setWallet({ ...wallet, incentive: digits(e.target.value) })} /></Field>
            </div>
            <p className="text-sm">합계 <b className="tabular-nums">{fmtWon((Number(wallet.cash) || 0) + (Number(wallet.incentive) || 0))}</b>
              {walletOf(wallet.owner_id) && <span className="text-xs text-faint"> · 지금 앱에 {fmtWon(walletOf(wallet.owner_id)!.balance)}</span>}</p>
            <p className="text-[11px] text-faint">지역화폐 앱에 보이는 지금 잔액을 넣어요 — 지금까지 쓴 건 이미 빠진 값이에요. 지금 시각이 기준이 되고, 그 뒤 통장에서 충전하면 충전액과 인센티브(10%)가 더해지고 적은 사용은 빠져요. 이전 사용은 날짜를 과거로 적으면 그 달 지출로만 잡혀요.</p>
            {err && <p className="text-xs text-rose-500">{err}</p>}
            <div className="flex gap-2">
              {walletOf(wallet.owner_id) && <Button type="button" variant="danger" onClick={async () => {
                const w = walletOf(wallet.owner_id)!
                if (await confirm({ title: '지역화폐를 지울까요?', body: '적어둔 사용 기록도 함께 사라져요.', confirmLabel: '삭제', danger: true })) { delWallet.mutate(w.id); setWallet(null) }
              }}>삭제</Button>}
              <div className="flex-1" />
              <Button type="submit" disabled={!wallet.owner_id || (wallet.cash === '' && wallet.incentive === '') || saveWallet.isPending}>저장</Button>
            </div>
          </form>
        )}
      </Sheet>
    </Card>
  )
}

function WalletView({ w, who, showAll, onAll, onAdd, onEdit }: {
  w: FinWallet
  who: string
  showAll: boolean
  onAll: () => void
  onAdd: () => void
  onEdit: (x: FinWallet['recent'][number]) => void
}) {
  const list = showAll ? w.recent : w.recent.slice(0, 5)
  return (
    <div>
      <div className="flex items-start justify-between gap-2">
        <div className="min-w-0">
          {who && <p className="truncate text-sm font-semibold">{who}</p>}
          <p className={`text-xl font-bold tabular-nums ${w.balance < 0 ? 'text-rose-500' : ''}`}>{fmtWon(w.balance)}</p>
          <p className="text-[11px] text-faint">이번 달 충전 {fmtWon(w.charged)}{w.earned > 0 && ` (+적립 ${fmtWon(w.earned)})`} · 사용 {fmtWon(w.spent)}</p>
        </div>
        <Button onClick={onAdd} className="shrink-0 px-3 py-1.5 text-xs">+ 사용 기록</Button>
      </div>
      {w.recent.length > 0 && (
        <ul className="mt-2 divide-y divide-line">
          {list.map((x) => (
            <li key={x.id}>
              <button type="button" onClick={() => onEdit(x)} className="-mx-1 flex w-full items-baseline gap-2 rounded-lg px-1 py-1.5 text-left text-sm">
                <span className="w-20 shrink-0 text-xs tabular-nums text-faint">{fmtAt(x.at)}</span>
                <span className="min-w-0 flex-1 truncate">{x.title}</span>
                <span className="shrink-0 tabular-nums">{fmtWon(x.amount)}</span>
              </button>
            </li>
          ))}
        </ul>
      )}
      {w.recent.length > 5 && <button onClick={onAll} className="mt-1 text-xs text-muted underline">{showAll ? '접기' : `최근 ${w.recent.length}건 모두 보기`}</button>}
    </div>
  )
}
