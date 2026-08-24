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
, content TEXT, priority INTEGER NOT NULL DEFAULT 0, end_at TEXT);

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

CREATE INDEX cards_column ON cards(column_id, position);

CREATE INDEX cards_due ON cards(due_at) WHERE due_at IS NOT NULL;

CREATE INDEX cards_end ON cards(end_at) WHERE end_at IS NOT NULL;

CREATE INDEX cards_priority ON cards(priority);

CREATE INDEX columns_board ON columns(board_id, position);

CREATE INDEX login_attempts_locked ON login_attempts(locked_until);

CREATE INDEX routine_checks_date ON routine_checks(date);

CREATE INDEX sessions_expires ON sessions(expires_at);

