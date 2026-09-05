-- 사진, 그리고 다른 앱에서 가져온 일기의 출처.
--
-- 일기에 사진이 붙는다. 파일은 DB 가 아니라 data/media/ 에 두고(수십 MB 를
-- SQLite 에 넣으면 백업과 VACUUM 이 그만큼 무거워진다), 여기에는 "무엇이
-- 있는지"만 적는다. 이름은 내용의 sha256 이다 — 같은 사진을 두 번 올려도
-- 파일은 하나고, 이름만 보고 경로를 만들 수 있으니 경로 탐색 걱정이 없다.
CREATE TABLE media (
  id         TEXT    PRIMARY KEY,           -- sha256 hex
  ext        TEXT    NOT NULL,              -- 'jpg' 만 받는다(재인코딩)
  bytes      INTEGER NOT NULL,
  width      INTEGER NOT NULL,
  height     INTEGER NOT NULL,
  created_by INTEGER REFERENCES users(id) ON DELETE SET NULL,
  created_at INTEGER NOT NULL
);

-- 목록에서 사진 몇 장을 미리 보여주려면 본문을 열지 않고도 알아야 한다.
-- plain 처럼 저장할 때 본문에서 뽑아 둔다. JSON 배열, 최대 4장.
ALTER TABLE diary ADD COLUMN photos TEXT NOT NULL DEFAULT '[]';

-- 가져온 글의 출처. 'babytime:181_1' 처럼 적어, 같은 export 를 두 번 돌려도
-- 두 번 들어가지 않게 한다.
ALTER TABLE diary ADD COLUMN source TEXT;
CREATE UNIQUE INDEX diary_source ON diary(source) WHERE source IS NOT NULL;
