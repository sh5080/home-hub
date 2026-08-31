-- 반복 일정.
--
-- 반복은 카드의 성질이지 별개의 개체가 아니라서 별도 테이블을 만들지 않는다.
-- 규칙은 문자열 하나다: daily | every:N | weekly:<마스크> | monthly:<일> |
-- yearly:MM-DD. 요일 마스크는 루틴(weekdays_mask)과 같은 인코딩(bit0=월)을
-- 쓴다 — 한 저장소에 요일 비트가 두 가지 있으면 언젠가 반드시 섞인다.
--
-- 완료했을 때 카드를 되돌리지 않고 **다음 회차를 새 카드로 만든다**. 완료한
-- 카드가 완료 칸에 그대로 남아야 '완료'가 늘 뜻하던 것을 계속 뜻한다.
-- recur_parent_id 는 그 연결이다.

ALTER TABLE cards ADD COLUMN recur           TEXT;
ALTER TABLE cards ADD COLUMN recur_until     TEXT;  -- 'YYYY-MM-DD'. 이 날을 넘으면 멈춘다
ALTER TABLE cards ADD COLUMN recur_parent_id INTEGER REFERENCES cards(id) ON DELETE SET NULL;

CREATE INDEX cards_recur ON cards(recur) WHERE recur IS NOT NULL;
