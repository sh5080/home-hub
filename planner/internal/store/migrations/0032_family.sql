-- 가족 퀘스트. 진행은 다른 기능의 기록에서 매번 계산하고, 여기엔 사람이 정한 것만 둔다.
-- week 는 그 주 월요일('YYYY-MM-DD'). reward 는 다 채우면 할 보상(사람이 적는다).
CREATE TABLE family_weeks (
  week       TEXT PRIMARY KEY,
  reward     TEXT NOT NULL DEFAULT '',
  updated_by INTEGER REFERENCES users(id) ON DELETE SET NULL,
  updated_at INTEGER NOT NULL
);

-- 함께 키우는 식물. 물방울을 모아 5단계가 되면 위시리스트에서 하나를 사고 새 씨앗을 심는다.
CREATE TABLE family_plants (
  id          INTEGER PRIMARY KEY,
  started     TEXT    NOT NULL,   -- 'YYYY-MM-DD' 이날부터 모은 물방울만 센다
  finished_at INTEGER,
  wish_id     INTEGER
);
-- 물방울 장부. key 가 같은 건 한 번만 준다(같은 날 같은 연속 기록, 같은 주 같은 퀘스트…).
CREATE TABLE family_drops (
  key    TEXT    PRIMARY KEY,
  amount INTEGER NOT NULL,
  day    TEXT    NOT NULL,        -- 'YYYY-MM-DD' 얻은 날
  label  TEXT    NOT NULL
);
CREATE INDEX family_drops_day ON family_drops(day);
CREATE TABLE family_wishes (
  id         INTEGER PRIMARY KEY,
  title      TEXT    NOT NULL,
  price      INTEGER NOT NULL DEFAULT 0,
  note       TEXT    NOT NULL DEFAULT '',
  created_by INTEGER REFERENCES users(id) ON DELETE SET NULL,
  created_at INTEGER NOT NULL,
  bought_at  INTEGER,
  plant_id   INTEGER REFERENCES family_plants(id) ON DELETE SET NULL
);
