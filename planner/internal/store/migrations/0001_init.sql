-- 0001_init: 가족 플래너 초기 스키마.
--
-- 시간 표현 규칙 (중요):
--   사용자가 고르는 일시/날짜는 타임존 없는 로컬 벽시계 문자열로 저장한다.
--     일시  'YYYY-MM-DDTHH:MM'   (<input type="datetime-local">가 내는 그대로)
--     날짜  'YYYY-MM-DD'
--   사전식 비교가 시간순 비교와 같아서 범위 쿼리가 문자열 비교로 끝난다.
--   서버가 만든 감사 필드(created_at 등)만 unix 초 INTEGER.

CREATE TABLE users (
  id            INTEGER PRIMARY KEY,
  name          TEXT    NOT NULL UNIQUE,
  password_hash TEXT    NOT NULL,
  created_at    INTEGER NOT NULL
);

CREATE TABLE sessions (
  token      TEXT    PRIMARY KEY,
  user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  expires_at INTEGER NOT NULL
);
CREATE INDEX sessions_expires ON sessions(expires_at);

CREATE TABLE boards (
  id         INTEGER PRIMARY KEY,
  name       TEXT    NOT NULL,
  created_by INTEGER REFERENCES users(id) ON DELETE SET NULL,  -- 시드 보드는 사용자보다 먼저 생긴다
  created_at INTEGER NOT NULL
);

CREATE TABLE columns (
  id       INTEGER PRIMARY KEY,
  board_id INTEGER NOT NULL REFERENCES boards(id) ON DELETE CASCADE,
  name     TEXT    NOT NULL,
  position INTEGER NOT NULL
);
CREATE INDEX columns_board ON columns(board_id, position);

CREATE TABLE cards (
  id          INTEGER PRIMARY KEY,
  column_id   INTEGER NOT NULL REFERENCES columns(id) ON DELETE CASCADE,
  title       TEXT    NOT NULL,
  description TEXT    NOT NULL DEFAULT '',
  position    INTEGER NOT NULL,
  due_date    TEXT,                                           -- 'YYYY-MM-DD' | NULL
  assignee_id INTEGER REFERENCES users(id) ON DELETE SET NULL,
  created_by  INTEGER NOT NULL REFERENCES users(id),
  created_at  INTEGER NOT NULL,
  updated_at  INTEGER NOT NULL
);
-- UNIQUE(column_id, position)를 걸지 않는다: 이동 시 position±1 시프트 UPDATE가
-- 행 단위로 검사되는 SQLite 특성상 일시적으로 충돌해 실패한다.
CREATE INDEX cards_column ON cards(column_id, position);
CREATE INDEX cards_due    ON cards(due_date) WHERE due_date IS NOT NULL;

CREATE TABLE events (
  id         INTEGER PRIMARY KEY,
  title      TEXT    NOT NULL,
  start_at   TEXT    NOT NULL,   -- 'YYYY-MM-DDTHH:MM', 종일이면 'YYYY-MM-DD'
  end_at     TEXT,               -- 같은 형식, NULL = 시점/하루
  all_day    INTEGER NOT NULL DEFAULT 0,
  created_by INTEGER NOT NULL REFERENCES users(id),
  created_at INTEGER NOT NULL
);
CREATE INDEX events_start ON events(start_at);

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

CREATE TABLE routine_checks (
  routine_id INTEGER NOT NULL REFERENCES routines(id) ON DELETE CASCADE,
  date       TEXT    NOT NULL,     -- 'YYYY-MM-DD'
  checked_by INTEGER NOT NULL REFERENCES users(id),
  checked_at INTEGER NOT NULL,
  PRIMARY KEY (routine_id, date)
);
CREATE INDEX routine_checks_date ON routine_checks(date);
