-- 육아 기록: 수유·기저귀·수면처럼 하루에 여러 번 남기는 것들.
--
-- 이유식 표는 "무엇을 먹일지"의 계획이고, 이건 "언제 무엇을 했는지"의 실행
-- 기록이다. 한 표에 종류(kind)만 다르게 넣는다 — 모유·분유·기저귀를 표로
-- 나누면 "오늘 있었던 일"을 시간순으로 보여줄 때 열다섯 표를 합쳐야 한다.
--
-- 시각은 카드의 마감과 같은 떠다니는 로컬 시각('YYYY-MM-DDTHH:MM')이다.
-- 분·양·상세는 종류마다 뜻이 다르고 없을 수도 있어 전부 NULL 허용이다:
--   minutes   모유·수면·유축·목욕·놀이·터미타임이 얼마나 걸렸는지. NULL 이면
--             아직 진행 중(수면은 재우고 나서 깰 때 채운다)
--   amount_ml 분유·유축 수유·우유·물·유축(짠 양)
--   detail    기저귀 '소변'|'대변'|'둘 다', 모유 '왼쪽'|'오른쪽'|'양쪽',
--             투약은 약 이름, 이유식은 메뉴
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
);
CREATE INDEX care_logs_child_at ON care_logs(child_id, at);
