import type { Card } from '../api'
import type { SortMode, SortOrder } from './hooks'

/**
 * 카드 정렬. 컬럼마다 다른 기준을 쓸 수 있어야 해서 클라이언트에서 돈다.
 *
 * **서버의 orderBy(internal/store/sort.go)와 규칙이 같아야 한다.** 홈의
 * "오늘 할 일"은 여러 보드를 가로지르는 조회라 서버에서 정렬하고, 보드는
 * 컬럼별로 달라야 해서 여기서 정렬한다. 규칙을 바꾸면 양쪽을 함께 고친다.
 *
 * 공통 규칙 두 가지:
 *   - 마감 없는 카드는 방향과 무관하게 항상 뒤. 역순이라고 날짜 없는 것이
 *     앞에 몰리면 목록을 읽을 수 없다.
 *   - 마지막 동점자는 position — 손으로 정한 순서가 최종 기준이다.
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
