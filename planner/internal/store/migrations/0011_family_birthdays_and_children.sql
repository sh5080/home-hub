-- 생년월일을 가족에게, 이유식을 아이별로.
--
-- 지금까지 아이 정보는 bf_profile 한 행에만 있었다. 둘째가 생기면 식단도
-- 재고도 알레르기 반응도 섞인다 — 당근에 반응한 게 첫째인지 둘째인지 구분할
-- 방법이 없다. 그래서 이유식 자료 전체에 '누구의 것인지'를 붙인다.
--
-- child_id 는 NULL 을 허용한다. 이 마이그레이션이 도는 시점에는 아이가 아직
-- 가족으로 등록되기 전일 수 있고, 없는 사람을 지어내서 붙일 수는 없다.
-- 주인 없는 자료는 조회에서 빠지고, `planner babyfood adopt <이름>` 으로
-- 한 번 연결한다.

ALTER TABLE users ADD COLUMN birth_date TEXT;   -- 'YYYY-MM-DD'. 어른도 포함

-- 이유식을 진행하는 아이. 가족 중에서 고른다.
CREATE TABLE bf_children (
  user_id      INTEGER PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
  horizon_days INTEGER NOT NULL DEFAULT 21,   -- 재고를 며칠치로 계산할지
  created_at   INTEGER NOT NULL
);

-- --- bf_days: (child_id, dday) 로 식별한다 ---
CREATE TABLE bf_days_new (
  id           INTEGER PRIMARY KEY,
  child_id     INTEGER REFERENCES users(id) ON DELETE CASCADE,
  dday         INTEGER NOT NULL,
  stage        TEXT NOT NULL,
  label        TEXT NOT NULL,
  kind         TEXT NOT NULL,
  new_item     TEXT,
  new_item_src TEXT,
  note         TEXT NOT NULL DEFAULT '',
  UNIQUE (child_id, dday)
);
INSERT INTO bf_days_new (child_id, dday, stage, label, kind, new_item, new_item_src, note)
  SELECT NULL, dday, stage, label, kind, new_item, new_item_src, note FROM bf_days;

-- bf_meals 는 날짜가 아니라 '그 아이의 그 날'을 가리킨다. id 를 그대로 옮겨
-- bf_meal_items 의 참조를 살린다.
CREATE TABLE bf_meals_new (
  id        INTEGER PRIMARY KEY,
  day_id    INTEGER NOT NULL REFERENCES bf_days_new(id) ON DELETE CASCADE,
  slot      TEXT    NOT NULL,
  pos       INTEGER NOT NULL,
  src       TEXT    NOT NULL,
  eaten_g   INTEGER,
  served_g  INTEGER,
  skipped   INTEGER NOT NULL DEFAULT 0,
  edited_by INTEGER REFERENCES users(id) ON DELETE SET NULL,
  edited_at INTEGER,
  UNIQUE (day_id, slot)
);
INSERT INTO bf_meals_new (id, day_id, slot, pos, src, eaten_g, served_g, skipped, edited_by, edited_at)
  SELECT m.id, d.id, m.slot, m.pos, m.src, m.eaten_g, m.served_g, m.skipped, m.edited_by, m.edited_at
    FROM bf_meals m JOIN bf_days_new d ON d.dday = m.dday AND d.child_id IS NULL;

-- bf_meal_items 는 모양이 그대로지만 같이 다시 만든다. 옛 bf_meals 를 그냥
-- DROP 하면 ON DELETE CASCADE 를 타고 구성품이 전부 지워진다 — 실제로 복사본
-- 에서 2,130행이 0이 되는 걸 확인하고 고쳤다.
CREATE TABLE bf_meal_items_new (
  meal_id INTEGER NOT NULL REFERENCES bf_meals_new(id) ON DELETE CASCADE,
  role    TEXT    NOT NULL,
  pos     INTEGER NOT NULL,
  name    TEXT    NOT NULL,
  PRIMARY KEY (meal_id, role, pos)
);
INSERT INTO bf_meal_items_new (meal_id, role, pos, name)
  SELECT meal_id, role, pos, name FROM bf_meal_items;

-- --- bf_ingredients: 반응·좋아함·재고가 전부 아이별이다 ---
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
  reaction       INTEGER NOT NULL DEFAULT 0,
  liked          INTEGER NOT NULL DEFAULT 0,
  tag_at         INTEGER,
  tag_by         INTEGER REFERENCES users(id) ON DELETE SET NULL,
  UNIQUE (child_id, name)
);
INSERT INTO bf_ingredients_new
  (child_id, name, kind, count_qty, count_dday, count_at, count_batch_id, count_by, reaction, liked, tag_at, tag_by)
  SELECT NULL, name, kind, count_qty, count_dday, count_at, count_batch_id, count_by, reaction, liked, tag_at, tag_by
    FROM bf_ingredients;

CREATE TABLE bf_batches_new (
  id        INTEGER PRIMARY KEY,
  child_id  INTEGER REFERENCES users(id) ON DELETE CASCADE,
  name      TEXT    NOT NULL,
  qty       INTEGER NOT NULL,
  made_dday INTEGER NOT NULL,
  made_at   INTEGER NOT NULL,
  made_by   INTEGER REFERENCES users(id) ON DELETE SET NULL,
  note      TEXT    NOT NULL DEFAULT ''
);
INSERT INTO bf_batches_new (id, child_id, name, qty, made_dday, made_at, made_by, note)
  SELECT id, NULL, name, qty, made_dday, made_at, made_by, note FROM bf_batches;

-- 자식(참조하는 쪽)부터 지운다. 부모를 먼저 지우면 CASCADE 로 자식이 날아간다.
DROP TABLE bf_meal_items;
DROP TABLE bf_batches;
DROP TABLE bf_ingredients;
DROP TABLE bf_meals;
DROP TABLE bf_days;
DROP TABLE bf_profile;

ALTER TABLE bf_meal_items_new  RENAME TO bf_meal_items;
ALTER TABLE bf_days_new        RENAME TO bf_days;
ALTER TABLE bf_meals_new       RENAME TO bf_meals;
ALTER TABLE bf_ingredients_new RENAME TO bf_ingredients;
ALTER TABLE bf_batches_new     RENAME TO bf_batches;

CREATE INDEX bf_meal_items_name ON bf_meal_items(name);
CREATE INDEX bf_days_child      ON bf_days(child_id, dday);
CREATE INDEX bf_meals_day       ON bf_meals(day_id);
CREATE INDEX bf_ingredients_tag ON bf_ingredients(child_id, reaction, liked);
CREATE INDEX bf_batches_name    ON bf_batches(child_id, name, id);
