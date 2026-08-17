-- 0004_event_content: 일정에도 블록 본문을 준다.
--
-- 카드에는 content가 있는데 일정에는 제목·시각뿐이었다. 노션에서 가져온
-- 일정 10건의 '내용' 메모를 담을 곳이 없어서 드러난 빈 자리다. 카드와
-- 같은 구조를 쓴다: content가 권위 있는 블록 문서, description은 서버가
-- 파생하는 평문(목록 미리보기용).

ALTER TABLE events ADD COLUMN content TEXT;
ALTER TABLE events ADD COLUMN description TEXT NOT NULL DEFAULT '';
