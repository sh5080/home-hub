
import { useEffect, useMemo } from 'react'
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
import { useResolvedTheme } from '../lib/theme'
import { uploadMedia } from '../api'
import '@blocknote/core/fonts/inter.css'
import '@blocknote/mantine/style.css'
import './blockeditor.css'

const schema = BlockNoteSchema.create({
  blockSpecs: { ...defaultBlockSpecs, callout: Callout() },
})

// 슬래시 메뉴를 한국어로(기본 항목은 영문 키워드만 있다).
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

// 스키마가 박힌 편집기 타입.
export type Editor = typeof schema.BlockNoteEditor

export default function BlockEditor({
  initial,
  onChange,
  onReady,
  editable = true,
}: {
  initial: string | null
  onChange?: (json: string) => void
  /** 바깥에서 블록을 끼워 넣을 수 있게 편집기를 넘긴다. */
  onReady?: (editor: Editor) => void
  editable?: boolean
}) {
  // 편집기 색 체계는 prop 으로 넘겨야 한다(<html class="dark"> 만으로는 글자가 묻힌다).
  const theme = useResolvedTheme()

  const editor = useCreateBlockNote({
    schema,
    dictionary: ko,
    initialContent: parseInitial(initial),
    placeholders: {
      emptyDocument: "입력하거나 '/' 를 눌러 블록 추가",
      default: "'/' 로 블록 추가",
    },
    uploadFile: async (file: File) => (await uploadMedia(file)).url,
  })

  useEffect(() => { onReady?.(editor) }, [editor, onReady])

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

  // 본문 아래 빈 곳을 누르면 단락이 생긴다. 마지막이 빈 단락이면 커서만 옮긴다.
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
        theme={theme}
        slashMenu={false} // 아래에서 한국어 메뉴로 대체
        onChange={() => onChange?.(JSON.stringify(editor.document))}
      >
        <SuggestionMenuController
          triggerCharacter="/"
          getItems={async (query) => filterSuggestionItems(items, query)}
        />
      </BlockNoteView>
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

