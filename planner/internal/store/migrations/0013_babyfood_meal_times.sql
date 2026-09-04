-- 끼니를 '아침/점심'이 아니라 '몇 시에 먹이는지'로 다룬다.
--
-- 지금까지 끼니 이름은 식단표에서 그대로 온 값이었다(pos 0=아침, 1=점심,
-- 2=저녁). 그런데 하루 한 끼를 먹는 아이에게 그 한 끼는 아침이 아니라
-- 점심이다 — 이름이 자리 번호를 따라오지 않아서 어긋났다.
--
-- 그래서 이름은 '하루 몇 끼인지 + 몇 번째인지'로 정하고(1끼=점심,
-- 2끼=점심·저녁, 3끼=아침·점심·저녁), 시각은 아이마다 기본값을 두고 끼니마다
-- 덮어쓸 수 있게 한다.

-- NULL이면 아래 기본값을 따른다. 되돌리기는 이 칸을 다시 비우는 것이고,
-- 그래서 'at_src' 같은 원본 칸이 따로 필요 없다 — 기본값이 곧 원본이다.
ALTER TABLE bf_meals ADD COLUMN at TEXT;   -- 'HH:MM'

-- 하루 몇 끼일 때 몇 시에 먹이는지. 끼니 수마다 따로 둔다 — 두 끼에서 세
-- 끼로 넘어갈 때 시간표 자체가 바뀌기 때문이다.
CREATE TABLE bf_meal_times (
  child_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  n        INTEGER NOT NULL,   -- 하루 끼니 수
  pos      INTEGER NOT NULL,   -- 그중 몇 번째 (0부터)
  at       TEXT    NOT NULL,   -- 'HH:MM'
  PRIMARY KEY (child_id, n, pos)
);

INSERT INTO bf_meal_times (child_id, n, pos, at)
  SELECT c.user_id, v.n, v.pos, v.at
    FROM bf_children c,
         (            SELECT 1 AS n, 0 AS pos, '12:00' AS at
          UNION ALL   SELECT 2, 0, '12:00'
          UNION ALL   SELECT 2, 1, '18:00'
          UNION ALL   SELECT 3, 0, '09:00'
          UNION ALL   SELECT 3, 1, '12:00'
          UNION ALL   SELECT 3, 2, '18:00') v;
