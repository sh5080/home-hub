-- 다른 앱에서 가져온 육아 기록의 출처.
--
-- 베이비타임 내보내기처럼 한꺼번에 들어오는 기록은 같은 파일을 두 번 넣어도
-- 한 번만 들어가야 한다. 'babytime:2026-09-01T07:40|formula|1' 처럼 적는다.
ALTER TABLE care_logs ADD COLUMN source TEXT;
CREATE UNIQUE INDEX care_logs_source ON care_logs(source) WHERE source IS NOT NULL;
