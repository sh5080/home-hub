import { NavLink, Outlet } from 'react-router'

// 앱 껍데기: 상단 없이 내용 + 하단 탭. 폰 우선.
export default function Shell() {
  return (
    <div className="flex h-full flex-col">
      <main className="flex-1 overflow-y-auto pb-20">
        <Outlet />
      </main>
      <TabBar />
    </div>
  )
}

const tabs = [
  { to: '/', label: '홈', icon: HomeIcon, end: true },
  { to: '/boards', label: '할 일', icon: BoardIcon },
  { to: '/calendar', label: '캘린더', icon: CalendarIcon },
  { to: '/routines', label: '루틴', icon: RoutineIcon },
  { to: '/babyfood', label: '이유식', icon: BabyfoodIcon },
]

function TabBar() {
  return (
    <nav
      className="fixed inset-x-0 bottom-0 z-20 border-t border-slate-200 bg-white/95 backdrop-blur"
      style={{ paddingBottom: 'env(safe-area-inset-bottom)' }}
    >
      <ul className="mx-auto flex max-w-lg">
        {tabs.map((t) => (
          <li key={t.to} className="flex-1">
            <NavLink
              to={t.to}
              end={t.end}
              className={({ isActive }) =>
                `flex flex-col items-center gap-0.5 py-2 text-[11px] font-medium transition-colors ${
                  isActive ? 'text-slate-900' : 'text-slate-400 hover:text-slate-600'
                }`
              }
            >
              {({ isActive }) => (
                <>
                  <t.icon className={`h-6 w-6 ${isActive ? 'stroke-[2.2]' : 'stroke-[1.6]'}`} />
                  {t.label}
                </>
              )}
            </NavLink>
          </li>
        ))}
      </ul>
    </nav>
  )
}

// 아이콘 라이브러리 없이 인라인 SVG (번들 최소화)
type IconProps = { className?: string }
function HomeIcon({ className }: IconProps) {
  return (
    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeLinecap="round" strokeLinejoin="round" className={className}>
      <path d="M3 11l9-8 9 8v9a1 1 0 0 1-1 1h-5v-6H9v6H4a1 1 0 0 1-1-1z" />
    </svg>
  )
}
function BoardIcon({ className }: IconProps) {
  return (
    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeLinecap="round" strokeLinejoin="round" className={className}>
      <rect x="3" y="4" width="5" height="16" rx="1" />
      <rect x="10" y="4" width="5" height="11" rx="1" />
      <rect x="17" y="4" width="4" height="7" rx="1" />
    </svg>
  )
}
function CalendarIcon({ className }: IconProps) {
  return (
    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeLinecap="round" strokeLinejoin="round" className={className}>
      <rect x="3" y="5" width="18" height="16" rx="2" />
      <path d="M3 10h18M8 3v4M16 3v4" />
    </svg>
  )
}
// 그릇과 숟가락
function BabyfoodIcon({ className }: IconProps) {
  return (
    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeLinecap="round" strokeLinejoin="round" className={className}>
      <path d="M3 11h12a6 6 0 0 1-6 6H9a6 6 0 0 1-6-6z" />
      <path d="M5 20h8" />
      <path d="M19 3c1.3 1.3 1.3 3.4 0 4.7L18 8.8V20" />
    </svg>
  )
}
function RoutineIcon({ className }: IconProps) {
  return (
    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeLinecap="round" strokeLinejoin="round" className={className}>
      <path d="M20 12a8 8 0 1 1-2.3-5.7" />
      <path d="M20 4v5h-5" />
      <path d="M9 12l2 2 4-4" />
    </svg>
  )
}
