import type { Card } from '../api'
import type { SortMode, SortOrder } from './hooks'

/**
 * 카드 정렬(칸마다 방향이 달라 클라이언트에서 돈다).
 * 서버 orderBy(store/sort.go)와 규칙이 같아야 한다: 마감 없는 건 항상 뒤, 마지막 동점자는 position.
 */
export function sortCards(cards: Card[], mode: SortMode, order: SortOrder): Card[] {
  const dir = order === 'desc' ? -1 : 1
  const out = [...cards]

  out.sort((a, b) => {
    if (mode === 'manual') {
      return (a.position - b.position) * dir || a.id - b.id
    }

    // 마감 없는 것은 항상 뒤 (dir을 곱하지 않는다)
    const aNull = a.due_at === null
    const bNull = b.due_at === null
    if (aNull !== bNull) return aNull ? 1 : -1

    if (mode === 'time') {
      if (!aNull && a.due_at !== b.due_at) {
        return (a.due_at! < b.due_at! ? -1 : 1) * dir
      }
      if (a.priority !== b.priority) return b.priority - a.priority
    } else {
      if (a.priority !== b.priority) return (b.priority - a.priority) * dir
      if (!aNull && a.due_at !== b.due_at) return a.due_at! < b.due_at! ? -1 : 1
    }
    return a.position - b.position || a.id - b.id
  })
  return out
}
