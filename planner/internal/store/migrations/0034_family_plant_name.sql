-- 식물 이름(언제든 바꾼다). 다 키운 식물은 구매권 하나가 되고, 위시리스트를 살 때 쓴다(family_wishes.plant_id).
ALTER TABLE family_plants ADD COLUMN name TEXT NOT NULL DEFAULT '';
