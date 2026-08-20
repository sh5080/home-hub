-- 0005_card_due_and_priority: 마감에 시각을, 중요도에 자리를.
--
-- due_date → due_at: 컬럼이 이제 날짜뿐 아니라 일시도 담는다
-- ('YYYY-MM-DD' 또는 'YYYY-MM-DDTHH:MM', 0001_init.sql의 시간 규칙 그대로).
-- 날짜 경계로 하는 범위 쿼리는 그대로 동작한다 — 사전식 비교에서
-- '2026-09-23T14:00' 은 '2026-09-23' 이상이고 '2026-09-24' 미만이다.
-- 이름을 바꾸는 건 date라는 이름이 일시를 담고 있으면 나중에 반드시 사고가
-- 나기 때문이다.
--
-- priority: 노션에서 가져온 중요도(⭐ 개수)를 본문 콜아웃 텍스트로만 들고
-- 있었다. 정렬 기준으로 쓰려면 컬럼이어야 한다. 0=없음, 1~3=⭐~⭐⭐⭐.
-- 기존 카드의 값은 Open()이 콜아웃에서 한 번 복구한다.

ALTER TABLE cards RENAME COLUMN due_date TO due_at;
ALTER TABLE cards ADD COLUMN priority INTEGER NOT NULL DEFAULT 0;

DROP INDEX IF EXISTS cards_due;
CREATE INDEX cards_due ON cards(due_at) WHERE due_at IS NOT NULL;
CREATE INDEX cards_priority ON cards(priority);
