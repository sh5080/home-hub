-- 0006_unify_events_into_cards: 일정과 할 일을 한 테이블로.
--
-- 칸반과 캘린더는 같은 데이터를 다르게 보여주는 뷰여야 한다. 그런데 노션에서
-- 가져올 때 '선택' 필드를 보고 events와 cards로 갈라놓아, 캘린더에서 만든 것과
-- 보드에서 만든 것이 서로 보이지 않았다.
--
-- 통합 후 규칙:
--   칸반  = 모든 카드를 컬럼별로 묶어 본다
--   캘린더 = due_at 이 있는 카드를 날짜로 본다
-- 한 테이블, 두 필터. column_id 를 nullable 로 두는 안은 택하지 않았다 —
-- 그건 이름만 바꾼 events 다.
--
-- end_at 을 더한다. 여러 날에 걸치는 항목(여행 등)을 담는다. due_at 과 같은
-- 규칙: 'YYYY-MM-DD' 또는 'YYYY-MM-DDTHH:MM'. 종일 여부는 따로 저장하지 않는다 —
-- 문자열 길이가 곧 '시각이 있는가'라서 컬럼을 두면 진실이 둘이 된다.
--
-- events 행 이관은 Go(backfillEvents)에서 한다. 컬럼별 position 을 조밀하게
-- 이어붙여야 하는데 SQL로 하면 읽기 어렵고, 기준 날짜(오늘)를 마이그레이션에
-- 박으면 결정적이지 않다.

ALTER TABLE cards ADD COLUMN end_at TEXT;
CREATE INDEX cards_end ON cards(end_at) WHERE end_at IS NOT NULL;
