import { useLayoutEffect, useRef } from 'react'
import { Outlet, useLocation, useNavigate } from 'react-router'
import DiaryViewer from './DiaryViewer'

// 스크롤은 <main> 이 하므로 브라우저 복원이 안 닿는다. 경로별로 직접 기억한다.
const scrollMemory = new Map<string, number>()

export default function Shell() {
  const main = useRef<HTMLElement>(null)
  const loc = useLocation()
  // ?view=(읽기 창)는 같은 화면 위라 스크롤 키에서 뺀다.
  const sp = new URLSearchParams(loc.search)
  sp.delete('view')
  const key = loc.pathname + (sp.toString() ? '?' + sp : '')

  // 스크롤할 때 적는다. cleanup 에서 읽으면 이미 다음 화면으로 바뀌어 0 이다.
  useLayoutEffect(() => {
    const el = main.current
    if (!el) return
    const onScroll = () => scrollMemory.set(key, el.scrollTop)
    el.addEventListener('scroll', onScroll, { passive: true })
    return () => el.removeEventListener('scroll', onScroll)
  }, [key])

  useLayoutEffect(() => {
    const el = main.current
    if (!el) return
    const saved = scrollMemory.get(key) ?? 0
    if (saved === 0) { el.scrollTop = 0; return }
    // 목록이 늦게 그려질 수 있어 몇 번 더 맞춘다. 사용자가 만졌으면 그만.
    let touched = false
    const mark = () => { touched = true }
    el.addEventListener('touchstart', mark, { passive: true, once: true })
    const tryRestore = () => { if (!touched && Math.abs(el.scrollTop - saved) > 1) el.scrollTop = saved }
    tryRestore()
    const raf = requestAnimationFrame(tryRestore)
    const t1 = window.setTimeout(tryRestore, 80)
    const t2 = window.setTimeout(tryRestore, 250)
    return () => {
      el.removeEventListener('touchstart', mark)
      cancelAnimationFrame(raf); window.clearTimeout(t1); window.clearTimeout(t2)
    }
  }, [key])

  return (
    <div className="flex h-full flex-col">
      {/* 탭바가 fixed 라 그 높이 + 안전영역만큼 비운다. */}
      <main
        ref={main}
        className="flex-1 overflow-y-auto"
        style={{ paddingBottom: 'calc(4rem + env(safe-area-inset-bottom, 0px))' }}
      >
        <Outlet />
      </main>
      <TabBar />
      <DiaryViewer />
    </div>
  )
}

// 하단 바 두 단: 첫 단(홈·할 일·육아) → 묶음을 누르면 둘째 단(← + 묶음 화면들).
// 어느 단인지는 주소로 정한다.
interface Tab { to: string; label: string; icon: (p: IconProps) => React.ReactElement; match: string[]; end?: boolean }
interface Group { key: string; label: string; icon: Tab['icon']; tabs: Tab[] }

const groups: Group[] = [
  {
    key: 'tasks', label: '할 일', icon: BoardIcon,
    tabs: [
      { to: '/boards', label: '할 일', icon: BoardIcon, match: ['/boards', '/cards'] },
      { to: '/routines', label: '루틴', icon: RoutineIcon, match: ['/routines'] },
      { to: '/calendar', label: '캘린더', icon: CalendarIcon, match: ['/calendar'] },
    ],
  },
  {
    key: 'baby', label: '육아', icon: BabyIcon,
    tabs: [
      { to: '/care', label: '기록', icon: LogIcon, match: ['/care'] },
      { to: '/babyfood', label: '식단', icon: BabyfoodIcon, match: ['/babyfood'] },
      { to: '/diary', label: '다이어리', icon: DiaryIcon, match: ['/diary'] },
    ],
  },
  {
    key: 'money', label: '재정', icon: WalletIcon,
    tabs: [
      { to: '/finance', label: '재정', icon: WalletIcon, match: ['/finance'] },
      { to: '/finance/goals', label: '목표', icon: GoalIcon, match: ['/finance/goals'] },
    ],
  },
]

const matchLen = (t: Tab, path: string) =>
  Math.max(0, ...t.match.filter((m) => path === m || path.startsWith(m + '/')).map((m) => m.length))
// '/finance' 와 '/finance/goals' 처럼 겹치면 더 길게 맞는 탭 하나만 켠다.
const inTab = (t: Tab, path: string) => {
  const n = matchLen(t, path)
  if (n === 0) return false
  const g = groups.find((x) => x.tabs.includes(t))
  return !g || g.tabs.every((o) => o === t || matchLen(o, path) <= n)
}
const groupOf = (path: string) => groups.find((g) => g.tabs.some((t) => inTab(t, path)))

