-- 지역화폐처럼 충전해서 쓰는 지갑. 이용 내역을 받을 수 없어 쓴 건 직접 적는다.
-- 잔액 = base_balance + base_at 이후 충전(통장 내역에서 match 가 든 이체) - base_at 이후 적은 사용.
CREATE TABLE fin_wallets (
  id           INTEGER PRIMARY KEY,
  owner_id     INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  name         TEXT    NOT NULL,
  match        TEXT    NOT NULL,   -- 충전 이체의 내용에 들어 있는 글자(예: 경기지역화폐)
  base_balance INTEGER NOT NULL,
  base_at      TEXT    NOT NULL,   -- 'YYYY-MM-DDTHH:MM'
  created_at   INTEGER NOT NULL
);
CREATE TABLE fin_wallet_spends (
  id         INTEGER PRIMARY KEY,
  wallet_id  INTEGER NOT NULL REFERENCES fin_wallets(id) ON DELETE CASCADE,
  at         TEXT    NOT NULL,
  title      TEXT    NOT NULL,
  amount     INTEGER NOT NULL,    -- 양수
  created_by INTEGER REFERENCES users(id) ON DELETE SET NULL,
  created_at INTEGER NOT NULL
);
CREATE INDEX fin_wallet_spends_wallet ON fin_wallet_spends(wallet_id, at);
