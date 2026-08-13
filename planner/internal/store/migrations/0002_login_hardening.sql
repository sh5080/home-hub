-- 0002_login_hardening: 로그인 시도 추적.
--
-- 플래너가 tailscale funnel로 공개되면 로그인이 유일한 방어선이 된다. 카운터를
-- 메모리에 두면 Restart=always와 make deploy가 매번 0으로 되돌리므로 DB에 둔다.
-- 덤으로 공격이 있었는지 나중에 눈으로 확인할 수 있다.
--
-- key 형식: 'user:<이름>' 또는 'ip:<주소>'. 한 번의 로그인 시도가 두 행을 건드린다.
--   계정 키 — 가족 계정을 보호한다(점진적 잠금).
--   IP  키 — Pi CPU를 보호한다. bcrypt 한 번이 3B에서 ~1초라 그 자체가 DoS 벡터다.

CREATE TABLE login_attempts (
  key          TEXT    PRIMARY KEY,
  fails        INTEGER NOT NULL DEFAULT 0,  -- 연속 실패 수 (성공하면 행 삭제)
  window_start INTEGER NOT NULL,            -- unix 초, IP 레이트 윈도의 시작
  window_count INTEGER NOT NULL DEFAULT 0,  -- 그 윈도 안의 시도 수
  locked_until INTEGER NOT NULL DEFAULT 0   -- unix 초, 0이면 잠기지 않음
);
CREATE INDEX login_attempts_locked ON login_attempts(locked_until);
