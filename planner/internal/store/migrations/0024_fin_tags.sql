-- 재정 항목 태그. 항목은 거래 내용을 정규화한 key 로 가리킨다(fin_fixed 와 같다) — 같은 가게는 같은 태그.
CREATE TABLE fin_tags (
  id       INTEGER PRIMARY KEY,
  name     TEXT    NOT NULL UNIQUE,
  color    TEXT    NOT NULL,
  position INTEGER NOT NULL
);
CREATE TABLE fin_tag_links (
  key    TEXT    NOT NULL,
  tag_id INTEGER NOT NULL REFERENCES fin_tags(id) ON DELETE CASCADE,
  PRIMARY KEY (key, tag_id)
);
CREATE INDEX fin_tag_links_tag ON fin_tag_links(tag_id);
INSERT INTO fin_tags (name, color, position) VALUES
  ('생활비', 'emerald', 0), ('주거', 'amber', 1), ('통신', 'sky', 2), ('보험', 'indigo', 3),
  ('구독', 'violet', 4), ('육아', 'pink', 5), ('의료', 'rose', 6), ('교통', 'cyan', 7),
  ('경조사', 'orange', 8), ('여가', 'lime', 9), ('쇼핑', 'fuchsia', 10), ('대출', 'slate', 11);
