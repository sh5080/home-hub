# Home Hub — Architecture

순수 Go 단일 바이너리로 동작하는 홈 오토메이션 허브(HA). **Zigbee · MQTT(ESP32) · HomeKit(HAP)** 을 하나의 프로세스에서 브리지하며, Matter 제어를 위한 **자체 컨트롤러 `go-matter`** 를 별도 모듈로 둔다.

## 1. 설계 원칙

- **단일 프로세스 / 단일 정적 바이너리** — 외부 의존 서비스 0 (MQTT 브로커도 내장).
- **포트 & 어댑터(hexagonal)** — 도메인은 프로토콜을 모르고, 각 프로토콜 어댑터가 포트 인터페이스만 구현한다. 어댑터 교체/추가가 국소적이다.
- **인프로세스 이벤트 버스** — 어댑터끼리 직접 참조하지 않고 pub/sub로만 통신한다.
- **Matter 이음새(seam)** — Matter는 드라이버 인터페이스로 추상화한다. HomeKit 위임 스텁 ↔ 네이티브 컨트롤러(`go-matter`)를 구현 교체만으로 전환한다.

## 2. 컴포넌트 개요

```
        HomeKit controllers (Apple Home 등)
                   │ HAP
 ┌─────────────────┴───────────── Go binary ─────────────────────┐
 │  [HomeKit Adapter] ─┐                    ┌─ [Zigbee Adapter] ── USB ─ Zigbee coordinator
 │  (brutella/hap)     │                    │  (shimmeringbee / zstack)
 │                     ▼                    │
 │  [Automation] ─▶ Event Bus (pub/sub) ◀───┤─ [MQTT Adapter] ──── WiFi ─ ESP32 / ESPHome
 │                     ▲                    │  (embedded mochi-mqtt broker)
 │  [Registry] ────────┘                    └─ [Matter Port] ───── delegate stub │ go-matter
 └────────────────────────────────────────────────────────────────┘
```

중심에 **Event Bus**. 모든 어댑터는 버스로만 대화하고 서로를 직접 모른다.

## 3. 도메인 모델 (`internal/domain`, 외부 의존 0)

```go
type DeviceType string
const (
    Switch DeviceType = "switch"
    Light  DeviceType = "light"
    Fan    DeviceType = "fan"
    Cover  DeviceType = "cover"
    Sensor DeviceType = "sensor"
)

type Device struct {
    ID          string      // 논리 식별자
    Name        string      // 표시 이름
    Integration string      // "zigbee" | "mqtt" | "matter"
    Type        DeviceType
    Addr        string      // zigbee IEEE addr | mqtt topic | matter node id
    State       State       // 현재 상태
}

type Command struct { DeviceID string; Action Action; Value any } // 예: SetOn(true), SetPosition(37)
type Event   struct { DeviceID string; Kind EventKind; State State }
```

기기 종류가 한정적이므로 Type 기반으로 단순화한다. (향후 필요 시 capability 방식으로 확장)

## 4. 포트 & 어댑터

**포트(인터페이스, `internal/driver`)** — 어댑터가 구현할 계약:

```go
type Driver interface {
    Start(ctx context.Context) error
    Apply(cmd domain.Command) error   // 도메인 명령 → 프로토콜 동작
    // 상태 변화는 어댑터가 Event Bus로 publish
}
```

| 어댑터 | 패키지 | 라이브러리 | 담당 |
|---|---|---|---|
| Zigbee | `internal/zigbee` | `shimmeringbee` (zstack/zcl) | 코디네이터 제어·리포트. 벤더 quirk는 별도 파일로 격리 |
| MQTT | `internal/mqtt` | `mochi-mqtt/server` (내장 브로커) | ESP32/ESPHome 토픽 ↔ 도메인 |
| HomeKit | `internal/homekit` | `brutella/hap` | 도메인 기기 → HAP accessory, 가상 트리거 스위치 |
| Matter(포트) | `internal/matter` | 위임 스텁 / `go-matter` | §8 |

## 5. 패키지 레이아웃

