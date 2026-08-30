import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api, type BFFood, type BFProfile, type BFRangeData, type BFStockView, type SearchResult, type Board, type BoardDetail, type Card, type Routine, type User } from '../api'

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

/** 캘린더는 별도 데이터가 아니라 날짜가 있는 카드를 기간으로 본 것이다. */
export interface CalendarData {
  cards: Card[]
}

export function useCalendar(from: string, to: string) {
  return useQuery({ queryKey: ['calendar', from, to], queryFn: () => api.get<CalendarData>(`/api/calendar?from=${from}&to=${to}`) })
}

/** 변경 후 관련 쿼리를 통째로 무효화하는 뮤테이션 헬퍼. 단순함이 우선. */
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
    queryFn: () => api.get<SearchResult[]>(`/api/search?q=${encodeURIComponent(term)}`),
    enabled: term !== '',
    staleTime: 30_000,
  })
}

// --- 이유식 ---

/** 날짜 구간의 식단. 서버가 D+n 으로 저장하고 날짜로 답한다. */
export function useBabyfood(from: string, to: string) {
  return useQuery({
    queryKey: ['babyfood', from, to],
    queryFn: () => api.get<BFRangeData>(`/api/babyfood?from=${from}&to=${to}`),
  })
}

/** 먹어본 음식 전체. 100종 남짓이라 한 번에 받아 화면에서 나눈다. */
export function useBFFoods() {
  return useQuery({ queryKey: ['babyfood-foods'], queryFn: () => api.get<BFFood[]>('/api/babyfood/foods') })
}

export function useBFProfile() {
  return useQuery({ queryKey: ['babyfood', 'profile'], queryFn: () => api.get<BFProfile>('/api/babyfood/profile') })
}

/** days를 주면 설정을 바꾸지 않고 그 기간으로만 계산해 본다. */
export function useBFStock(days?: number) {
  return useQuery({
    queryKey: ['babyfood-stock', days ?? 0],
    queryFn: () => api.get<BFStockView>(`/api/babyfood/stock${days ? `?days=${days}` : ''}`),
  })
}
