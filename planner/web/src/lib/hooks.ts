import { useMutation, useQuery, useQueryClient, useInfiniteQuery } from '@tanstack/react-query'
import { api, type BFChild, type BFFood, type BFRangeData, type BFStockView, type SearchAll, type Board, type BoardDetail, type Card, type Routine, type User, type DiaryEntry, type CareDay, type CareKind, type CareLog, type FinOverview, type FinFormat, type FamilyBoard} from '../api'

export function useMe() {
  return useQuery({ queryKey: ['me'], queryFn: () => api.get<User>('/api/me'), staleTime: Infinity })
}

export function useUsers() {
  return useQuery({ queryKey: ['users'], queryFn: () => api.get<User[]>('/api/users'), staleTime: 60_000 })
}

export function useBoards() {
  return useQuery({ queryKey: ['boards'], queryFn: () => api.get<Board[]>('/api/boards') })
}

export type SortMode = 'manual' | 'time' | 'priority'

export type SortOrder = 'asc' | 'desc'

export function useBoard(id: number, sort: SortMode = 'time', order: SortOrder = 'asc') {
  return useQuery({
    queryKey: ['board', id, sort, order],
    queryFn: () => api.get<BoardDetail>(`/api/boards/${id}?sort=${sort}&order=${order}`),
  })
}

export function useRoutinesForDate(date: string) {
  return useQuery({ queryKey: ['routines', 'date', date], queryFn: () => api.get<Routine[]>(`/api/routines?date=${date}`) })
}

export interface TodayData {
  routines: Routine[]
  cards: (Card & { board_id: number; board_name: string; done_column_id: number })[]
}

export function useToday(date: string, sort: SortMode = 'time', order: SortOrder = 'asc') {
  return useQuery({
    queryKey: ['today', date, sort, order],
    queryFn: () => api.get<TodayData>(`/api/today?date=${date}&sort=${sort}&order=${order}`),
  })
}

export function useRoutines() {
  return useQuery({ queryKey: ['routines', 'all'], queryFn: () => api.get<Routine[]>('/api/routines') })
}

export function useRoutineChecks(from: string, to: string) {
  return useQuery({
    queryKey: ['routines', 'checks', from, to],
    queryFn: () => api.get<Record<string, string[]>>(`/api/routines/checks?from=${from}&to=${to}`),
  })
}

export interface CalendarData {
  cards: Card[]
  /** 일기가 있는 날짜 — 격자에 점 하나로 표시한다 */
  diary_dates: string[]
}

export function useCalendar(from: string, to: string) {
  return useQuery({ queryKey: ['calendar', from, to], queryFn: () => api.get<CalendarData>(`/api/calendar?from=${from}&to=${to}`) })
}

/** 변경 후 관련 쿼리를 통째로 무효화하는 뮤테이션 헬퍼. */
export function useInvalidating<TArgs, TResult = unknown>(fn: (args: TArgs) => Promise<TResult>, keys: readonly (string | number)[][]) {
  const qc = useQueryClient()
  return useMutation<TResult, Error, TArgs>({
    mutationFn: fn,
    onSettled: () => keys.forEach((k) => qc.invalidateQueries({ queryKey: k })),
  })
}

/** 빈 질의는 서버에 묻지 않는다 — 결과가 어차피 비어 있다. */
export function useSearch(q: string) {
  const term = q.trim()
  return useQuery({
    queryKey: ['search', term],
    queryFn: () => api.get<SearchAll>(`/api/search?q=${encodeURIComponent(term)}`),
    enabled: term !== '',
    staleTime: 30_000,
  })
}


/** 아이를 안 주면 대상이 한 명일 때만 서버가 고른다. */
export function useBabyfood(from: string, to: string, child?: number) {
  const q = child ? `&child=${child}` : ''
  return useQuery({
    queryKey: ['babyfood', from, to, child ?? 0],
    queryFn: () => api.get<BFRangeData>(`/api/babyfood?from=${from}&to=${to}${q}`),
  })
}

