/**
 * 밝게/어둡게/시스템. <html class="dark"> 만 켜고 끈다.
 * BlockNote 는 색 체계를 prop 으로 받아야 해서 모드를 구독 가능한 모듈 상태로 둔다.
 */
import { useSyncExternalStore } from 'react'

export type Theme = 'system' | 'light' | 'dark'

const KEY = 'planner.theme'

function read(): Theme {
  try {
    const v = localStorage.getItem(KEY)
    if (v === 'light' || v === 'dark' || v === 'system') return v
  } catch {
    /* 사파리 비공개 모드에서 localStorage 가 막힐 수 있다 */
  }
  return 'system'
}

/** 실제로 어두운 화면인지 (system 이면 기기 설정을 본다) */
function darkNow(t: Theme) {
  if (t === 'system') return window.matchMedia('(prefers-color-scheme: dark)').matches
  return t === 'dark'
}

let theme: Theme = read()
let resolved: 'light' | 'dark' = darkNow(theme) ? 'dark' : 'light'
const listeners = new Set<() => void>()

function paint() {
  const dark = darkNow(theme)
  resolved = dark ? 'dark' : 'light'
  document.documentElement.classList.toggle('dark', dark)
  // iOS 상태표시줄·바운스 영역 색까지 맞춘다.
  document
    .querySelector('meta[name="theme-color"]')
    ?.setAttribute('content', dark ? '#020617' : '#f8fafc')
  for (const f of listeners) f()
}

/** 첫 렌더 전에 부른다(나중에 켜면 한 번 번쩍인다). */
export function startTheme() {
  paint()
  // 항상 듣는다 — system 이 아니면 paint 가 기기 설정을 무시한다.
  window.matchMedia('(prefers-color-scheme: dark)').addEventListener('change', paint)
}

export function setTheme(t: Theme) {
  theme = t
  try {
    localStorage.setItem(KEY, t)
  } catch {
    /* 저장 못 해도 이번 세션은 적용된다 */
  }
  paint()
}

function subscribe(f: () => void) {
  listeners.add(f)
  return () => listeners.delete(f)
}

/** 사용자가 고른 값 그대로 (system 포함) */
export function useTheme() {
  return useSyncExternalStore(subscribe, () => theme, () => theme)
}

/** 지금 밝은지 어두운지(prop 으로 색 체계를 받는 라이브러리용). */
export function useResolvedTheme() {
  return useSyncExternalStore(subscribe, () => resolved, () => 'light' as const)
}
