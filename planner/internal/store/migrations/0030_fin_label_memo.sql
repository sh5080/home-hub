-- 항목 메모. 이름은 거래를 찾고 묶는 기준이라 가급적 고치지 않고, 설명은 여기에 적는다.
ALTER TABLE fin_labels ADD COLUMN memo TEXT NOT NULL DEFAULT '';
