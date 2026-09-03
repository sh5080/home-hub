import { useUsers } from '../lib/hooks'
import { fmtStamp } from '../lib/date'

/** '누가 · 언제'. 사람을 모르면(예전 데이터) 날짜만 보여준다. */
export default function Byline({ label = '만든 사람', by, at, className = '' }: {
  label?: string
  by?: number | null
  at?: number | null
  className?: string
}) {
  const users = useUsers()
  if (!at && !by) return null
  const name = by ? users.data?.find((u) => u.id === by)?.name : undefined
  const parts = [name, at ? fmtStamp(at) : undefined].filter(Boolean)
  if (parts.length === 0) return null
  return (
    <p className={`text-[11px] text-faint ${className}`}>
      {label} · {parts.join(' · ')}
    </p>
  )
}
