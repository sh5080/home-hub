-- 수입원 판정을 사람이 뒤집은 것. key 는 거래 내용을 정규화한 것(fin_fixed 와 같다).
CREATE TABLE fin_income (
  key    TEXT PRIMARY KEY,
  income INTEGER NOT NULL
);
