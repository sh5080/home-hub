-- 이유식 식단표와 큐브 재고.
--
-- 식단은 날짜가 아니라 **D+n(생후 일수)** 으로 저장한다. 생일을 잘못 넣었다가
-- 고치면 194일치가 통째로 밀려야 하는데, 날짜로 박아두면 전부 다시 써야 한다.
-- 날짜 변환은 조회할 때 profile.birth_date 로 한다.
--
-- 식단 내용 자체는 이 파일에 없다. `planner babyfood import` 로 넣는다.

CREATE TABLE bf_profile (
  id           INTEGER PRIMARY KEY CHECK (id = 1),
  name         TEXT    NOT NULL DEFAULT '',
  birth_date   TEXT,                          -- 'YYYY-MM-DD'. NULL이면 아직 설정 전
  horizon_days INTEGER NOT NULL DEFAULT 21,   -- 총 필요를 며칠치로 계산할지
  updated_at   INTEGER NOT NULL
);

CREATE TABLE bf_days (
  dday         INTEGER PRIMARY KEY,
  stage        TEXT NOT NULL,                 -- 구간 id
  label        TEXT NOT NULL,                 -- 사람이 읽는 구간 이름
  -- 'topping' = 베이스+토핑 큐브로 차리는 구간(재고 계산 대상),
  -- 'menu'    = 요리 이름으로 적힌 구간. 큐브가 아니므로 재고에서 뺀다.
  kind         TEXT NOT NULL,
  new_item     TEXT,                          -- 그날 처음 먹이는 재료
  new_item_src TEXT,                          -- 시드 원본 (수정 표시용)
  allergy      TEXT,                          -- NULL(미확인) | 'o' | 'x'
  note         TEXT NOT NULL DEFAULT ''
);

CREATE TABLE bf_meals (
  id        INTEGER PRIMARY KEY,
  dday      INTEGER NOT NULL REFERENCES bf_days(dday) ON DELETE CASCADE,
  slot      TEXT    NOT NULL,                 -- '아침' | '점심' | '저녁'
  pos       INTEGER NOT NULL,
  -- 시드 당시의 {base, toppings, snack}. 한 번 쓰고 다시 안 건드린다.
  -- "수정됨 · 원래 소고기/청경채" 를 보여주려면 원본이 남아 있어야 한다.
  src       TEXT    NOT NULL,
  eaten_g   INTEGER,                          -- 먹은 양
  served_g  INTEGER,                          -- 차린 양
  -- 안 먹인 끼니. 재고는 '지난 끼니만큼 자동 차감'이라, 건너뛴 끼니까지
  -- 빼면 실제보다 적게 나온다.
  skipped   INTEGER NOT NULL DEFAULT 0,
  edited_by INTEGER REFERENCES users(id) ON DELETE SET NULL,
  edited_at INTEGER,
  UNIQUE (dday, slot)
);

-- 끼니에 올라가는 것들을 행으로 쪼갠다. JSON 배열로 두면 재료별 집계를
-- SQL로 못 한다 — 재고 계산이 전부 이 집계다.
CREATE TABLE bf_meal_items (
  meal_id INTEGER NOT NULL REFERENCES bf_meals(id) ON DELETE CASCADE,
  role    TEXT    NOT NULL,                   -- 'base' | 'topping' | 'snack'
  pos     INTEGER NOT NULL,
  name    TEXT    NOT NULL,
  PRIMARY KEY (meal_id, role, pos)
);
CREATE INDEX bf_meal_items_name ON bf_meal_items(name);

-- 재고는 숫자 하나로 들고 있지 않는다. 그렇게 하면 지금 종이가 그렇듯 조용히
-- 틀어진다. 대신 **마지막 실사값**만 저장하고, 그 뒤의 소모(지난 끼니)와
-- 제조(bf_batches)를 더해서 계산한다. 어긋나면 다시 세서 실사만 갱신하면 된다.
CREATE TABLE bf_ingredients (
  name       TEXT PRIMARY KEY,
  kind       TEXT NOT NULL DEFAULT 'cube',    -- 'base' | 'cube' | 'dish'
  count_qty  INTEGER,                         -- 마지막 실사 수량. NULL이면 아직 안 셈
  count_dday INTEGER,                         -- 실사 시점(D+n). 이 날까지는 실사값에 반영됨
  count_at   INTEGER,                         -- 보여주기용. 계산에는 쓰지 않는다
  -- 실사 당시 가장 최근 제조 기록의 id. "실사 이후 만든 것"을 시각으로
  -- 가르면 같은 초에 찍힌 기록이 통째로 빠진다. id는 단조 증가라 안 샌다.
  count_batch_id INTEGER NOT NULL DEFAULT 0,
  count_by   INTEGER REFERENCES users(id) ON DELETE SET NULL
);

CREATE TABLE bf_batches (
  id        INTEGER PRIMARY KEY,
  name      TEXT    NOT NULL REFERENCES bf_ingredients(name) ON DELETE CASCADE,
  qty       INTEGER NOT NULL,
  made_dday INTEGER NOT NULL,
  made_at   INTEGER NOT NULL,
  made_by   INTEGER REFERENCES users(id) ON DELETE SET NULL,
  note      TEXT    NOT NULL DEFAULT ''
);
CREATE INDEX bf_batches_name ON bf_batches(name, made_at);
