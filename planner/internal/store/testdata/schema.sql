CREATE TABLE "bf_batches" (
  id        INTEGER PRIMARY KEY,
  child_id  INTEGER REFERENCES users(id) ON DELETE CASCADE,
  name      TEXT    NOT NULL,
  qty       INTEGER NOT NULL,
  made_dday INTEGER NOT NULL,
  made_at   INTEGER NOT NULL,
  made_by   INTEGER REFERENCES users(id) ON DELETE SET NULL,
  note      TEXT    NOT NULL DEFAULT ''
);

CREATE TABLE bf_children (
  user_id      INTEGER PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
  horizon_days INTEGER NOT NULL DEFAULT 21,   -- 재고를 며칠치로 계산할지
  created_at   INTEGER NOT NULL
);

CREATE TABLE "bf_days" (
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

CREATE TABLE "bf_ingredients" (
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

CREATE TABLE bf_logs (
  id        INTEGER PRIMARY KEY,
  child_id  INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  dday      INTEGER NOT NULL,
  name      TEXT    NOT NULL,
  reaction  INTEGER NOT NULL DEFAULT 0,
  liked     INTEGER NOT NULL DEFAULT 0,
  logged_at INTEGER NOT NULL,
  logged_by INTEGER REFERENCES users(id) ON DELETE SET NULL, disliked INTEGER NOT NULL DEFAULT 0,
  UNIQUE (child_id, dday, name)
);

CREATE TABLE "bf_meal_items" (
  meal_id INTEGER NOT NULL REFERENCES "bf_meals"(id) ON DELETE CASCADE,
  role    TEXT    NOT NULL,
  pos     INTEGER NOT NULL,
  name    TEXT    NOT NULL,
  PRIMARY KEY (meal_id, role, pos)
);

CREATE TABLE bf_meal_times (
  child_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  n        INTEGER NOT NULL,   -- 하루 끼니 수
  pos      INTEGER NOT NULL,   -- 그중 몇 번째 (0부터)
  at       TEXT    NOT NULL,   -- 'HH:MM'
  PRIMARY KEY (child_id, n, pos)
);

CREATE TABLE "bf_meals" (
  id        INTEGER PRIMARY KEY,
  day_id    INTEGER NOT NULL REFERENCES "bf_days"(id) ON DELETE CASCADE,
  slot      TEXT    NOT NULL,
  pos       INTEGER NOT NULL,
  src       TEXT    NOT NULL,
  eaten_g   INTEGER,
  served_g  INTEGER,
  skipped   INTEGER NOT NULL DEFAULT 0,
  edited_by INTEGER REFERENCES users(id) ON DELETE SET NULL,
  edited_at INTEGER, at TEXT,
  UNIQUE (day_id, slot)
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
, content TEXT, priority INTEGER NOT NULL DEFAULT 0, end_at TEXT, recur           TEXT, recur_until     TEXT, recur_parent_id INTEGER REFERENCES cards(id) ON DELETE SET NULL, done_at INTEGER, archived_at INTEGER);

CREATE TABLE care_log_items (
  log_id        INTEGER NOT NULL REFERENCES care_logs(id) ON DELETE CASCADE,
  pos           INTEGER NOT NULL,
  ingredient_id INTEGER NOT NULL REFERENCES bf_ingredients(id) ON DELETE CASCADE,
  PRIMARY KEY (log_id, pos)
);

CREATE TABLE care_logs (
  id         INTEGER PRIMARY KEY,
  child_id   INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  kind       TEXT    NOT NULL,
  at         TEXT    NOT NULL,
  minutes    INTEGER,
  amount_ml  INTEGER,
  detail     TEXT,
  note       TEXT    NOT NULL DEFAULT '',
  created_by INTEGER REFERENCES users(id) ON DELETE SET NULL,
  created_at INTEGER NOT NULL
, source TEXT, meal_id INTEGER REFERENCES bf_meals(id) ON DELETE SET NULL);

CREATE TABLE columns (
  id       INTEGER PRIMARY KEY,
  board_id INTEGER NOT NULL REFERENCES boards(id) ON DELETE CASCADE,
  name     TEXT    NOT NULL,
  position INTEGER NOT NULL
);

CREATE TABLE diary (
  id         INTEGER PRIMARY KEY,
  date       TEXT    NOT NULL,              -- 'YYYY-MM-DD'
  title      TEXT    NOT NULL DEFAULT '',
  content    TEXT,                          -- 블록 편집기 JSON (카드와 같은 형식)
  plain      TEXT    NOT NULL DEFAULT '',   -- 본문에서 뽑은 평문. 목록·검색용
  created_by INTEGER REFERENCES users(id) ON DELETE SET NULL,
  created_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL
, photos TEXT NOT NULL DEFAULT '[]', source TEXT);

CREATE TABLE diary_refs (
  entry_id INTEGER NOT NULL REFERENCES diary(id) ON DELETE CASCADE,
  pos      INTEGER NOT NULL,
  kind     TEXT    NOT NULL,   -- 'card' | 'routine' | 'babyfood'
  ref_id   INTEGER,            -- 카드·루틴의 id
  ref_date TEXT,               -- 이유식은 날짜로 가리킨다
  child_id INTEGER,            -- 이유식일 때 누구의 식단인지
  label    TEXT    NOT NULL,
  PRIMARY KEY (entry_id, pos)
);

CREATE TABLE fin_fixed (
  key   TEXT    PRIMARY KEY,
  fixed INTEGER NOT NULL
);

CREATE TABLE fin_goals (
  id         INTEGER PRIMARY KEY,
  name       TEXT    NOT NULL,
  target     INTEGER NOT NULL,
  due        TEXT,                        -- 'YYYY-MM-DD' | NULL
  monthly    INTEGER NOT NULL DEFAULT 0,  -- 월 적립 계획. 월 가용 계산에서 뺀다
  items      TEXT    NOT NULL DEFAULT '[]', -- 채우는 자산 이름들(JSON). 최신 스냅샷에서 합한다
  created_by INTEGER REFERENCES users(id) ON DELETE SET NULL,
  created_at INTEGER NOT NULL
);

CREATE TABLE fin_income (
  key    TEXT PRIMARY KEY,
  income INTEGER NOT NULL
);

CREATE TABLE fin_items (
  id          INTEGER PRIMARY KEY,
  snapshot_id INTEGER NOT NULL REFERENCES fin_snapshots(id) ON DELETE CASCADE,
  group_      TEXT    NOT NULL,
  category    TEXT    NOT NULL,          -- 파일의 분류 이름 그대로(자유입출금 자산, 장기대출 …)
  institution TEXT    NOT NULL DEFAULT '',
  name        TEXT    NOT NULL,
  amount      INTEGER NOT NULL,          -- 원. 부채도 양수
  principal   INTEGER,                   -- 투자 원금 / 대출 원금
  rate        REAL                       -- 수익률 / 대출 금리(%)
);

CREATE TABLE fin_labels (
  key   TEXT PRIMARY KEY,
  label TEXT NOT NULL DEFAULT '',
  cat1  TEXT NOT NULL DEFAULT ''
);

CREATE TABLE fin_not_spend (
  key TEXT PRIMARY KEY
);

CREATE TABLE fin_snapshots (
  id          INTEGER PRIMARY KEY,
  owner_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  taken_at    TEXT    NOT NULL,          -- 'YYYY-MM-DD' (파일 기간의 끝날)
  total_asset INTEGER NOT NULL,          -- 항목 합(파일의 총자산 칸은 계산 안 된 수식이라 0 으로 온다)
  total_debt  INTEGER NOT NULL,
  created_by  INTEGER REFERENCES users(id) ON DELETE SET NULL,
  created_at  INTEGER NOT NULL,
  UNIQUE (owner_id, taken_at)
);

CREATE TABLE fin_tag_links (
  key    TEXT    NOT NULL,
  tag_id INTEGER NOT NULL REFERENCES fin_tags(id) ON DELETE CASCADE,
  PRIMARY KEY (key, tag_id)
);

CREATE TABLE fin_tags (
  id       INTEGER PRIMARY KEY,
  name     TEXT    NOT NULL UNIQUE,
  color    TEXT    NOT NULL,
  position INTEGER NOT NULL
);

CREATE TABLE fin_tx (
  id        INTEGER PRIMARY KEY,
  owner_id  INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  at        TEXT    NOT NULL,            -- 'YYYY-MM-DDTHH:MM'
  type      TEXT    NOT NULL,            -- 수입 | 지출 | 이체
  cat1      TEXT    NOT NULL DEFAULT '',
  cat2      TEXT    NOT NULL DEFAULT '',
  content   TEXT    NOT NULL DEFAULT '',
  amount    INTEGER NOT NULL,            -- 지출은 음수, 환불은 양수
  method    TEXT    NOT NULL DEFAULT '',
  memo      TEXT    NOT NULL DEFAULT '',
  hash      TEXT    NOT NULL,
  UNIQUE (owner_id, hash)
);

CREATE TABLE kv (
  key   TEXT PRIMARY KEY,
  value TEXT NOT NULL
);

CREATE TABLE login_attempts (
  key          TEXT    PRIMARY KEY,
  fails        INTEGER NOT NULL DEFAULT 0,  -- 연속 실패 수 (성공하면 행 삭제)
  window_start INTEGER NOT NULL,            -- unix 초, IP 레이트 윈도의 시작
  window_count INTEGER NOT NULL DEFAULT 0,  -- 그 윈도 안의 시도 수
  locked_until INTEGER NOT NULL DEFAULT 0   -- unix 초, 0이면 잠기지 않음
);

CREATE TABLE media (
  id         TEXT    PRIMARY KEY,           -- sha256 hex
  ext        TEXT    NOT NULL,              -- 'jpg' 만 받는다(재인코딩)
  bytes      INTEGER NOT NULL,
  width      INTEGER NOT NULL,
  height     INTEGER NOT NULL,
  created_by INTEGER REFERENCES users(id) ON DELETE SET NULL,
  created_at INTEGER NOT NULL
);

CREATE TABLE notify_rules (
  id         INTEGER PRIMARY KEY,
  kind       TEXT    NOT NULL,              -- 'tasks' | 'routines' | 'care_gap' | 'diary' | 'custom'
  title      TEXT    NOT NULL DEFAULT '',   -- 목록에 보이는 이름. custom 이면 알림 문구
  days_mask  INTEGER NOT NULL DEFAULT 127,  -- bit0=월 … bit6=일 (루틴과 같다)
  times      TEXT    NOT NULL DEFAULT '[]', -- ["21:00"]. care_gap 은 [시작, 끝] 감시 구간
  params     TEXT    NOT NULL DEFAULT '{}', -- 종류별 조건 (JSON)
  recipients TEXT    NOT NULL DEFAULT '[]', -- 받을 가족 id. 비면 모두
  enabled    INTEGER NOT NULL DEFAULT 1,
  created_by INTEGER REFERENCES users(id) ON DELETE SET NULL,
  created_at INTEGER NOT NULL
);

CREATE TABLE notify_sent (
  rule_id INTEGER NOT NULL REFERENCES notify_rules(id) ON DELETE CASCADE,
  key     TEXT    NOT NULL,
  sent_at INTEGER NOT NULL,
  PRIMARY KEY (rule_id, key)
);

CREATE TABLE push_subs (
  endpoint   TEXT    PRIMARY KEY,
  user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  p256dh     TEXT    NOT NULL,
  auth       TEXT    NOT NULL,
  device     TEXT    NOT NULL DEFAULT '',
  created_at INTEGER NOT NULL,
  last_ok_at INTEGER
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
, created_by INTEGER REFERENCES users(id) ON DELETE SET NULL);

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
, birth_date TEXT);

CREATE INDEX bf_batches_name    ON bf_batches(child_id, name, id);

CREATE INDEX bf_days_child      ON bf_days(child_id, dday);

CREATE INDEX bf_logs_name ON bf_logs(child_id, name);

CREATE INDEX bf_meal_items_name ON bf_meal_items(name);

CREATE INDEX bf_meals_day       ON bf_meals(day_id);

CREATE INDEX cards_archived ON cards(archived_at) WHERE archived_at IS NOT NULL;

CREATE INDEX cards_column ON cards(column_id, position);

CREATE INDEX cards_due ON cards(due_at) WHERE due_at IS NOT NULL;

CREATE INDEX cards_end ON cards(end_at) WHERE end_at IS NOT NULL;

CREATE INDEX cards_priority ON cards(priority);

CREATE INDEX cards_recur ON cards(recur) WHERE recur IS NOT NULL;

CREATE INDEX care_log_items_ing ON care_log_items(ingredient_id);

CREATE INDEX care_logs_child_at ON care_logs(child_id, at);

CREATE INDEX care_logs_meal ON care_logs(meal_id) WHERE meal_id IS NOT NULL;

CREATE UNIQUE INDEX care_logs_source ON care_logs(source) WHERE source IS NOT NULL;

CREATE INDEX columns_board ON columns(board_id, position);

CREATE INDEX diary_date ON diary(date);

CREATE INDEX diary_refs_target ON diary_refs(kind, ref_id);

CREATE UNIQUE INDEX diary_source ON diary(source) WHERE source IS NOT NULL;

CREATE INDEX fin_items_snap ON fin_items(snapshot_id);

CREATE INDEX fin_tag_links_tag ON fin_tag_links(tag_id);

CREATE INDEX fin_tx_owner_at ON fin_tx(owner_id, at);

CREATE INDEX login_attempts_locked ON login_attempts(locked_until);

CREATE INDEX routine_checks_date ON routine_checks(date);

CREATE INDEX sessions_expires ON sessions(expires_at);

