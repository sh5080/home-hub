import { useQuery } from '@tanstack/react-query'
import { api } from '../api'

interface Storage {
  quota: number
  used_bytes: number
  disk_total: number
  disk_free: number
  disk_used: number
  db_bytes: number
  backup_bytes: number
  backup_count: number
  media_bytes: number
  media_count: number
  bytes_per_card: number
  cards: number
  boards: number
  routines: number
  users: number
}

/** 바이트를 사람이 읽는 단위로. 소수점은 GB 이상에서만 쓴다. */
function fmtBytes(n: number) {
  if (n < 1024) return `${n} B`
  const kb = n / 1024
  if (kb < 1024) return `${Math.round(kb)} KB`
  const mb = kb / 1024
  if (mb < 1024) return mb < 10 ? `${mb.toFixed(1)} MB` : `${Math.round(mb)} MB`
  return `${(mb / 1024).toFixed(1)} GB`
}

function fmtCount(n: number) {
  if (n >= 100_000_000) return `${Math.round(n / 100_000_000)}억`
  if (n >= 10_000) return `${Math.round(n / 10_000)}만`
  if (n >= 1_000) return `${(n / 1_000).toFixed(1)}천`
  return String(Math.round(n))
}

export default function StorageBar() {
  const q = useQuery({
    queryKey: ['storage'],
    queryFn: () => api.get<Storage>('/api/storage'),
    staleTime: 60_000,
    refetchInterval: false,
  })

  if (q.isPending) {
    return <div className="h-16 animate-pulse rounded-xl bg-surface-2" />
  }
  if (q.isError || !q.data) return null

  const s = q.data
  const pct = Math.min(100, (s.used_bytes / s.quota) * 100)
  // 1% 미만이어도 막대가 보이게.
  const width = Math.max(pct, s.used_bytes > 0 ? 1.5 : 0)
  const tone = pct >= 90 ? 'bg-rose-500' : pct >= 70 ? 'bg-amber-500' : 'bg-emerald-500'

  const left = Math.max(0, s.quota - s.used_bytes)
  const perCard = s.bytes_per_card > 0 ? s.bytes_per_card : 2500 // 카드가 없을 때의 대략치
  const roomFor = Math.floor(left / perCard)

  return (
    <div className="space-y-2">
      <div className="flex items-baseline justify-between">
        <span className="text-xs text-faint">
          {fmtBytes(s.used_bytes)} / {fmtBytes(s.quota)} · {pct < 0.1 ? '0.1% 미만' : `${pct.toFixed(1)}%`}
        </span>
      </div>

      <div className="h-2 overflow-hidden rounded-full bg-line">
        <div className={`h-full rounded-full transition-all ${tone}`} style={{ width: `${width}%` }} />
      </div>

      <p className="text-xs text-muted">
        카드 약 <b>{fmtCount(roomFor)}장</b>을 더 넣을 수 있어요
      </p>

      <dl className="grid grid-cols-2 gap-x-3 gap-y-1 border-t border-line pt-2 text-[11px] text-faint">
        <Row label="데이터" value={fmtBytes(s.db_bytes)} />
        <Row label={`백업 ${s.backup_count}개`} value={fmtBytes(s.backup_bytes)} />
        <Row label={`사진 ${s.media_count}장`} value={fmtBytes(s.media_bytes)} />
        <Row label="카드" value={`${s.cards}장`} />
        <Row label="루틴" value={`${s.routines}개`} />
        <Row label="카드당" value={fmtBytes(s.bytes_per_card)} />
        <Row label="SD카드 남음" value={fmtBytes(s.disk_free)} />
      </dl>
    </div>
  )
}

function Row({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex justify-between gap-2">
      <dt className="truncate">{label}</dt>
      <dd className="shrink-0 font-medium text-ink-2">{value}</dd>
    </div>
  )
}
