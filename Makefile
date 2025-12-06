BINARY := hub
PKG := ./cmd/hub

.PHONY: build run test vet fmt tidy pi clean

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

# Raspberry Pi (ARMv7) cross-compile
pi:
	GOOS=linux GOARCH=arm GOARM=7 go build -o $(BINARY)-arm $(PKG)

clean:
	rm -f $(BINARY) $(BINARY)-arm
