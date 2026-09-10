import { shrinkImage } from './lib/image'
// API 클라이언트. 세션은 HttpOnly 쿠키. 401 이면 로그인으로(로그인 페이지 제외).

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
  /** 'YYYY-MM-DD'. 어른도 포함, 아이는 이유식 기준. */
  birth_date: string | null
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
  /** content 에서 파생한 평문(미리보기·검색). 쓰기는 content 로. */
  description: string
  /** 본문 원본(블록 문서 JSON). */
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
  /** 규칙을 사람이 읽는 말로(서버가 해석). */
  recur_label: string
  /** 0=없음, 1~3 */
  priority: number
  assignee_id: number | null
  created_by: number
  created_at: number
  updated_at: number
  /** 보관함·카드 상세에서만 온다(omitempty — 비면 필드가 없다). */
  done_at?: number | null
  archived_at?: number | null
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
  /** 0010 이전 루틴은 null. */
  created_by?: number | null
  created_at?: number
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

export interface SearchResult extends Omit<Card, 'content'> {
  board_id: number
  board_name: string
  column_name: string
  /** 본문에서 검색어 주변을 잘라낸 것. 제목에만 맞으면 빈 문자열. */
  snippet: string
}

export interface SearchAll {
  cards: SearchResult[]
  routines: Routine[]
  foods: BFFood[]
  diary: DiaryEntry[]
}

/** 이유식 대상 아이. 생년월일은 User 에 있다. */
export interface BFChild {
  user_id: number
  name: string
  birth_date: string
  horizon_days: number
  today_dday: number
  today: string
  from_dday: number | null
  to_dday: number | null
  days: number
  /** 하루 n끼일 때의 기본 시각. { "1": ["12:00"], ... } */
  meal_times: Record<string, string[]>
}

/** 시드 당시의 끼니 구성("원래 …" 표시용). */
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
  /** 그날 끼니 수로 정해지는 이름. 1끼면 '점심' */
  title: string
  /** 'HH:MM'. 직접 넣은 값이 없으면 설정의 기본값 */
  at: string
  at_set: boolean
  /** 이 끼니에 걸린 이유식 기록. null 이면 아직 */
  eaten: { log_id: number; at: string; amount_ml: number | null } | null
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
  /** 그날 남긴 반응·좋아함. 표시가 있는 재료만 들어온다 */
  logs: BFLog[]
}

/** (아이, 날짜, 재료) 하나에 대한 기록 */
export interface BFLog {
  dday: number
  date: string
  name: string
  reaction: boolean
  liked: boolean
  disliked: boolean
  at: number
  by: number | null
}

export interface BFRangeData {
  children: BFChild[]
  /** 아이가 없으면 없다 */
  child?: BFChild
  days: BFDay[]
}

