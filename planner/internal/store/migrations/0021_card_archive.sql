-- 완료한 카드의 보관.
--
-- 완료 칸은 끝없이 길어진다. 한 달 넘게 지난 완료 카드는 보드에서 빼
-- '보관'으로 넘기고, 완료 칸의 보관함에서만 찾아보게 한다. 상태 칸을
-- 새로 만들지 않는 건, 보관은 칸이 아니라 "완료 칸 안에서 오래된 것"이라서다.
-- 보관된 카드를 다른 칸으로 옮기면 보관이 풀린다.
--
-- done_at: 완료 칸에 들어온 시각. 이게 있어야 "완료한 지 한 달"을 셀 수
--          있다. updated_at 은 완료 뒤 메모만 고쳐도 바뀌어 기준이 못 된다.
-- archived_at: 보관된 시각. NULL 이면 보드에 보인다.
ALTER TABLE cards ADD COLUMN done_at INTEGER;
ALTER TABLE cards ADD COLUMN archived_at INTEGER;

-- 이미 완료 칸에 있는 카드는 마지막으로 고친 때를 완료 시각으로 본다.
-- 정확하진 않지만 그 뒤로 옮겨진 적이 없으니 가장 가까운 추정이다.
UPDATE cards SET done_at = updated_at
 WHERE column_id IN (
   SELECT (SELECT c2.id FROM columns c2 WHERE c2.board_id = b.id ORDER BY c2.position DESC, c2.id DESC LIMIT 1)
     FROM boards b);

CREATE INDEX cards_archived ON cards(archived_at) WHERE archived_at IS NOT NULL;
