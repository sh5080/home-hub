import { setTheme, useTheme, type Theme } from '../lib/theme'

const OPTIONS: { v: Theme; label: string }[] = [
  { v: 'system', label: '시스템' },
  { v: 'light', label: '밝게' },
  { v: 'dark', label: '어둡게' },
]

export default function ThemeToggle() {
  const theme = useTheme()

  return (
    <div>
      <div className="flex gap-1 rounded-xl bg-surface-2 p-1">
        {OPTIONS.map((o) => (
          <button
            key={o.v}
            onClick={() => setTheme(o.v)}
            aria-pressed={theme === o.v}
            className={`flex-1 rounded-lg py-2 text-sm font-medium transition ${
              theme === o.v ? 'bg-surface text-ink shadow-sm' : 'text-muted'
            }`}
          >
            {o.label}
          </button>
        ))}
      </div>
      <p className="mt-1.5 text-[11px] text-faint">
        시스템을 고르면 아이폰의 다크 모드 설정을 따라가요.
      </p>
    </div>
  )
}
