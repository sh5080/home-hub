-- 다이어리.
--
-- 칸반은 '할 일'을, 캘린더는 '언제'를 다룬다. 남는 것이 "그날 어땠는지"다.
-- 카드 본문에 적을 수도 있지만 카드는 끝나면 완료 칸으로 넘어가 묻힌다.
-- 일기는 끝나는 것이 아니라 쌓이는 것이라 자기 자리가 필요하다.
--
-- 하루에 여러 장을 허용한다. '하루 한 장'은 종이 다이어리의 제약이지
-- 사람의 습관이 아니다 — 아침에 한 줄, 밤에 한 장을 쓰는 게 자연스럽다.
CREATE TABLE diary (
  id         INTEGER PRIMARY KEY,
  date       TEXT    NOT NULL,              -- 'YYYY-MM-DD'
  title      TEXT    NOT NULL DEFAULT '',
  content    TEXT,                          -- 블록 편집기 JSON (카드와 같은 형식)
  plain      TEXT    NOT NULL DEFAULT '',   -- 본문에서 뽑은 평문. 목록·검색용
  created_by INTEGER REFERENCES users(id) ON DELETE SET NULL,
  created_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL
);
CREATE INDEX diary_date ON diary(date);

-- 글에 거는 참조.
--
-- 본문 안에 링크를 심지 않고 따로 표에 둔 이유는, 반대 방향으로도 물을 수
-- 있어야 해서다 — "이 카드가 나온 일기"를 찾으려면 본문을 뒤지는 게 아니라
-- 이 표를 봐야 한다.
--
-- label 은 걸던 때의 이름을 그대로 둔다. 대상이 지워지거나 이름이 바뀌어도
-- 일기 쪽 문장은 그때의 기록이어야 한다.
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
CREATE INDEX diary_refs_target ON diary_refs(kind, ref_id);
