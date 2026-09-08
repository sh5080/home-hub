-- 재정: 가계부 앱에서 내보낸 파일(엑셀)을 사람별로 올려 쌓는다.
-- 파일 자체는 저장하지 않는다(이름·신용점수가 들어 있다). 읽은 숫자만.

-- 올린 파일 하나 = 그 시점의 자산·부채 스냅샷.
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

-- 스냅샷의 한 줄. group_: liquid(자유입출금·현금·전자금융) | saving | invest | insurance | pension | other | debt
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
CREATE INDEX fin_items_snap ON fin_items(snapshot_id);

-- 거래. 파일마다 1년치가 겹쳐 오므로 hash 로 거른다(사람별).
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
CREATE INDEX fin_tx_owner_at ON fin_tx(owner_id, at);

-- 고정지출 판정을 사람이 뒤집은 것. key 는 정규화한 거래 내용.
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
