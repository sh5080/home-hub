// 서버는 타임존 없는 로컬 문자열('YYYY-MM-DD', 'YYYY-MM-DDTHH:MM')을 쓴다.
// Date 는 로컬 시각으로만 다루고 UTC 변환은 하지 않는다.

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

/** 저장(weekdays_mask)은 ISO(bit0=월), 표시는 일월화수목금토. 표시만 돌린다. */

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

/** 오전/오후 태그. */
export function ampm(s: string) {
  if (!hasTime(s)) return ''
  return Number(s.slice(11, 13)) < 12 ? '오전' : '오후'
}

export function fmtDue(s: string) {
  const label = dueLabel(s)
  const t = ampm(s)
  return t ? `${label} ${t}` : label
}

export function dueLabel(s: string) {
  const diff = Math.round((fromDateStr(s).getTime() - fromDateStr(today()).getTime()) / 86400000)
  if (diff === 0) return '오늘'
  if (diff === 1) return '내일'
  if (diff === -1) return '어제'
  if (diff < 0) return `${-diff}일 지남`
  if (diff < 7) return `${diff}일 후`
  return `${fromDateStr(s).getMonth() + 1}/${fromDateStr(s).getDate()}`
}

/** unix 초 → '9월 25일'. 해가 다르면 연도까지 붙인다. */
export function fmtStamp(unix: number) {
  const d = new Date(unix * 1000)
  const y = d.getFullYear() === new Date().getFullYear() ? '' : `${d.getFullYear()}년 `
  return `${y}${d.getMonth() + 1}월 ${d.getDate()}일`
}

/** 태어난 날부터의 일수(태어난 날 = D+0, 이유식 저장 기준). */
export function daysSince(birth: string, from = today()) {
  const a = fromDateStr(birth).getTime()
  const b = fromDateStr(from).getTime()
  return Math.round((b - a) / 86_400_000)
}

/** 만 나이. */
export function ageOf(birth: string, from = today()) {
  const b = fromDateStr(birth)
  const f = fromDateStr(from)
  let age = f.getFullYear() - b.getFullYear()
  const md = (d: Date) => (d.getMonth() + 1) * 100 + d.getDate()
  if (md(f) < md(b)) age--
  return age
}

/**
 * 생후 며칠째 — 태어난 날이 1일(백일 세는 방식). 저장은 D+0 이라 표시할 때만 +1 한다.
 * 두 기준이 섞이지 않게 반드시 이 함수를 거친다.
 */
export function lifeDay(dday: number) {
  return `생후 ${dday + 1}일`
}

export function lifeDayOf(birth: string, from = today()) {
  return lifeDay(daysSince(birth, from))
}

/** 원 단위, 쉼표. 12,345,678원 */
export function fmtWon(n: number) {
  return `${Math.round(n).toLocaleString('ko-KR')}원`
}
/** 짧게: 1.2억, 345만 */
export function fmtWonShort(n: number) {
  const a = Math.abs(n), sign = n < 0 ? '-' : ''
  if (a >= 1e8) return `${sign}${(a / 1e8).toFixed(a >= 1e9 ? 0 : 1)}억`
  if (a >= 1e4) return `${sign}${Math.round(a / 1e4).toLocaleString('ko-KR')}만`
  return `${sign}${Math.round(a).toLocaleString('ko-KR')}`
}