export function useBFChildren() {
  return useQuery({ queryKey: ['babyfood', 'children'], queryFn: () => api.get<BFChild[]>('/api/babyfood/children') })
}

export function useBFFoods(child?: number) {
  const q = child ? `?child=${child}` : ''
  return useQuery({ queryKey: ['babyfood-foods', child ?? 0], queryFn: () => api.get<BFFood[]>(`/api/babyfood/foods${q}`) })
}



/** days 를 주면 설정을 바꾸지 않고 그 기간으로만 계산한다. */
export function useBFStock(days?: number, child?: number) {
  const qs = [days ? `days=${days}` : '', child ? `child=${child}` : ''].filter(Boolean).join('&')
  return useQuery({
    queryKey: ['babyfood-stock', days ?? 0, child ?? 0],
    queryFn: () => api.get<BFStockView>(`/api/babyfood/stock${qs ? `?${qs}` : ''}`),
  })
}

export function useDiary(from?: string, to?: string) {
  const qs = new URLSearchParams()
  if (from) qs.set('from', from)
  if (to) qs.set('to', to)
  const suffix = qs.toString() ? `?${qs}` : ''
  return useQuery({ queryKey: ['diary', from ?? '', to ?? ''], queryFn: () => api.get<DiaryEntry[]>(`/api/diary${suffix}`) })
}

export function useDiaryEntry(id: number, enabled = true) {
  return useQuery({ queryKey: ['diary-entry', id], queryFn: () => api.get<DiaryEntry>(`/api/diary/${id}`), enabled })
}

const DIARY_PAGE = 40

/** 다이어리 피드. 40장씩, 마지막 글의 (날짜, id) 커서로. 키가 'diary' 로 시작해 무효화가 닿는다. */
export function useDiaryFeed() {
  return useInfiniteQuery({
    queryKey: ['diary', 'feed'],
    initialPageParam: null as { date: string; id: number } | null,
    queryFn: ({ pageParam }) => {
      const qs = new URLSearchParams({ limit: String(DIARY_PAGE) })
      if (pageParam) { qs.set('before_date', pageParam.date); qs.set('before_id', String(pageParam.id)) }
      return api.get<DiaryEntry[]>(`/api/diary?${qs}`)
    },
    getNextPageParam: (last) => (last.length < DIARY_PAGE ? undefined : { date: last[last.length - 1].date, id: last[last.length - 1].id }),
  })
}

export function useCareKinds() {
  return useQuery({ queryKey: ['care-kinds'], queryFn: () => api.get<CareKind[]>('/api/care/kinds'), staleTime: Infinity })
}
export function useCareDay(date: string, child?: number) {
  const qs = new URLSearchParams({ date })
  if (child) qs.set('child', String(child))
  return useQuery({ queryKey: ['care', date, child ?? 0], queryFn: () => api.get<CareDay>(`/api/care?${qs}`) })
}
export function useCareLast(child?: number) {
  const qs = child ? `?child=${child}` : ''
  return useQuery({ queryKey: ['care-last', child ?? 0], queryFn: () => api.get<Record<string, CareLog>>(`/api/care/last${qs}`), refetchInterval: 60_000 })
}

export function useFinance(owner: number, month?: string) {
  const qs = new URLSearchParams({ owner: String(owner) })
  if (month) qs.set('month', month)
  // 달만 바꿀 땐 받는 동안 이전 화면을 유지한다(뼈대로 바뀌면 페이지가 짧아져 스크롤이 튄다). 사람을 바꾸면 비운다.
  return useQuery({
    queryKey: ['finance', owner, month ?? ''],
    queryFn: () => api.get<FinOverview>(`/api/finance/overview?${qs}`),
    placeholderData: (prev, prevQuery) => (prevQuery?.queryKey[1] === owner ? prev : undefined),
  })
}

export function useFinFormat() {
  return useQuery({ queryKey: ['finance', 'format'], queryFn: () => api.get<FinFormat>('/api/finance/format'), staleTime: Infinity })
}

export function useFamily() {
  return useQuery({ queryKey: ['family'], queryFn: () => api.get<FamilyBoard>('/api/family') })
}
