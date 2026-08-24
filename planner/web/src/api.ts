// 서버 API 클라이언트. 세션은 HttpOnly 쿠키라 여기서 토큰을 다루지 않는다.
// 401이면 로그인으로 보낸다 (로그인 페이지 자체는 제외).

export class ApiError extends Error {
  status: number
  constructor(status: number, message: string) {
    super(message)
    this.status = status
  }
}

async function request<T>(method: string, path: string, body?: unknown): Promise<T> {
  const res = await fetch(path, {
    method,
    credentials: 'same-origin',
    headers: body !== undefined ? { 'Content-Type': 'application/json' } : undefined,
    body: body !== undefined ? JSON.stringify(body) : undefined,
  })

  if (res.status === 401 && !location.pathname.startsWith('/login')) {
    location.assign('/login')
    throw new ApiError(401, '로그인이 필요해요')
  }

  if (res.status === 204) return undefined as T

  const text = await res.text()
  let data: unknown = undefined
  if (text) {
    try {
      data = JSON.parse(text)
    } catch {
      data = text
    }
  }

  if (!res.ok) {
    const msg =
      typeof data === 'object' && data && 'error' in data
        ? String((data as { error: unknown }).error)
        : res.statusText
    throw new ApiError(res.status, msg)
  }
  return data as T
}

export const api = {
  get: <T>(path: string) => request<T>('GET', path),
  post: <T>(path: string, body?: unknown) => request<T>('POST', path, body ?? {}),
  patch: <T>(path: string, body: unknown) => request<T>('PATCH', path, body),
  put: <T>(path: string, body?: unknown) => request<T>('PUT', path, body ?? {}),
  del: <T>(path: string) => request<T>('DELETE', path),
}

// --- 타입 (서버 snake_case 그대로) ---

export interface User {
  id: number
  name: string
}

export interface Board {
  id: number
  name: string
  card_count?: number
}

export interface Card {
  id: number
  column_id: number
  title: string
  /** content에서 파생한 평문. 미리보기·검색용이고 쓰기는 content로 한다. */
  description: string
  /** 권위 있는 본문. 블록 문서 JSON 문자열. 아직 옮기지 않았으면 null. */
  content: string | null
  position: number
  /** 'YYYY-MM-DD' 또는 'YYYY-MM-DDTHH:MM'. 시각은 선택이다. */
  due_at: string | null
  /** 여러 날에 걸치는 항목의 끝. 없으면 하루짜리. */
  end_at: string | null
  /** 0=없음, 1~3 */
  priority: number
  assignee_id: number | null
  created_by: number
  updated_at: number
}

export interface Column {
  id: number
  board_id: number
  name: string
  position: number
  cards: Card[]
}

export interface BoardDetail {
  board: Board
  columns: Column[]
}


export interface Routine {
  id: number
  title: string
  weekdays_mask: number
  time_of_day: string | null
  assignee_id: number | null
  active: boolean
  position: number
  // GET /api/routines?date= 일 때만
  checked_by?: number | null
  checked_at?: number | null
}
