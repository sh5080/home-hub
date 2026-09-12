-- 처음 정한 마감. 마감을 미뤄도 그대로 남아, 처음 약속한 날까지 끝냈는지 본다(할 일 보상).
ALTER TABLE cards ADD COLUMN first_due TEXT;
UPDATE cards SET first_due = due_at WHERE due_at IS NOT NULL;
