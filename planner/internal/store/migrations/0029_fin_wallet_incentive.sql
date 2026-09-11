-- 충전하면 인센티브가 붙는다(경기지역화폐 10%). 잔액 = 충전 + 인센티브라 둘 다 쓸 수 있는 돈이다.
-- base_balance 는 맞출 때의 합계(충전 잔액 + 인센티브 잔액), base_incentive 는 그중 인센티브(보여주기용).
ALTER TABLE fin_wallets ADD COLUMN incentive_rate REAL NOT NULL DEFAULT 0.1;
ALTER TABLE fin_wallets ADD COLUMN base_incentive INTEGER NOT NULL DEFAULT 0;
