-- 반응·좋아함을 '재료'가 아니라 '그날의 기록'으로 옮긴다.
--
-- 0008에서 날짜(bf_days.allergy)를 버리고 재료에 달았다. 그때의 이유는
-- "당근은 괜찮았나"를 한눈에 보려는 것이었고, 그건 지금도 맞다. 틀린 건
-- 기록하는 쪽이다. 재료에 불리언 하나면 남길 수 있는 말이 "괜찮다/아니다"
-- 뿐이라, 표시를 달 수 있는 자리도 그날 처음 먹는 재료 하나로 좁아졌다.
-- 실제로는 어제 잘 먹던 걸 오늘 뱉기도 하고, 세 번째 먹은 날 발진이 오기도
-- 한다.
--
-- 그래서 기록은 (아이, 날짜, 재료)마다 남기고, "당근은 괜찮았나"는 그 기록을
-- 모아서 답한다. 재료 쪽 요약 칸은 두 번째 진실이 되므로 지운다.

CREATE TABLE bf_logs (
  id        INTEGER PRIMARY KEY,
  child_id  INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  dday      INTEGER NOT NULL,
  name      TEXT    NOT NULL,
  reaction  INTEGER NOT NULL DEFAULT 0,
  liked     INTEGER NOT NULL DEFAULT 0,
  logged_at INTEGER NOT NULL,
  logged_by INTEGER REFERENCES users(id) ON DELETE SET NULL,
  UNIQUE (child_id, dday, name)
);
CREATE INDEX bf_logs_name ON bf_logs(child_id, name);

-- 지금까지의 표시를 그 재료를 **처음 먹은 날**의 기록으로 옮긴다. 언제
-- 눌렀는지(tag_at)는 남아 있지만 어느 날 먹고 눌렀는지는 애초에 없었다.
-- 처음 먹은 날이 가장 그럴듯한 추정이고, 표시를 달던 자리가 거기였다.
INSERT INTO bf_logs (child_id, dday, name, reaction, liked, logged_at, logged_by)
  SELECT i.child_id, f.first_dday, i.name, i.reaction, i.liked,
         COALESCE(i.tag_at, strftime('%s', 'now')), i.tag_by
    FROM bf_ingredients i
    JOIN (SELECT d.child_id AS child_id, x.name AS name, min(d.dday) AS first_dday
            FROM bf_meal_items x
            JOIN bf_meals m ON m.id = x.meal_id
            JOIN bf_days  d ON d.id = m.day_id
           GROUP BY d.child_id, x.name) f
      ON f.child_id = i.child_id AND f.name = i.name
   WHERE i.reaction = 1 OR i.liked = 1;

-- 재료 표는 이제 재고만 든다. DROP COLUMN 은 인덱스·외래키가 걸린 칸을
-- 거부하므로 다시 만든다. 이 표를 참조하는 자식 표는 없다(bf_batches 는
-- 이름으로만 잇는다) — 0011 에서 겪은 CASCADE 사고가 여기선 없다.
CREATE TABLE bf_ingredients_new (
  id             INTEGER PRIMARY KEY,
  child_id       INTEGER REFERENCES users(id) ON DELETE CASCADE,
  name           TEXT NOT NULL,
  kind           TEXT NOT NULL DEFAULT 'cube',
  count_qty      INTEGER,
  count_dday     INTEGER,
  count_at       INTEGER,
  count_batch_id INTEGER NOT NULL DEFAULT 0,
  count_by       INTEGER REFERENCES users(id) ON DELETE SET NULL,
  UNIQUE (child_id, name)
);
INSERT INTO bf_ingredients_new
  (id, child_id, name, kind, count_qty, count_dday, count_at, count_batch_id, count_by)
  SELECT id, child_id, name, kind, count_qty, count_dday, count_at, count_batch_id, count_by
    FROM bf_ingredients;

DROP TABLE bf_ingredients;
ALTER TABLE bf_ingredients_new RENAME TO bf_ingredients;
