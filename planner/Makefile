BINARY := planner
PKG    := ./cmd/planner

.PHONY: build run test vet fmt tidy web dev pi deploy clean

build:
	go build -o $(BINARY) $(PKG)

# 로컬 개발 서버 (--dev: Vite http://localhost용으로 Secure 쿠키 해제)
run:
	go run $(PKG) serve --dev --data ./data

test:
	go test ./...

vet:
	go vet ./...

fmt:
	gofmt -s -w .

tidy:
	go mod tidy

# 프론트 빌드 → internal/webui/dist (Go embed 대상). Mac에서만 실행한다.
# dist/.gitkeep은 남기고 이전 번들만 지운다 (emptyOutDir:false 와 짝).
web:
	cd web && npm ci && rm -rf ../internal/webui/dist/assets && npm run build

# Go(8090, --dev) + Vite(5190)를 한 터미널에서. Ctrl-C 한 번에 둘 다 종료.
# .ONESHELL 없이는 Make가 줄마다 셸을 새로 띄워 trap이 죽는다.
.ONESHELL:
dev:
	set -e
	mkdir -p data
	go run $(PKG) serve --dev --data ./data &
	GO_PID=$$!
	trap 'kill $$GO_PID 2>/dev/null' EXIT INT TERM
	cd web && npm run dev

# Raspberry Pi 3B (ARMv7, 32bit Bookworm) cross-compile — 루트 Makefile과 동일 플래그.
# 첫 빌드는 modernc.org/sqlite 컴파일 때문에 수 분 걸린다.
pi:
	GOOS=linux GOARCH=arm GOARM=7 CGO_ENABLED=0 \
		go build -trimpath -ldflags="-s -w" -o $(BINARY)-arm $(PKG)

# 프론트 빌드 → 크로스컴파일 → Pi 전송 → 서비스 재시작. 접속 정보는 ../deploy/local.mk (git 제외).
-include ../deploy/local.mk
PI_HOST ?= pi@raspberrypi.local
PI_KEY  ?= ~/.ssh/id_rsa
PI_DIR  := /var/lib/planner
# 재시작 직전에 DB 스냅샷을 뜬다 — 마이그레이션이 잘못됐을 때 되돌릴 유일한 길이다.
# 스냅샷은 $(PI_DIR)/data/backup-*.db 로 남고, 최근 5개만 유지한다.
deploy: web pi
	scp -i $(PI_KEY) $(BINARY)-arm $(PI_HOST):$(PI_DIR)/bin/$(BINARY).new
	ssh -i $(PI_KEY) $(PI_HOST) 'chmod +x $(PI_DIR)/bin/$(BINARY).new \
		&& $(PI_DIR)/bin/$(BINARY) backup --data $(PI_DIR)/data \
		&& ls -t $(PI_DIR)/data/backup-*.db 2>/dev/null | tail -n +6 | xargs -r rm -f \
		&& mv $(PI_DIR)/bin/$(BINARY).new $(PI_DIR)/bin/$(BINARY) \
		&& sudo systemctl restart planner && sleep 3 \
		&& systemctl is-active planner && curl -sf http://127.0.0.1:8090/api/healthz'

clean:
	rm -f $(BINARY) $(BINARY)-arm
