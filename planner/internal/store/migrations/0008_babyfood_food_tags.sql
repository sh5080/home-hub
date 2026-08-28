-- 알레르기 반응과 좋아함을 **재료 단위**로 옮긴다.
--
-- 0007에서는 날짜에 달아뒀는데(bf_days.allergy), 알고 싶은 건 "9월 21일이
-- 어땠나"가 아니라 "당근은 괜찮았나, 잘 먹나"다. 같은 재료가 194일에 걸쳐
-- 수십 번 나오므로 날짜에 달면 한눈에 모이지 않는다.
--
-- 둘은 서로 독립이다. 반응이 있으면서 잘 먹을 수도 있고(그래도 빼야 한다),
-- 반응은 없는데 안 먹을 수도 있다.

ALTER TABLE bf_ingredients ADD COLUMN reaction INTEGER NOT NULL DEFAULT 0;
ALTER TABLE bf_ingredients ADD COLUMN liked    INTEGER NOT NULL DEFAULT 0;
ALTER TABLE bf_ingredients ADD COLUMN tag_at   INTEGER;
ALTER TABLE bf_ingredients ADD COLUMN tag_by   INTEGER REFERENCES users(id) ON DELETE SET NULL;

-- 날짜에 남겨둔 'o'(반응 있음)를 그날 처음 먹은 재료로 옮긴다.
-- 'x'(이상 없음)는 옮길 곳이 없다 — 새 모델에서 '반응 없음'은 표시가 없는
-- 상태 그 자체다.
UPDATE bf_ingredients SET reaction = 1
 WHERE name IN (SELECT new_item FROM bf_days WHERE allergy = 'o' AND new_item IS NOT NULL);

ALTER TABLE bf_days DROP COLUMN allergy;

CREATE INDEX bf_ingredients_tagged ON bf_ingredients(reaction, liked);