```
home-hub/
├── cmd/hub/main.go            # 진입점: 설정 로드 → 어댑터 wiring → 기동
├── internal/
│   ├── domain/               # Device/Command/Event/State (의존 0)
│   ├── bus/                  # in-proc pub/sub 이벤트 버스 (채널 기반)
│   ├── registry/             # 기기 목록 + 현재 상태 저장
│   ├── driver/               # Driver 포트 인터페이스 (이음새)
│   ├── zigbee/               # ZigbeeDriver + 벤더 quirk
│   ├── mqtt/                 # 내장 broker + MQTTDriver 브리지
│   ├── matter/               # Matter Driver 인터페이스 + DelegatedDriver(스텁)
│   ├── homekit/              # HAP 브리지: accessory + 가상 트리거
│   ├── automation/           # 규칙 엔진
│   └── config/               # devices.yaml 로딩
├── configs/devices.yaml
├── deploy/home-hub.service
├── Makefile                  # arm 크로스컴파일
└── go.mod
```

## 6. 데이터 흐름

**① 명령 (HomeKit → 기기)**
```
HomeKit "on" → [HAP] OnValueRemoteUpdate
  → bus.Publish(Command{id, SetOn(true)})
  → [Zigbee] 구독 → 코디네이터에 ZCL On/Off → 기기 반영
```

**② 상태 (기기 → HomeKit)**
```
기기 상태 변화 → 코디네이터 → [Zigbee] 리포트
  → bus.Publish(Event{StateChanged})
  ├─ [HAP] characteristic 갱신 → HomeKit 즉시 반영
  ├─ [Registry] 상태 저장
  └─ [Automation] 규칙 평가
```

**③ Matter 위임 (허브 → HomeKit, 간접 트리거)**
```
[Automation] 조건 충족
  → [Matter/Delegated] Close()
  → [HAP] 가상 스위치 순간 On
  → HomeKit 자동화 발동 → Matter 기기 동작
```

## 7. 설정 스키마 (`configs/devices.yaml`)

```yaml
zigbee:
  port: /dev/ttyUSB0          # Zigbee coordinator (CC2652 계열)

mqtt:
  listen: ":1883"             # 내장 브로커

devices:
  - {id: switch_1, integration: zigbee, addr: "0x00158d0001abcd01", type: switch}
  - {id: fan_1,    integration: mqtt,   addr: "home/room/fan",      type: fan}
  - {id: cover_1,  integration: mqtt,   addr: "home/room/curtain",  type: cover}

  # Matter 기기: 현재 HomeKit 위임. 가상 트리거 스위치만 노출.
  - id: blind_1
    integration: matter
    driver: delegated           # 나중에 → go-matter
    triggers:                   # HAP 가상 스위치 ↔ HomeKit 자동화 (1회 수동 생성)
      close: blind_close
      open:  blind_open
```

## 8. Matter 이음새 (seam)

Matter 기기는 `matter` 통합으로 모델링하되 **드라이버 구현만 교체**한다.

```go
// internal/matter
type Driver interface {
    Open() error
    Close() error
    SetLiftPercent(p int) error   // 0..100
    LiftPercent() (int, error)    // 상태 읽기
}
```

- **현재: `DelegatedDriver` (HomeKit 위임)**
  - `Open/Close` → HAP 가상 스위치를 순간 On → HomeKit 자동화 발동.
  - `SetLiftPercent/LiftPercent` → `ErrUnsupported`.
  - 전제: HomeKit 측에 트리거 자동화를 1회 수동 생성.
- **구현됨: `GoMatterDriver` (`go-matter` 기반)**
  - 동일 인터페이스를 실제 Matter(CASE 세션)로 구현 → 기기가 허브 소유의 first-class `Cover`가 됨(위치·상태 자유).
  - config `driver: go-matter` + `gomatter{ fabricStore, nodeId, address?, endpoint }` 로 활성화. `address` 생략 시 node id를 mDNS로 resolve.
  - 상태는 전용 세션 **Subscribe**(푸시)로 반영하고, 미지원 기기는 30초 **폴링**으로 폴백.
  - 기존 커미셔닝(chip-tool 등)으로 fabric에 온보딩된 기기가 전제(자체 커미셔닝은 미구현, §9 M6).

> HomeKit 컨트롤러는 서드파티용 inbound 제어 API를 제공하지 않는다. 따라서 위임 드라이버는 **단방향 트리거**가 한계이며, 자유 제어는 `go-matter` 네이티브 드라이버로 달성한다.

## 9. `go-matter` — 자체 Matter 컨트롤러 (from scratch)

허브 코드와 분리된 **독립 Go 모듈**. `matter.Driver`를 통해서만 허브에 연결되므로 완성 전까지 허브는 위임 스텁으로 동작한다.

