import { createReactBlockSpec } from '@blocknote/react'

// 콜아웃 블록(BlockNote 기본엔 없다).
const TONES = {
  gray: 'bg-surface-2 border-line',
  blue: 'bg-sky-50 border-sky-200',
  green: 'bg-emerald-50 border-emerald-200',
  yellow: 'bg-amber-50 border-amber-200',
  red: 'bg-rose-50 border-rose-200',
} as const

export const Callout = createReactBlockSpec(
  {
    type: 'callout',
    propSchema: {
      emoji: { default: '💡' },
      tone: { default: 'yellow', values: ['gray', 'blue', 'green', 'yellow', 'red'] },
    },
    content: 'inline',
  },
  {
    render: ({ block, contentRef, editor }) => {
      const tone = (block.props.tone as keyof typeof TONES) ?? 'yellow'
      const cycle = () => {
        if (!editor.isEditable) return
        const keys = Object.keys(TONES) as (keyof typeof TONES)[]
        const next = keys[(keys.indexOf(tone) + 1) % keys.length]
        editor.updateBlock(block, { props: { tone: next } })
      }
      return (
        <div className={`my-1 flex gap-2 rounded-lg border px-3 py-2 ${TONES[tone]}`}>
          <button
            type="button"
            onClick={cycle}
            contentEditable={false}
            title="색 바꾸기"
            className="select-none text-lg leading-6"
          >
            {block.props.emoji as string}
          </button>
          <div ref={contentRef} className="min-w-0 flex-1 leading-6" />
        </div>
      )
    },
  },
)
