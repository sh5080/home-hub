import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api, type Board, type BoardDetail, type Event, type Routine, type User } from '../api'

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

export function useBoard(id: number, sort: SortMode = 'manual') {
  return useQuery({ queryKey: ['board', id, sort], queryFn: () => api.get<BoardDetail>(`/api/boards/${id}?sort=${sort}`) })
}

export function useRoutinesForDate(date: string) {
  return useQuery({ queryKey: ['routines', 'date', date], queryFn: () => api.get<Routine[]>(`/api/routines?date=${date}`) })
}

export interface TodayData {
  routines: Routine[]
  cards: (import('../api').Card & { board_id: number; board_name: string; done_column_id: number })[]
}

export function useToday(date: string, sort: SortMode = 'time') {
  return useQuery({ queryKey: ['today', date, sort], queryFn: () => api.get<TodayData>(`/api/today?date=${date}&sort=${sort}`) })
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
  events: Event[]
  due_cards: import('../api').Card[]
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