**구성 방침**
- 전 계층을 **0에서 신규 구현**한다(외부 Matter 스택 차용 없음). 의존성은 표준 라이브러리 + `golang.org/x/net`(mDNS wire) + `filippo.io/nistec`(P-256)로 제한.
- 스펙 해독은 `connectedhomeip(CHIP)` · `matter.js`를 레퍼런스로, 각 프리미티브는 **공개 테스트 벡터**(RFC 3610/5869/7914/9383, 스펙 부록, 실제 CHIP 인증서)로 바이트 단위 검증한다.

**마일스톤**

| M | 내용 | 상태 |
|---|---|---|
| M0 | `chip-tool`로 커미셔닝 + 자격증명 추출 (오라클) | 전제(수동) |
| M1 | operational mDNS 디스커버리 (`_matter._tcp`) + resolve | ✅ |
| M2 | **CASE 세션** (Sigma1/2/3, P-256/HKDF/TLV cert/AES-CCM) | ✅ |
| M3 | 보안 메시지 레이어 (AES-CCM AEAD) | ✅ (MRP 재전송은 단순화) |
| M4 | TLV + Interaction Model **Invoke/Read** (OnOff·WindowCovering·LevelControl) | ✅ |
| M5 | 속성 **Subscribe** → 푸시 상태 | ✅ (setup + 스트리밍) |
| M6 | 자체 커미셔닝 (PASE/SPAKE2+ + DAC 검증) | ⬜ (SPAKE2+ 프리미티브만 완료) |

**크립토 주의**
- **AES-CCM**은 Go 표준 라이브러리에 없음 → 외부 라이브러리 또는 직접 구현.
- **SPAKE2+** 표준 라이브러리 없음 → 스펙 기반 직접 구현(M6).
- Matter 인증서는 **compact-TLV** 포맷(≠ 평문 X.509 DER) → 자체 인코딩 필요.

**시퀀싱** — operational 제어(M1~M5)를 먼저 완료했고, 자체 커미셔닝(M6)은 뒤로 둔다. 현재는 chip-tool로 커미셔닝한 기기를 `go-matter`가 resolve→CASE→Invoke/Read/Subscribe 한다. 하드웨어 실검증(실제 blind)은 남아 있다.

## 10. 배포

`deploy/home-hub.service`:

```ini
[Unit]
Description=Home Hub
After=network-online.target

[Service]
ExecStart=/opt/home-hub/hub --config /opt/home-hub/devices.yaml
Restart=always
RestartSec=5
User=homehub          # 시리얼 코디네이터 접근 위해 dialout 그룹 필요

[Install]
WantedBy=multi-user.target
```

크로스컴파일: `GOOS=linux GOARCH=arm GOARM=7 go build -o hub ./cmd/hub` → 바이너리 하나 배포.

## 11. 로드맵

| 단계 | 내용 | 완료 기준 | 상태 |
|---|---|---|---|
| 0 | 스켈레톤: domain + bus + registry + config | 기기 목록 로드/로그 | ✅ 완료 |
| 1 | Zigbee + HAP: 스위치 1개 On/Off | HomeKit에서 제어 | ✅ 코드 완료 (HW 검증 대기) |
| 2 | Zigbee 다기기 + 물리버튼 상태 반영 | 양방향 동기화 | ⬜ |
| 3 | 내장 MQTT + ESP32 연동 | HomeKit에 등장 | ⬜ |
| 4 | 추가 ESP32 기기(팬 등) | 제어 | ⬜ |
| 5 | Automation 엔진 + Matter 위임 트리거 | 규칙 동작 | 🔶 배선 완료 |
| 6 | systemd 배포 + 안정화 | 상시 구동 | ⬜ |
| (병렬) | `go-matter` operational 제어 (M1→M5) | 위임→네이티브 스왑 + 구독 상태 | ✅ 코드 완료 (HW 검증 대기) |

**Stage 1 상태**: HAP 브리지(brutella/hap)와 Zigbee 코디네이터(shimmeringbee/zstack) 연동 코드가 컴파일·단위테스트를 통과했다. 실제 On/Off 동작은 Zigbee 동글 + Aqara 스위치 + iPhone 페어링으로 검증이 남아 있다. Matter 위임(가상 트리거)과 automation 규칙 배선은 완료되어 어댑터 위에서 바로 흐른다.
