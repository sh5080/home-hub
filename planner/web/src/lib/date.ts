// 날짜 유틸. 서버는 타임존 없는 로컬 문자열('YYYY-MM-DD', 'YYYY-MM-DDTHH:MM')을 쓴다.
// 여기서도 Date 객체는 로컬 시각으로만 다루고 UTC 변환은 절대 하지 않는다.

export function pad(n: number) {
  return n < 10 ? `0${n}` : String(n)
}

/** Date → 'YYYY-MM-DD' (로컬) */
export function toDateStr(d: Date) {
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`
}

/** Date → 'YYYY-MM-DDTHH:MM' (로컬) */
export function toDateTimeStr(d: Date) {
  return `${toDateStr(d)}T${pad(d.getHours())}:${pad(d.getMinutes())}`
}

/** 'YYYY-MM-DD' → Date (로컬 자정) */
export function fromDateStr(s: string) {
  const [y, m, d] = s.slice(0, 10).split('-').map(Number)
  return new Date(y, m - 1, d)
}

export function today() {
  return toDateStr(new Date())
}

export function addDays(s: string, n: number) {
  const d = fromDateStr(s)
  d.setDate(d.getDate() + n)
  return toDateStr(d)
}

/**
 * 저장 형식과 표시 순서가 다르다.
 *
 *   저장(루틴 weekdays_mask): ISO — bit0=월 … bit6=일
 *   표시: 일월화수목금토 — 한국 달력 관행
 *
 * 마스크를 바꾸면 기존 데이터를 마이그레이션해야 하므로 표시만 돌린다.
 */

/** ISO 요일 인덱스: 월=0 … 일=6. 저장 마스크의 비트 번호다. */
export function isoWeekday(s: string) {
  return (fromDateStr(s).getDay() + 6) % 7
}

/** 표시 요일 인덱스: 일=0 … 토=6. JS getDay()와 같다. */
export function weekdayIndex(s: string) {
  return fromDateStr(s).getDay()
}

/** 표시 인덱스(일=0) → 저장 마스크 비트(월=0) */
export function maskBit(displayIndex: number) {
  return displayIndex === 0 ? 6 : displayIndex - 1
}

/** 그 주 일요일 */
export function startOfWeek(s: string) {
  return addDays(s, -weekdayIndex(s))
}

/** 그 달 1일 */
export function startOfMonth(s: string) {
  return s.slice(0, 7) + '-01'
}

export function addMonths(s: string, n: number) {
  const d = fromDateStr(s)
  d.setMonth(d.getMonth() + n, 1)
  return toDateStr(d)
}

/** 표시 순서. 인덱스는 weekdayIndex()(일=0)와 맞춘다. */
export const WEEKDAYS = ['일', '월', '화', '수', '목', '금', '토']

/** '2026-09-21' → '9월 21일 (월)' */
export function fmtDate(s: string) {
  const d = fromDateStr(s)
  return `${d.getMonth() + 1}월 ${d.getDate()}일 (${WEEKDAYS[weekdayIndex(s)]})`
}

/** '2026-09-21T10:30' → '10:30', '2026-09-21' → '' */
export function fmtTime(s: string) {
  return s.length > 10 ? s.slice(11, 16) : ''
}

/** 마감 값에 시각이 들어 있나 */
export function hasTime(s: string) {
  return s.length > 10
}

/** 시각을 오전/오후로 태깅. 정확한 시:분은 상세 페이지에서만 보여준다. */
export function ampm(s: string) {
  if (!hasTime(s)) return ''
  return Number(s.slice(11, 13)) < 12 ? '오전' : '오후'
}

/** 목록에 쓰는 마감 표기: "내일 오후" 처럼 날짜 + 오전/오후 */
export function fmtDue(s: string) {
  const label = dueLabel(s)
  const t = ampm(s)
  return t ? `${label} ${t}` : label
}

/** 마감일 상대 표기 */
export function dueLabel(s: string) {
  const diff = Math.round((fromDateStr(s).getTime() - fromDateStr(today()).getTime()) / 86400000)
  if (diff === 0) return '오늘'
  if (diff === 1) return '내일'
  if (diff === -1) return '어제'
  if (diff < 0) return `${-diff}일 지남`
  if (diff < 7) return `${diff}일 후`
  return `${fromDateStr(s).getMonth() + 1}/${fromDateStr(s).getDate()}`
}
