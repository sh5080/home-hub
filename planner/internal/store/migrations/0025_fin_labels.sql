-- 항목(거래 내용 key)의 이름·분류를 사람이 고친 것. 빈 칸은 파일 값 그대로.
-- 같은 이름으로 고친 항목들은 수입원에서 하나로 묶인다(예: 회사 이름·'급여'·'10월 급여' → '급여').
CREATE TABLE fin_labels (
  key   TEXT PRIMARY KEY,
  label TEXT NOT NULL DEFAULT '',
  cat1  TEXT NOT NULL DEFAULT ''
);
