-- 0003_card_content: 카드 본문을 블록 문서로.
--
-- description(평문)만으로는 체크리스트·헤딩·토글 같은 걸 담을 수 없다. 본문을
-- 블록 트리 JSON으로 옮기되, 두 컬럼을 모두 유지한다:
--
--   content     — 권위 있는 본문. 블록 문서 JSON (ProseMirror 계열 모델).
--                 NULL이면 아직 블록으로 옮기지 않은 카드다.
--   description — content에서 파생한 평문. 백로그 미리보기와 나중의 검색이
--                 JSON을 파싱하지 않고 쓰기 위한 것이다. 서버가 쓸 때마다 갱신한다.
--
-- 백필은 Go에서 한다 — 문서 JSON의 정확한 모양을 SQL과 Go 두 곳에 두지 않는다.
-- 에디터 라이브러리가 바뀌어도 이 스키마는 그대로다: 어떤 에디터든 결과는
-- 블록 배열이고, 평문 폴백(textarea)도 단락 하나짜리 문서를 쓰면 된다.

ALTER TABLE cards ADD COLUMN content TEXT;
