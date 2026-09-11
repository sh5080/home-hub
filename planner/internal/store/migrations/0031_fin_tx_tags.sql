-- 태그를 거래 한 건에 붙인다. 같은 가게라도 거래마다 쓰임이 달라서(다이소: 생활용품·육아·…) 이름 단위로
-- 붙이면 새 거래가 엉뚱한 태그를 물려받는다. ref 는 't<fin_tx.id>' 또는 'w<fin_wallet_spends.id>'.
-- 고정 항목(관리비·급여)은 매달 같은 것이라 여전히 fin_tag_links(항목 key)에 붙는다.
CREATE TABLE fin_tx_tags (
  ref    TEXT    NOT NULL,
  tag_id INTEGER NOT NULL REFERENCES fin_tags(id) ON DELETE CASCADE,
  PRIMARY KEY (ref, tag_id)
);
CREATE INDEX fin_tx_tags_tag ON fin_tx_tags(tag_id);