/** 하단 바. 둥근 흰 시트 + 옅은 그림자, 고른 칸은 아이콘을 채워 구분. */
function TabBar() {
  const { pathname } = useLocation()
  const nav = useNavigate()
  const group = groupOf(pathname)

  return (
    <nav
      className="fixed inset-x-0 bottom-0 z-20 overflow-hidden rounded-t-[20px] bg-surface shadow-[0_-2px_12px_rgba(15,23,42,0.07)]"
      // 홈 인디케이터(34px)는 뷰포트 안이라 바 배경으로 덮는다.
      style={{ paddingBottom: 'env(safe-area-inset-bottom, 0px)' }}
    >
      {/* 자르는 경계가 화면 전체면 넓은 화면에서 숨긴 단이 옆에 보인다. */}
      <div className="relative mx-auto h-[58px] max-w-lg overflow-hidden">
        <ul
          className="absolute inset-0 flex"
          style={{ opacity: group ? 0 : 1, visibility: group ? 'hidden' : 'visible', transition: 'opacity 180ms ease-out, visibility 180ms' }}
          aria-hidden={!!group}
        >
          <li className="flex-1">
            <BarButton label="홈" icon={HomeIcon} active={pathname === '/'} onClick={() => nav('/')} />
          </li>
          {groups.map((g) => (
            <li key={g.key} className="flex-1">
              <BarButton label={g.label} icon={g.icon} active={false} onClick={() => nav(g.tabs[0].to)} />
            </li>
          ))}
        </ul>

        {groups.map((g) => {
          const on = group?.key === g.key
          return (
            <div
              key={g.key}
              className="absolute inset-0 flex items-center gap-1 pl-3"
              style={{ visibility: on ? 'visible' : 'hidden', transition: 'visibility 300ms' }}
              aria-hidden={!on}
            >
              <button
                onClick={() => nav('/')}
                aria-label="처음으로"
                className="flex h-11 w-11 shrink-0 items-center justify-center rounded-full bg-surface-2 text-ink-2 active:bg-line"
                style={{ opacity: on ? 1 : 0, transition: 'opacity 200ms ease-out' }}
              >
                <svg viewBox="0 0 24 24" className="h-5 w-5" fill="none" stroke="currentColor" strokeWidth="2.2" strokeLinecap="round" strokeLinejoin="round"><path d="M19 12H5M11 18l-6-6 6-6" /></svg>
              </button>
              <ul
                className="flex flex-1"
                style={{
                  transform: on ? 'translateX(0)' : 'translateX(100%)',
                  opacity: on ? 1 : 0,
                  transition: 'transform 300ms ease-out, opacity 200ms ease-out',
                }}
              >
                {g.tabs.map((t) => (
                  <li key={t.to} className="flex-1">
                    <BarButton label={t.label} icon={t.icon} active={inTab(t, pathname)} onClick={() => nav(t.to)} />
                  </li>
                ))}
              </ul>
            </div>
          )
        })}
      </div>
    </nav>
  )
}

function BarButton({ label, icon: Icon, active, onClick }: {
  label: string
  icon: Tab['icon']
  active: boolean
  onClick: () => void
}) {
  return (
    <button onClick={onClick} className="flex w-full flex-col items-center gap-2 pb-1.5 pt-2.5 text-xs text-ink-2">
      <Icon filled={active} className="h-5 w-5" />
      <span className={active ? 'font-bold' : 'font-medium'}>{label}</span>
    </button>
  )
}

// 고른 칸은 같은 모양을 채워 그린다.
type IconProps = { className?: string; filled?: boolean }

function Svg({ className, filled, children }: IconProps & { children: React.ReactNode }) {
  return (
    <svg
      viewBox="0 0 24 24"
      fill={filled ? 'currentColor' : 'none'}
      stroke="currentColor"
      strokeWidth={filled ? 1.5 : 1.9}
      strokeLinecap="round"
      strokeLinejoin="round"
      className={className}
    >
      {children}
    </svg>
  )
}

