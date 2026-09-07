-- 알림.
--
-- 규칙(무엇을, 언제, 누구에게)과 기기(어디로 보내나)와 보낸 기록(두 번
-- 보내지 않게)을 나눈다.
--
-- 알림은 "조건이 아직 풀리지 않았을 때만" 간다. 오늘 할 일을 다 끝냈으면
-- 21시의 '할 일 남았어요'는 가지 않는다 — 보내는 순간에 조건을 다시 본다.

-- 서버 설정 한 줄짜리 값들. 지금은 웹푸시 서명 키(VAPID)만 산다.
CREATE TABLE kv (
  key   TEXT PRIMARY KEY,
  value TEXT NOT NULL
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

-- 기기마다 하나. 같은 사람이 폰과 태블릿을 쓰면 둘이다.
CREATE TABLE push_subs (
  endpoint   TEXT    PRIMARY KEY,
  user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  p256dh     TEXT    NOT NULL,
  auth       TEXT    NOT NULL,
  device     TEXT    NOT NULL DEFAULT '',
  created_at INTEGER NOT NULL,
  last_ok_at INTEGER
);

-- 보낸 기록. key 는 규칙마다 "한 번만"의 단위다 — 시각 알림은 날짜+시각,
-- 기록 공백은 마지막 기록의 id(새 기록이 생기면 다시 보낼 수 있다).
CREATE TABLE notify_sent (
  rule_id INTEGER NOT NULL REFERENCES notify_rules(id) ON DELETE CASCADE,
  key     TEXT    NOT NULL,
  sent_at INTEGER NOT NULL,
  PRIMARY KEY (rule_id, key)
);
