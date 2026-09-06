-- 이유식 기록을 글자가 아니라 재료로.
--
-- 지금까지 이유식 기록의 메뉴는 detail 에 적힌 글자였다. 같은 재료가
-- '쌀오트밀'·'쌀오트밀미음'·'오트밀' 로 흩어지면 식단의 재료와 이어지지
-- 않는다. 이제 식단과 같은 재료 목록(bf_ingredients)에서 골라 id 로 건다.
-- detail 은 그 이름들을 이어 붙인 보기용 사본으로만 남는다.
CREATE TABLE care_log_items (
  log_id        INTEGER NOT NULL REFERENCES care_logs(id) ON DELETE CASCADE,
  pos           INTEGER NOT NULL,
  ingredient_id INTEGER NOT NULL REFERENCES bf_ingredients(id) ON DELETE CASCADE,
  PRIMARY KEY (log_id, pos)
);
CREATE INDEX care_log_items_ing ON care_log_items(ingredient_id);

-- 그 기록이 식단의 어느 끼니를 먹인 것인지. 식단 화면이 "먹었어요"를
-- 보여주는 근거다. 끼니가 지워지면 기록은 남고 연결만 풀린다.
ALTER TABLE care_logs ADD COLUMN meal_id INTEGER REFERENCES bf_meals(id) ON DELETE SET NULL;
CREATE INDEX care_logs_meal ON care_logs(meal_id) WHERE meal_id IS NOT NULL;
