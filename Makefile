BINARY := hub
PKG := ./cmd/hub

.PHONY: build run tidy pi clean

build:
	go build -o $(BINARY) $(PKG)

run:
	go run $(PKG) --config configs/devices.yaml

tidy:
	go mod tidy

# Raspberry Pi (ARMv7) cross-compile
pi:
	GOOS=linux GOARCH=arm GOARM=7 go build -o $(BINARY)-arm $(PKG)

clean:
	rm -f $(BINARY) $(BINARY)-arm
