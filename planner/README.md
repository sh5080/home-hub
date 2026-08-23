# planner — 단아네 플래너

칸반 · 캘린더 · 주간 루틴을 한 화면에서 보는 가족용 웹앱. home-hub와 **같은 저장소의
별도 서브프로젝트**(자체 `go.mod`)이고, Pi에서는 **별도 바이너리·별도 systemd 유닛**으로
돈다. 플래너가 죽어도 실링팬은 켜져야 하니까.

- 백엔드: Go stdlib `net/http` + `modernc.org/sqlite`(순수 Go → `CGO_ENABLED=0` 크로스컴파일)
- 프론트: React + Vite + TypeScript. **Mac에서 빌드**해 `internal/webui/dist`로 내고 Go 바이너리에 `embed`.
  Pi는 Node를 모른다 — 정적 파일을 서빙할 뿐이다.
- 데이터: 가족 전체 공유. 로그인은 "누가 했는지" 기록용.
- 접근: `127.0.0.1:8090`에만 바인딩. 외부는 Tailscale(`tailscale serve --bg 8090`)이 담당 →
  포트포워딩·공개 노출 없음, 도메인·인증서 자동.

## 개발

```bash
make dev                       # Go(8090, --dev) + Vite(5190). Ctrl-C 한 번에 둘 다 종료
go run ./cmd/planner user add 이름 --data ./data   # 첫 사용자 (비밀번호는 터미널에서)
make test vet
```

- UI는 `http://localhost:5190` (HMR). `/api`는 8090으로 프록시.
- `--dev`는 세션 쿠키의 `Secure`를 뺀다 — `tailscale serve` 뒤에선 `r.TLS==nil`이라 요청으로
  판단할 수 없고, Safari는 http://localhost에서 Secure 쿠키를 거부한다.

## 배포 (Pi)

1회 준비:

```bash
sudo mkdir -p /var/lib/planner/{bin,data} && sudo chown -R sh5080:sh5080 /var/lib/planner
sudo cp planner/deploy/planner.service /etc/systemd/system/
sudo systemctl daemon-reload && sudo systemctl enable --now planner
```

이후엔 `make deploy` (프론트 빌드 → ARMv7 크로스컴파일 → scp → **DB 스냅샷** → 원자적 교체 →
재시작 → healthz). 접속 정보는 `../deploy/local.mk`(git 제외).

Tailscale(사용자 작업): Pi에 설치 → 관리 콘솔에서 MagicDNS·HTTPS Certificates 켜기 →
`sudo tailscale serve --bg 8090` → 가족 폰에 Tailscale 앱. **설치 직후 홈 앱에서 팬이
응답하는지 확인** — `tailscale0` 인터페이스가 mDNS를 흔들 수 있는 유일한 지점이다.

## CLI

```
planner serve  [--listen ADDR] [--data DIR] [--log LEVEL] [--dev]
planner user   add|passwd|del|list [name]
planner backup [--out FILE]          # VACUUM INTO, 서비스 중에도 안전
planner migrate status               # 적용/대기/변조 여부 (적용은 serve가 시작 시 함)
```

Pi에서 CLI는 서비스와 같은 사용자(`sh5080`)로 실행한다 — `-wal`/`-shm` 소유권.

## 스키마 관리

`internal/store/migrations/NNNN_name.sql`을 바이너리에 embed하고, 시작 시
`schema_migrations`(버전·이름·sha256·적용시각)와 대조해 빠진 것만 순서대로 각각
트랜잭션으로 적용한다.

- **배포된 파일을 수정하면 서버가 기동을 거부한다.** 새 번호로 파일을 추가한다.
- `testdata/schema.sql`이 전체 마이그레이션 결과 스키마의 스냅샷이다. 마이그레이션을
  추가하면 `UPDATE_SNAPSHOT=1 go test ./internal/store -run TestSchemaSnapshot`으로 갱신해
  같이 커밋한다 — 그 diff가 리뷰 대상.
- SQLite `ALTER TABLE`은 ADD/RENAME/DROP COLUMN까지만. 타입·제약 변경은 테이블 재생성
  (`CREATE new → INSERT SELECT → DROP → RENAME`)을 마이그레이션 하나 안에서 한다.
- 롤백은 down 마이그레이션이 아니라 `make deploy`가 재시작 직전에 남기는
  `data/backup-*.db`(최근 5개)다.

## 시간 표현

사용자가 고른 일시·날짜는 **타임존 없는 로컬 문자열**로 저장한다:
`YYYY-MM-DDTHH:MM`(`datetime-local` 그대로) / `YYYY-MM-DD`. 사전식 비교 = 시간순 비교라
범위 쿼리가 문자열 비교로 끝나고, 서버는 tz 변환을 하지 않는다. 서버가 만든 감사 필드만
unix 초. 요일 마스크는 bit0=월 … bit6=일(ISO).

## 칸반 규칙

각 보드의 **첫 컬럼 = 할 일, 마지막 컬럼 = 완료**. 이름이 아니라 위치 기준이라 이름을
바꿔도 홈의 "오늘 할 일"과 체크→완료 이동이 그대로 동작한다. 카드 순서는 splice
의미론(`{column_id, position}` = 목적지 인덱스)이고 `(column_id, position)`에 UNIQUE를
걸지 않는다 — 시프트 UPDATE가 행 단위로 검사돼 일시적으로 충돌한다.
