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
  /** 반복 규칙. daily | every:N | weekly:마스크 | monthly:일 | yearly:MM-DD */
  recur: string | null
  recur_until: string | null
  /** 이 카드를 낳은 앞 회차 */
  recur_parent_id: number | null
  /** 규칙을 사람이 읽는 말로. 해석은 서버가 한다. */
  recur_label: string
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

// --- 이유식 ---

/** 검색 결과 한 줄. 어느 보드의 어느 칸인지까지 온다. */
export interface SearchResult extends Omit<Card, 'content'> {
  board_id: number
  board_name: string
  column_name: string
  /** 본문에서 검색어 주변을 잘라낸 것. 제목에만 맞으면 빈 문자열. */
  snippet: string
}

/** 아이 정보와 계산 기준. 생일은 앱에서 입력한다. */
export interface BFProfile {
  name: string
  birth_date: string | null
  horizon_days: number
  today_dday: number | null
  today: string
  from_dday: number | null
  to_dday: number | null
}

/** 시드 당시의 끼니 구성. "원래 …"를 보여주는 데만 쓴다. */
export interface BFMealSrc {
  base: string
  toppings: string[]
  snack: string | null
}

export interface BFMeal {
  id: number
  slot: string
  base: string
  toppings: string[]
  snack: string | null
  src: BFMealSrc
  /** 지금 값이 시드 원본과 다른가 */
  edited: boolean
  eaten_g: number | null
  served_g: number | null
  skipped: boolean
  edited_by: number | null
  edited_at: number | null
}

export interface BFDay {
  dday: number
  date: string
  stage: string
  label: string
  /** 'topping' = 큐브로 차리는 구간, 'menu' = 요리 이름으로 적힌 구간 */
  kind: string
  new_item: string | null
  new_item_src: string | null
  note: string
  meals: BFMeal[]
}

export interface BFRangeData {
  profile: BFProfile
  days: BFDay[]
}

/** 재료 한 종류와 거기 달린 표시. 반응과 좋아함은 서로 독립이다. */
export interface BFFood {
  name: string
  kind: 'base' | 'cube' | 'dish'
  reaction: boolean
  liked: boolean
  tag_at: number | null
  tag_by: number | null
  first_dday: number | null
  first_date: string
  uses: number
}

export interface BFStock {
  name: string
  kind: 'base' | 'cube' | 'dish'
  need: number
  /** null이면 아직 실사하지 않음 — 0과 구분해야 한다 */
  stock: number | null
  make: number
  count_qty: number | null
  count_dday: number | null
  count_at: number | null
  used: number
  made: number
}

export interface BFStockView {
  from: string
  to: string
  from_dday: number
  to_dday: number
  horizon_days: number
  items: BFStock[]
}
