BINARY := hub
PKG := ./cmd/hub

.PHONY: build run test vet fmt tidy pi deploy clean

build:
	go build -o $(BINARY) $(PKG)

run:
	go run $(PKG) --config configs/devices.yaml

test:
	go test ./...

vet:
	go vet ./...

fmt:
	gofmt -l -w .

tidy:
	go mod tidy

# Raspberry Pi 3B (ARMv7, 32bit Bookworm) cross-compile.
# CGO_ENABLED=0으로 정적 링크 — Pi에 툴체인/glibc 버전 걱정 없이 바이너리만 던지면 된다.
pi:
	GOOS=linux GOARCH=arm GOARM=7 CGO_ENABLED=0 \
		go build -trimpath -ldflags="-s -w" -o $(BINARY)-arm $(PKG)

# 빌드 → Pi로 전송 → 서비스 재시작. 자세한 절차는 docs/pi-deploy.md.
#
# 접속 정보는 레포에 넣지 않는다. deploy/local.mk(git 제외)에 적거나 환경변수로 준다:
#   PI_HOST = pi@192.168.0.x
#   PI_KEY  = ~/path/to/key.pem
-include deploy/local.mk
PI_HOST ?= pi@raspberrypi.local
PI_KEY  ?= ~/.ssh/id_rsa
deploy: pi
	scp -i $(PI_KEY) $(BINARY)-arm $(PI_HOST):/var/lib/homehub/bin/hub.new
	scp -i $(PI_KEY) configs/pi.yaml $(PI_HOST):/var/lib/homehub/configs/pi.yaml
	ssh -i $(PI_KEY) $(PI_HOST) 'chmod +x /var/lib/homehub/bin/hub.new \
		&& mv /var/lib/homehub/bin/hub.new /var/lib/homehub/bin/hub \
		&& sudo systemctl restart homehub && sleep 3 \
		&& systemctl is-active homehub && curl -sf http://127.0.0.1:8086/healthz'

clean:
	rm -f $(BINARY) $(BINARY)-arm
