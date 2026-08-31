CREATE TABLE bf_batches (
  id        INTEGER PRIMARY KEY,
  name      TEXT    NOT NULL REFERENCES bf_ingredients(name) ON DELETE CASCADE,
  qty       INTEGER NOT NULL,
  made_dday INTEGER NOT NULL,
  made_at   INTEGER NOT NULL,
  made_by   INTEGER REFERENCES users(id) ON DELETE SET NULL,
  note      TEXT    NOT NULL DEFAULT ''
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
  note         TEXT NOT NULL DEFAULT ''
);

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
, reaction INTEGER NOT NULL DEFAULT 0, liked    INTEGER NOT NULL DEFAULT 0, tag_at   INTEGER, tag_by   INTEGER REFERENCES users(id) ON DELETE SET NULL);

CREATE TABLE bf_meal_items (
  meal_id INTEGER NOT NULL REFERENCES bf_meals(id) ON DELETE CASCADE,
  role    TEXT    NOT NULL,                   -- 'base' | 'topping' | 'snack'
  pos     INTEGER NOT NULL,
  name    TEXT    NOT NULL,
  PRIMARY KEY (meal_id, role, pos)
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

CREATE TABLE bf_profile (
  id           INTEGER PRIMARY KEY CHECK (id = 1),
  name         TEXT    NOT NULL DEFAULT '',
  birth_date   TEXT,                          -- 'YYYY-MM-DD'. NULL이면 아직 설정 전
  horizon_days INTEGER NOT NULL DEFAULT 21,   -- 총 필요를 며칠치로 계산할지
  updated_at   INTEGER NOT NULL
);

CREATE TABLE boards (
  id         INTEGER PRIMARY KEY,
  name       TEXT    NOT NULL,
  created_by INTEGER REFERENCES users(id) ON DELETE SET NULL,  -- 시드 보드는 사용자보다 먼저 생긴다
  created_at INTEGER NOT NULL
);

CREATE TABLE cards (
  id          INTEGER PRIMARY KEY,
  column_id   INTEGER NOT NULL REFERENCES columns(id) ON DELETE CASCADE,
  title       TEXT    NOT NULL,
  description TEXT    NOT NULL DEFAULT '',
  position    INTEGER NOT NULL,
  due_at    TEXT,                                           -- 'YYYY-MM-DD' | NULL
  assignee_id INTEGER REFERENCES users(id) ON DELETE SET NULL,
  created_by  INTEGER NOT NULL REFERENCES users(id),
  created_at  INTEGER NOT NULL,
  updated_at  INTEGER NOT NULL
, content TEXT, priority INTEGER NOT NULL DEFAULT 0, end_at TEXT, recur           TEXT, recur_until     TEXT, recur_parent_id INTEGER REFERENCES cards(id) ON DELETE SET NULL);

CREATE TABLE columns (
  id       INTEGER PRIMARY KEY,
  board_id INTEGER NOT NULL REFERENCES boards(id) ON DELETE CASCADE,
  name     TEXT    NOT NULL,
  position INTEGER NOT NULL
);

CREATE TABLE login_attempts (
  key          TEXT    PRIMARY KEY,
  fails        INTEGER NOT NULL DEFAULT 0,  -- 연속 실패 수 (성공하면 행 삭제)
  window_start INTEGER NOT NULL,            -- unix 초, IP 레이트 윈도의 시작
  window_count INTEGER NOT NULL DEFAULT 0,  -- 그 윈도 안의 시도 수
  locked_until INTEGER NOT NULL DEFAULT 0   -- unix 초, 0이면 잠기지 않음
);

CREATE TABLE routine_checks (
  routine_id INTEGER NOT NULL REFERENCES routines(id) ON DELETE CASCADE,
  date       TEXT    NOT NULL,     -- 'YYYY-MM-DD'
  checked_by INTEGER NOT NULL REFERENCES users(id),
  checked_at INTEGER NOT NULL,
  PRIMARY KEY (routine_id, date)
);

CREATE TABLE routines (
  id            INTEGER PRIMARY KEY,
  title         TEXT    NOT NULL,
  weekdays_mask INTEGER NOT NULL,  -- bit0=월 … bit6=일 (ISO)
  time_of_day   TEXT,              -- 'HH:MM' 표시용
  assignee_id   INTEGER REFERENCES users(id) ON DELETE SET NULL,
  active        INTEGER NOT NULL DEFAULT 1,
  position      INTEGER NOT NULL,
  created_at    INTEGER NOT NULL
);

CREATE TABLE sessions (
  token      TEXT    PRIMARY KEY,
  user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  expires_at INTEGER NOT NULL
);

CREATE TABLE users (
  id            INTEGER PRIMARY KEY,
  name          TEXT    NOT NULL UNIQUE,
  password_hash TEXT    NOT NULL,
  created_at    INTEGER NOT NULL
);

CREATE INDEX bf_batches_name ON bf_batches(name, made_at);

CREATE INDEX bf_ingredients_tagged ON bf_ingredients(reaction, liked);

CREATE INDEX bf_meal_items_name ON bf_meal_items(name);

CREATE INDEX cards_column ON cards(column_id, position);

CREATE INDEX cards_due ON cards(due_at) WHERE due_at IS NOT NULL;

CREATE INDEX cards_end ON cards(end_at) WHERE end_at IS NOT NULL;

CREATE INDEX cards_priority ON cards(priority);

CREATE INDEX cards_recur ON cards(recur) WHERE recur IS NOT NULL;

CREATE INDEX columns_board ON columns(board_id, position);

CREATE INDEX login_attempts_locked ON login_attempts(locked_until);

CREATE INDEX routine_checks_date ON routine_checks(date);

CREATE INDEX sessions_expires ON sessions(expires_at);