function HomeIcon(p: IconProps) {
  return (
    <Svg {...p}>
      <path d="M3 11l9-8 9 8v9a1 1 0 0 1-1 1h-5v-6H9v6H4a1 1 0 0 1-1-1z" />
    </Svg>
  )
}
function BoardIcon(p: IconProps) {
  return (
    <Svg {...p}>
      <rect x="3" y="4" width="5" height="16" rx="1.4" />
      <rect x="10" y="4" width="5" height="11" rx="1.4" />
      <rect x="17" y="4" width="4" height="7" rx="1.4" />
    </Svg>
  )
}
function DiaryIcon({ filled, className }: IconProps) {
  return (
    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={filled ? 1.5 : 1.9}
         strokeLinecap="round" strokeLinejoin="round" className={className}>
      <path d="M4 5.5A1.5 1.5 0 0 1 5.5 4H10a2 2 0 0 1 2 2v13a2 2 0 0 0-2-2H5.5A1.5 1.5 0 0 1 4 15.5z"
            fill={filled ? 'currentColor' : 'none'} />
      <path d="M20 5.5A1.5 1.5 0 0 0 18.5 4H14a2 2 0 0 0-2 2v13a2 2 0 0 1 2-2h4.5a1.5 1.5 0 0 0 1.5-1.5z"
            fill={filled ? 'currentColor' : 'none'} />
    </svg>
  )
}

function RoutineIcon({ filled, className }: IconProps) {
  return (
    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={filled ? 1.6 : 1.9}
         strokeLinecap="round" strokeLinejoin="round" className={className}>
      <path d="M20 12a8 8 0 1 1-2.3-5.6" fill={filled ? 'currentColor' : 'none'} />
      <path d="M20 4v5h-5" />
      <path d="M8.5 12.2l2.4 2.4 4.6-4.6" stroke={filled ? 'var(--c-surface)' : 'currentColor'} strokeWidth="2.1" />
    </svg>
  )
}
function CalendarIcon({ filled, className }: IconProps) {
  // 통째로 채우면 칸이 사라져 가로선은 바탕색으로 남긴다.
  return (
    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={filled ? 1.5 : 1.9}
         strokeLinecap="round" strokeLinejoin="round" className={className}>
      <rect x="3" y="5" width="18" height="16" rx="2.5" fill={filled ? 'currentColor' : 'none'} />
      <path d="M3 10h18" stroke={filled ? 'var(--c-surface)' : 'currentColor'} />
      <path d="M8 3v4M16 3v4" />
    </svg>
  )
}
function BabyIcon({ filled, className }: IconProps) {
  return (
    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={filled ? 1.6 : 1.9}
         strokeLinecap="round" strokeLinejoin="round" className={className}>
      <circle cx="12" cy="13" r="8" fill={filled ? 'currentColor' : 'none'} />
      <path d="M12 5c-1.6-1.4-.2-3 1.3-2" />
      <g stroke={filled ? 'var(--c-surface)' : 'currentColor'}>
        <path d="M9.5 12h.01M14.5 12h.01" strokeWidth="2.6" />
        <path d="M9.5 15.5c1.4 1.2 3.6 1.2 5 0" />
      </g>
    </svg>
  )
}

function WalletIcon({ filled, className }: IconProps) {
  return (
    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={filled ? 1.6 : 1.9}
         strokeLinecap="round" strokeLinejoin="round" className={className}>
      <path d="M4 7a2 2 0 0 1 2-2h11v4" />
      <rect x="3" y="7" width="18" height="13" rx="2.5" fill={filled ? 'currentColor' : 'none'} />
      <path d="M16.5 13.5h.01" stroke={filled ? 'var(--c-surface)' : 'currentColor'} strokeWidth="3" />
    </svg>
  )
}
function GoalIcon({ filled, className }: IconProps) {
  return (
    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={filled ? 1.6 : 1.9}
         strokeLinecap="round" strokeLinejoin="round" className={className}>
      <circle cx="12" cy="12" r="9" fill={filled ? 'currentColor' : 'none'} />
      <circle cx="12" cy="12" r="5" stroke={filled ? 'var(--c-surface)' : 'currentColor'} />
      <circle cx="12" cy="12" r="1.2" fill={filled ? 'var(--c-surface)' : 'currentColor'} stroke="none" />
    </svg>
  )
}

function LogIcon({ filled, className }: IconProps) {
  return (
    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={filled ? 1.6 : 1.9}
         strokeLinecap="round" strokeLinejoin="round" className={className}>
      <circle cx="12" cy="12" r="9" fill={filled ? 'currentColor' : 'none'} />
      <path d="M12 7v5l3.5 2" stroke={filled ? 'var(--c-surface)' : 'currentColor'} strokeWidth="2.1" />
    </svg>
  )
}

function BabyfoodIcon({ filled, className }: IconProps) {
  return (
    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={filled ? 1.5 : 1.9}
         strokeLinecap="round" strokeLinejoin="round" className={className}>
      <path d="M3 11h12a6 6 0 0 1-6 6H9a6 6 0 0 1-6-6z" fill={filled ? 'currentColor' : 'none'} />
      <path d="M5 20h8" />
      <path d="M19 3c1.3 1.3 1.3 3.4 0 4.7L18 8.8V20" />
    </svg>
  )
}
