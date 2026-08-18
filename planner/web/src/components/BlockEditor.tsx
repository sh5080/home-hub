
import { useMemo } from 'react'
import {
  useCreateBlockNote,
  SuggestionMenuController,
  getDefaultReactSlashMenuItems,
  type DefaultReactSuggestionItem,
} from '@blocknote/react'
import { BlockNoteView } from '@blocknote/mantine'
import { BlockNoteSchema, defaultBlockSpecs, filterSuggestionItems, type PartialBlock } from '@blocknote/core'
import { ko } from '@blocknote/core/locales'
import { Callout } from './Callout'
import '@blocknote/core/fonts/inter.css'
import '@blocknote/mantine/style.css'
import './blockeditor.css'

// 기본 블록 + 직접 만든 콜아웃.
const schema = BlockNoteSchema.create({
  blockSpecs: { ...defaultBlockSpecs, callout: Callout() },
})

// 슬래시 메뉴를 한국어로 다시 이름 붙이고 묶는다. BlockNote 기본 항목은
// 영문 키워드만 쥐고 있어서 "제목"이나 "체크"로 검색이 안 된다.
const LABELS: Record<string, { title: string; group: string; aliases: string[] }> = {
  'Heading 1':        { title: '제목 1',      group: '제목',   aliases: ['h1', '큰제목', 'jemok'] },
  'Heading 2':        { title: '제목 2',      group: '제목',   aliases: ['h2', 'jemok'] },
  'Heading 3':        { title: '제목 3',      group: '제목',   aliases: ['h3', 'jemok'] },
  'Numbered List':    { title: '번호 목록',    group: '목록',   aliases: ['ol', '숫자', 'beonho'] },
  'Bullet List':      { title: '글머리 목록',  group: '목록',   aliases: ['ul', '점', 'geulmeori'] },
  'Check List':       { title: '체크리스트',   group: '목록',   aliases: ['todo', '할일', 'chekeu'] },
  'Toggle List':      { title: '토글 목록',    group: '목록',   aliases: ['접기', '펼치기', 'togeul'] },
  'Quote':            { title: '인용',        group: '기본',   aliases: ['blockquote', 'inyong'] },
  'Code Block':       { title: '코드',        group: '기본',   aliases: ['code', '코드블록'] },
  'Paragraph':        { title: '본문',        group: '기본',   aliases: ['text', '텍스트', 'bonmun'] },
  'Divider':          { title: '구분선',      group: '기본',   aliases: ['hr', '선', 'gubunseon'] },
  'Table':            { title: '표',          group: '기본',   aliases: ['table', '테이블', 'pyo'] },
  'Image':            { title: '이미지',      group: '미디어', aliases: ['image', '사진', 'imiji'] },
  'Video':            { title: '동영상',      group: '미디어', aliases: ['video', 'dongyeongsang'] },
  'Audio':            { title: '오디오',      group: '미디어', aliases: ['audio', '소리'] },
  'File':             { title: '파일',        group: '미디어', aliases: ['file', 'pail'] },
  'Emoji':            { title: '이모지',      group: '기본',   aliases: ['emoji', '이모티콘'] },
}

export default function BlockEditor({
  initial,
  onChange,
  editable = true,
}: {
  initial: string | null
  onChange?: (json: string) => void
  editable?: boolean
}) {
  const editor = useCreateBlockNote({
    schema,
    dictionary: ko,
    initialContent: parseInitial(initial),
    // 본문이 비었을 때 무엇을 할 수 있는지 알려준다 — 슬래시 메뉴는
    // 존재를 모르면 아무도 안 쓴다.
    placeholders: {
      emptyDocument: "입력하거나 '/' 를 눌러 블록 추가",
      default: "'/' 로 블록 추가",
    },
  })

  // 기본 항목을 한국어로 바꾸고 콜아웃을 끼워 넣는다.
  const items = useMemo<DefaultReactSuggestionItem[]>(() => {
    const base = getDefaultReactSlashMenuItems(editor).map((item) => {
      const l = LABELS[item.title]
      if (!l) return item
      return {
        ...item,
        title: l.title,
        group: l.group,
        aliases: [...(item.aliases ?? []), item.title.toLowerCase(), ...l.aliases],
      }
    })
    return [
      ...base,
      {
        title: '콜아웃',
        group: '기본',
        aliases: ['callout', '강조', '박스', 'kolaut'],
        subtext: '눈에 띄는 상자로 강조',
        icon: <span className="text-base">💡</span>,
        onItemClick: () => {
          const cur = editor.getTextCursorPosition().block
          editor.insertBlocks([{ type: 'callout' }], cur, 'after')
          const next = editor.getTextCursorPosition().nextBlock
          if (next) editor.setTextCursorPosition(next, 'end')
        },
      },
    ]
  }, [editor])

  // 노션처럼, 본문 아래 빈 곳을 누르면 단락이 하나 생긴다.
  // 마지막 블록이 이미 빈 단락이면 새로 만들지 않고 커서만 옮긴다 —
  // 여러 번 눌렀다고 빈 줄이 쌓이면 곤란하다.
  function appendParagraph() {
    const blocks = editor.document
    const last = blocks[blocks.length - 1]
    const isEmptyParagraph =
      last?.type === 'paragraph' && Array.isArray(last.content) && last.content.length === 0

    if (last && !isEmptyParagraph) {
      editor.insertBlocks([{ type: 'paragraph' }], last, 'after')
    }
    const target = editor.document[editor.document.length - 1]
    if (target) editor.setTextCursorPosition(target, 'end')
    editor.focus()
  }

  return (
    <div className="flex min-h-0 flex-col">
      <BlockNoteView
        editor={editor}
        editable={editable}
        theme="light"
        slashMenu={false} // 아래에서 한국어 메뉴로 대체
        onChange={() => onChange?.(JSON.stringify(editor.document))}
      >
        <SuggestionMenuController
          triggerCharacter="/"
          getItems={async (query) => filterSuggestionItems(items, query)}
        />
      </BlockNoteView>
      {/* 본문 아래 빈 곳 — 누르면 단락이 생긴다(노션과 같은 동작).
          cursor-text 로 "여기 쓸 수 있다"는 걸 알린다. */}
      {editable && <div onClick={appendParagraph} aria-hidden className="min-h-[35vh] flex-1 cursor-text" />}
    </div>
  )
}

function parseInitial(raw: string | null): PartialBlock[] | undefined {
  if (!raw) return undefined
  try {
    const parsed = JSON.parse(raw)
    if (Array.isArray(parsed) && parsed.length > 0) return parsed as PartialBlock[]
  } catch {
    /* 저장된 문서가 깨졌으면 빈 문서로 시작한다 — 편집을 막지는 않는다 */
  }
  return undefined
}

