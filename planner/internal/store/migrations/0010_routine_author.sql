-- 루틴에 작성자를 남긴다.
--
-- 카드에는 created_by 가 처음부터 있었는데 루틴에만 없었다. 가족이 같이 쓰는
-- 앱에서 "이거 누가 만든 거야"를 물을 곳이 없으면 지우지도 고치지도 못한다.
--
-- NULL 을 허용한다. 이미 들어 있는 루틴은 누가 만들었는지 알 방법이 없고,
-- 아무나 골라 넣으면 없는 사실을 지어내는 것이다.
ALTER TABLE routines ADD COLUMN created_by INTEGER REFERENCES users(id) ON DELETE SET NULL;