/** 재료 한 종류와 기록 요약. 반응·좋아함·싫어함은 서로 독립(좋아함·싫어함만 배타). */
export interface BFFood {
  name: string
  kind: 'base' | 'cube' | 'dish'
  /** 한 번이라도 있었는가 */
  reaction: boolean
  liked: boolean
  disliked: boolean
  /** 언제 그랬는지 (YYYY-MM-DD) */
  reaction_dates: string[]
  liked_dates: string[]
  disliked_dates: string[]
  last_at: number | null
  last_by: number | null
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

export interface DiaryRef {
  kind: 'card' | 'routine' | 'babyfood'
  ref_id: number | null
  ref_date: string
  child_id: number | null
  /** 걸던 때의 이름. 대상이 바뀌거나 지워져도 그대로 */
  label: string
  /** 대상이 지금은 없다 */
  gone: boolean
}

export interface DiaryEntry {
  id: number
  date: string
  title: string
  /** 목록 조회에서는 null — 본문은 상세에서만 */
  content: string | null
  /** 본문에서 뽑은 평문 (목록은 앞부분만) */
  plain: string
  created_by: number | null
  created_at: number
  updated_at: number
  refs: DiaryRef[]
  /** 본문의 사진 앞 몇 장 (/media/...) */
  photos: string[]
  /** 다른 앱에서 가져온 글이면 출처 */
  source: string | null
}

export interface Media {
  id: string
  url: string
  bytes: number
  width: number
  height: number
}

/** 사진 올리기(multipart 라 api.post 를 안 쓴다). */
export async function uploadMedia(file: File): Promise<Media> {
  // 폰에서 먼저 줄인다 — 원본을 Pi 가 처리하면 한 장에 몇 분.
  const small = await shrinkImage(file)
  const fd = new FormData()
  fd.append('file', small, 'photo.jpg')
  const res = await fetch('/api/media', { method: 'POST', body: fd, credentials: 'same-origin' })
  if (!res.ok) {
    let msg = '사진을 올리지 못했어요'
    try { msg = (await res.json()).error ?? msg } catch { /* 본문이 JSON 이 아닐 수 있다 */ }
    throw new Error(msg)
  }
  return res.json()
}

/** 육아 기록 종류(서버가 순서를 정한다) */
export interface CareKind {
  key: string
  label: string
  duration: boolean
  amount: boolean
  details: string[] | null
  free: boolean
}

export interface CareLog {
  id: number
  child_id: number
  kind: string
  /** 'YYYY-MM-DDTHH:MM' */
  at: string
  /** null 이면 아직 진행 중(수면 등) */
  minutes: number | null
  amount_ml: number | null
  detail: string | null
  note: string
  created_by: number | null
  created_at: number
  /** 이유식: 먹인 재료(식단 재료 목록) */
  items: { id: number; name: string }[]
  /** 이유식: 식단의 어느 끼니인지 */
  meal_id: number | null
}

export interface CareDay {
  date: string
  logs: CareLog[]
  formula_ml: number
  solids_ml: number
  sleep_min: number
  night_min: number
  nap_min: number
  diapers: number
  breast_min: number
  feedings: number
}

// --- 재정 ---
export interface FinItem {
  owner_id: number
  group: 'liquid' | 'saving' | 'invest' | 'insurance' | 'pension' | 'other' | 'debt'
  category: string
  institution: string
  name: string
  amount: number
  principal: number | null
  rate: number | null
}
export interface FinMonth { month: string; in: number; other_in: number; fixed_in: number; fixed_save: number; unexpected: number; spend: number; save: number; fixed: number; variable: number; partial: boolean }
export interface FinFixed { key: string; label: string; cat1: string; kind: 'spend' | 'save'; monthly: number; months: number; auto: boolean; fixed: boolean; override: boolean | null; edited: boolean }
export interface FinDetail {
  months: { month: string; amount: number; count: number }[]
  tx: { owner_id: number; at: string; type: string; cat1: string; cat2: string; content: string; amount: number; method: string; memo: string }[]
}
export interface FinTag { id: number; name: string; color: string }
export interface FinIncome { key: string; keys: string[]; edited: boolean; label: string; cat1: string; monthly: number; months: number; total: number; auto: boolean; income: boolean; override: boolean | null
  months6: number; fixed: boolean; fixed_auto: boolean; fixed_override: boolean | null }
export interface FinNotSpend { key: string; label: string; cat1: string; total: number; count: number; edited: boolean }
export interface FinIncomeTx { group: string; keys: string[]; at: string; content: string; cat1: string; amount: number; edited: boolean }
export interface FinUnexpected { key: string; edited: boolean; at: string; content: string; cat1: string; amount: number; reason: string }
export interface FinPlan {
  income_base: number; income_hint: number; fixed_income: number; fixed_spend: number; fixed_save: number; goals_monthly: number
  spendable: number; variable_avg: number; buffer_months: number; liquid: number; emergency: number; spend_avg: number; free: number
}
export interface FinGoal { id: number; name: string; target: number; due: string | null; monthly: number; items: string[]; current: number }
export interface FinOverview {
  owners: number[]
  as_of: Record<string, string>
  total_asset: number; total_debt: number; net_worth: number
  groups: Record<string, number>
  items: FinItem[]
  history: { month: string; net_worth: number }[] | null
  months: FinMonth[]
  fixed: FinFixed[]
  income: FinIncome[]
  income_tx: FinIncomeTx[]
  not_spend: FinNotSpend[]
  tags: FinTag[]
  labels: Record<string, { label: string; cat1: string }>
  tag_links: Record<string, number[]>
  tag_spend: { tag_id: number; amount: number; count: number }[]
  untagged: number
  month: string
  unexpected: FinUnexpected[]
  plan: FinPlan
  goals: FinGoal[]
  tx_count: number
  upload_days: number
  uploaded: Record<string, number>
}
/** 올리는 파일의 규격(서버 설정). 서비스 이름은 코드에 두지 않는다. */
export interface FinFormat {
  configured: boolean
  source_name?: string; how_to?: string
  summary_sheet?: string; tx_sheet?: string
  sections?: string[]
  tx_header?: Record<string, string>
}
export interface FinImportResult {
  taken_at: string; items: number; groups: Record<string, number>; total_asset: number; total_debt: number
  tx_total: number; tx_new: number; tx_from: string; tx_to: string; replace_snapshot: boolean
}

/** 재정 파일 올리기. apply=false 면 미리보기만. */
export async function importFinance(file: File, owner: number, apply: boolean): Promise<FinImportResult> {
  const fd = new FormData()
  fd.append('file', file)
  fd.append('owner', String(owner))
  if (apply) fd.append('apply', '1')
  const res = await fetch('/api/finance/import', { method: 'POST', body: fd, credentials: 'same-origin' })
  if (!res.ok) {
    let msg = '파일을 읽지 못했어요'
    try { msg = (await res.json()).error ?? msg } catch { /* JSON 이 아닐 수 있다 */ }
    throw new Error(msg)
  }
  return res.json()
}
