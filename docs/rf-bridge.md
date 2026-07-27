# RF 브리지 연동 (거실 실링팬 + 안방 블라인드)

> 근거 리포: [zobithecat/rf-remote-analyzer](https://github.com/zobithecat/rf-remote-analyzer) —
> SDR로 두 리모컨의 447 MHz 프로토콜을 해독/검증하고, ESP32+CC1101 펌웨어(`esp32/rf_bridge`)로
> 전파 재현까지 SDR 대조로 비트 단위 확인을 끝낸 상태.

## 전체 그림

```
iPhone / Siri
    │ HomeKit (HAP)
home-hub (Go)  ── internal/rf ──▶ 내장 MQTT 브로커 (mochi-mqtt, :1883)
                                        │ WiFi
                              ESP32 + CC1101 (rf_bridge 펌웨어)
                                        │ OOK 447 MHz
                          거실 실링팬 수신기 / 안방 블라인드 수신기
```

- **프로토콜 두뇌는 펌웨어에 남긴다.** 펄스 타이밍(259 µs 단위), 팬 프레임 생성
  (33비트: 주소+버튼+누름 카운터+패리티), 블라인드 코드북 재생은 전부 ESP32가 한다.
  하드웨어 검증(SDR 대조 비트일치)이 끝난 코드라 허브로 옮길 이유가 없고, 팬의
  누름 카운터는 펌웨어 NVS에 단일 저장소로 유지돼야 한다(시리얼 콘솔로 눌러도 동기 유지).
- **허브(Go)는 의미 변환만 한다.** `internal/rf` 어댑터가 HomeKit 명령(위치·속도)을
  펌웨어의 MQTT 토픽으로 바꾼다. 펌웨어는 무수정 — HA 디스커버리 토픽을 같이
  발행하지만 허브는 무시하므로 무해하다.

## 펌웨어 토픽 (rf_bridge.ino 그대로)

| 토픽 | 페이로드 | 동작 |
|---|---|---|
| `rf447/blind/<ch>/set` | `OPEN` / `CLOSE` / `STOP` | 블라인드 완전 올림/내림/정지 |
| `rf447/blind/<ch>/step_up`·`step_down` | 아무거나 | 한 칸 이동 (허브 미사용, 수동용) |
| `rf447/fan/<n>/press` | 아무거나 | 팬 리모컨 버튼 n(1..15) 누름 |
| `rf447/status` | `online`/`offline` (LWT) | 브리지 생존 여부 |

`rf447`은 펌웨어의 `DEVICE_ID` = config의 `addr`. **슬래시 없는 단일 세그먼트**여야
한다(펌웨어가 `%*[^/]/blind/...`로 파싱).

## 실기 검증 결과 (2026-08-15, 거실 실링팬)

| 확인한 것 | 결과 |
|---|---|
| CC1101 하드웨어 | `PARTNUM=0x00 VERSION=0x14 OK` — SPI 정상, 펌웨어 기탑재 |
| 코드북이 **우리 집** 리모컨 것인가 | ✅ 실물 리모컨 수신 캡처의 주소 `10101000100111011001`가 `kFanAddress`와 일치, 패리티 4식도 성립 |
| 브리지 송신 → 팬 반응 | ✅ 정지 상태에서 `fan 1`로 기동, `fan 7`로 정지 |
| 누름 카운터 동작 | ✅ 같은 버튼 4회(카운터 4·5·6·7)가 **각각 개별 누름**으로 인식됨 |
| WiFi + 허브 MQTT 연결 | ✅ ESP32 192.168.0.112 → 허브 192.168.0.107:1883 |

### 거실 실링팬 버튼 맵 (물리 순서 = 코드북 번호)

| 번호 | 기능 | 번호 | 기능 |
|---|---|---|---|
| 1~6 | 풍속 1~6 (**1·6 실측**) | 10 | 정방향 |
| 7 | 전체 종료 (**실측**) | 11 | 자연풍 |
| 8 | 회전 종료 | 12 | 역방향 |
| 9 | LED — 이 팬은 조명이 없어 무동작 | 13~15 | 타이머 1H / 2H / 4H |

> 조명이 없으므로 "전체 종료"와 "회전 종료"는 실질적으로 같다 → `off: 7`.
> 9번(무동작)은 **팬 상태를 바꾸지 않고 카운터만 올리는** 용도로 쓸 수 있다.

## 허브 config

`configs/rf-test.yaml`이 실기 검증용 최소 설정이다(RF 기기 2개만).

```yaml
devices:
  - id: living_ceiling_fan
    name: "거실 실링팬"
    integration: rf
    type: fan
    addr: rf447
    rf:
      buttons:              # 리모컨 버튼 번호 1..15
        off: 7
        speeds: {17: 1, 33: 2, 50: 3, 67: 4, 83: 5, 100: 6}  # 6단
  - id: bedroom_blind
    name: "안방 블라인드"
    integration: rf
    type: cover
    addr: rf447
    rf:
      channel: 0            # 블라인드 리모컨 16채널 중 몇 번인지 (미확인)
```

### 명령 매핑 (internal/rf)

- **cover**: 위치 100 → `OPEN`(완전이동), 0 → `CLOSE`, **중간값 → `STOP`**.
  리모컨엔 위치 제어가 없으므로 "슬라이더를 중간에 두기 = 지금 멈춰"라는 규약.
  낙관적 상태로 드래그한 값을 되돌려줘 Home 앱이 "여는 중…"에 멈추지 않는다.
- **fan**: 속도% → `speeds`에서 가장 가까운 단(동률이면 위)으로 스냅해 해당 버튼
  누름. 0% 또는 끔 → `off` 버튼. 단순 "켬" → 최저단 버튼.
- **전송 페이싱**: 블라인드 완전이동은 전파를 ~3.6초 쏘고 펌웨어가 명령 간 4초
  간격을 요구한다. 허브가 기기별 **최신 명령만 남기는 큐**로 코얼레싱하고 전송 후
  쿨다운(팬 1s / 정지 1.5s / 완전이동 4s)을 지킨다. HomeKit 슬라이더 연타 안전.

## 펌웨어 (esp32/rf_bridge)

레포에 포함돼 있다(rf-remote-analyzer 원본 + 자격증명 분리만 수정).

```bash
cp esp32/rf_bridge/secrets.h.example esp32/rf_bridge/secrets.h   # 편집: WiFi + 허브 IP
arduino-cli lib install PubSubClient
arduino-cli compile --fqbn esp32:esp32:esp32 esp32/rf_bridge
arduino-cli upload  --fqbn esp32:esp32:esp32 -p /dev/cu.usbserial-10 esp32/rf_bridge
```

- `secrets.h`는 **git 제외**(비번 커밋 방지). 템플릿만 커밋된다.
- `MQTT_HOST`는 허브 IP 하드코딩 → 허브 IP가 바뀌면 재플래시. 공유기에서
  **DHCP 고정 할당**을 걸어두는 편이 낫다. (Pi 이전 시 반드시 재플래시)
- **다른 CC1101 개체로 교체하면 크리스털 보정 필수**(원본 리포 FLASH.md).
  현재 펌웨어의 `XTAL_HZ`는 지금 꽂혀 있는 이 모듈에 맞춰진 값이고, 실기에서
  송수신 모두 검증됐다.

## 셋업/디버깅 순서

1. 허브 실행: `go run ./cmd/hub --config configs/rf-test.yaml`
2. 브리지 연결 확인 (리셋 없이): `netstat -an | grep 1883.*ESTABLISHED`
   → ESP32 IP가 보이면 붙은 것.
3. 브리지 직접 시험(허브 우회, 전파만 확인):
   `python3 mqttpub.py <허브IP> rf447/fan/3/press` — 저장소 밖 임시 스크립트라
   없으면 아무 MQTT 클라이언트나 무방.
4. 블라인드 채널 확인: `rf447/blind/<0..3>/set` 에 `STOP`을 쏴서 반응하는
   채널을 찾아 `rf.channel`에 기입.

### 시리얼 콘솔 (WiFi 없이 단독 시험)

`fan <1-15>` / `blind <ch> <up|down|enter> [hold]` / `rx [fan|blind]` / `regs`.

⚠️ **포트를 열면 ESP32가 리셋된다.** 명령마다 새 세션을 열면 매번 재부팅되므로
여러 명령은 한 세션에서 연달아 보내야 한다. 실사용(상시 전원 + MQTT)에서는
리셋이 없어 해당 없음.

## 한계와 주의

- **송신 전용.** 실제로 움직였는지 허브는 모른다(리모컨과 동일한 한계).
  HomeKit 상태는 마지막으로 보낸 명령의 낙관적 반영이다.
- **팬 카운터 공유**: 수신기는 3비트 누름 카운터로 새 누름을 구분한다. 브리지와
  실물 리모컨을 번갈아 쓰면 ~1/8 확률로 한 번의 누름이 중복으로 무시될 수 있다 —
  한 번 더 누르면 됨. 프로토콜 고유 특성.
- **STX882/SRX882 경로는 이 두 기기엔 사용 불가**: 433.92 MHz 고정 SAW 모듈이라
  447.887/447.7234 MHz를 못 쏜다. `esphome/living-rf.yaml`(캡처 스테이지)은
  무빙쇼파 등 **다른 433 MHz 기기 캡처용으로만** 유효.
- **출력/대역 준수**: 펌웨어 기본 출력(≈3.2 mW, `PA_POWER=0x84`)을 올리지 말 것.
  팬의 447.887 MHz는 비신고 대역(447.6–447.85)을 살짝 벗어난다 — 리포 법적 고지 참조.
