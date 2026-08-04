# 배포 롤아웃 계획 (하드웨어 도입 순서)

실제 기기(Aqara H2, 삼성 가전, 거실 실링팬, 안방 조명일체형 실링팬)가 준비되는 대로
아래 순서로 연동한다. **코드는 대부분 완료**되어 있고, 각 단계는 주로 "기기 페어링 +
config에 등록 + HomeKit 연동" 작업이다.

역할 정리:
- **home-hub (이 Go 바이너리)** — Zigbee/MQTT/Matter를 하나로 브리지하고, 모든 기기를
  HAP(HomeKit) 액세서리로 노출한다.
- **Apple TV** — HomeKit *home hub*(외부 원격 제어 + 자동화 실행) 겸 Thread 보더라우터.
  우리 허브 코드가 특별히 할 일은 없음(HAP만 서빙하면 iPhone/Apple TV가 알아서 붙는다).
- **iPhone** — Home 앱으로 허브 브리지를 페어링하면 모든 기기가 나타난다.

---

## Phase 1 — 조명 먼저 (iPhone + Apple TV) ← **가장 먼저**

**대상:** 거실/작업방 간접·일반 조명 (Aqara H2 Zigbee 스위치의 릴레이에 물린 다운라이트),
안방 조명일체형 실링팬의 **조명 부분**.

**전제:**
- Zigbee 코디네이터 동글(CC2652 계열, 예: Sonoff ZBDongle-P) 연결.
- Aqara H2 스위치 물리 설치 + 조명 배선.
- 안방 실링팬은 조명/팬을 각각 켜려면 RF 브리지(ESP32)가 필요 → 조명만 먼저면
  H2 릴레이 직결 조명부터. (실링팬 조명이 RF-only면 Phase 3과 함께)

**절차:**
1. `configs/*.yaml`에서 `zigbee.permitJoin: true`로 두고 허브 실행 → H2를 페어링 모드로.
2. 조인 로그(`zigbee node joined ieee=0x...`)에서 **IEEE 주소** 확보.
3. 그 주소로 config에 조명 디바이스 등록 (2-gang이면 `endpoint: 1`, `endpoint: 2` 두 개).
   시작 템플릿: **`configs/phase1-lights.yaml`**.
4. `permitJoin: false`로 되돌리고 재시작.
5. iPhone Home 앱 → 액세서리 추가 → 허브 PIN 입력 → 조명들이 등장. Apple TV가
   자동으로 home hub가 됨(원격 제어/자동화 가능).

**이 단계의 코드 상태: ✅ 완료** — Zigbee OnOff 제어/상태반영, 멀티갱 endpoint,
HAP Lightbulb 노출, 파일 영속화(재시작해도 재페어링 불필요). 조명은 **on/off만**
필요(단순 다운라이트, 디밍/안정기 없음).

> H2를 이 단계에선 **coupled(기본)** 모드로 둔다 = 패들이 곧바로 자기 릴레이(조명)를
> 켠다(허브가 꺼져도 물리 스위치는 동작). 패들로 *다른* 기기를 제어하려면(Phase 2+)
> 그 스위치만 `decoupled: true`로.

---

> ⚠️ **2026-09-03 정정.** 아래 Phase 1·2는 "H2 = Zigbee, 전동커튼 = Zigbee"라는 **실물 확인 전
> 추정**으로 쓰였고, 둘 다 틀렸다.
> - **H2는 Zigbee/Thread 듀얼이며 Thread로 출고된다.** 거실 스위치는 **Matter로 Apple Home에
>   직접** 붙였다 — 허브를 거치지 않는다. 자세한 건 `docs/switch-strategy.md`.
> - **전동커튼은 헤이홈(Hejhome) 앱으로 동작 중**이다. Zigbee라는 근거가 없다. 헤이홈이
>   Matter 브리지를 제공하면 Apple Home 직행이고, Tuya OEM이면 `docs/tuya-protocol.md`의
>   LAN 3.5 어댑터를 재사용한다. 집 랜에서 Tuya UDP 브로드캐스트 스캔으로 판별 가능.
> - 결과적으로 **현재 확정된 Zigbee 기기가 하나도 없다.** EZSP 백엔드는 보류 상태다
>   (`docs/ezsp-backend.md`).
>
> 허브가 대체 불가능한 영역은 **HomeKit이 모르는 프로토콜**뿐이다 — RF 447MHz, BLE FanLamp,
> Tuya IR, SmartThings. Matter 기기는 허브 없이 Apple Home에 직행한다.

## Phase 2 — 전동블라인드(Matter) + 전동커튼(Zigbee)

**대상:** 작업방 전동블라인드(Matter), 거실 전동커튼(Zigbee 커튼모듈).

**전제:**
- 블라인드: Apple Home에 이미 붙어 있으면 → 우리 허브가 직접 소유하려면 **커미셔닝**
  필요. go-matter가 자체 커미셔닝(PASE→NOC 발급→CASE) 코드까지 완료(실HW 검증만 남음).
  또는 초기엔 `driver: delegated`(HomeKit 위임 트리거)로 단방향 제어.
- 커튼: Zigbee 커튼모듈을 H2와 같은 방식으로 페어링.

**절차:**
- 커튼: 조인 → IEEE 확보 → `type: cover`로 등록. HomeKit에서 위치(%) 제어.
- 블라인드(네이티브): 페어링 코드(QR/수동)로 커미셔닝 → `driver: go-matter` +
  `gomatter{fabricStore, nodeId, endpoint}` 등록. 주소 생략 시 mDNS로 resolve.

**코드 상태: ✅ 완료** — Zigbee WindowCovering(위치·방향반전), go-matter 네이티브
제어 + 구독 + 재접속 + 자체 커미셔닝. 실기기에서 **블라인드 방향** 최종 확인
(반전은 드라이버 경계 한 곳에서 뒤집으면 됨).

---

## Phase 3 — 실링팬 (RF → ESP32/MQTT)

**대상:** 거실 실링팬, 안방 조명일체형 실링팬(조명+팬).

**전제:** ESP32 + RF 송신기, 리모컨 RF 코드 캡처.

**절차:**
- ESPHome로 ESP32를 MQTT 브리지로 구성(`examples/esp32/ceiling_fan.yaml`).
- **조명일체형은 논리 디바이스 2개**(`type: light` + `type: fan`, 다른 addr)로 등록 →
  조명과 팬을 각각 제어.
- 벽 버튼으로 팬/조명 토글하려면 그 H2를 `decoupled: true` + `button` 규칙.

**코드 상태: ✅ 완료(허브)** — MQTT 브로커, 팬 속도(RotationSpeed), 2-디바이스 모델,
button/threshold 규칙. 남은 건 ESP32 펌웨어 + RF 코드 캡처(하드웨어 작업).

---

## Phase 4 — 삼성 가전 (에어컨 등)

**대상:** 시스템 에어컨(DVM 천장 카세트 등), 기타 삼성 가전.

**옵션 (SmartThings 로컬 인바운드 API 없음):**
- IR 블래스터(Broadlink RM4) 또는 ESP32 IR → MQTT (가장 단순).
- 삼성 Wi-Fi 키트(MIM-N10) / NASA 버스 탭 → ESPHome → MQTT (더 정교, 상태 읽기).
- (선택) SmartThings 클라우드 어댑터 — 별도 구현 필요, 토큰 필요.

**코드 상태: ⬜ 미구현.** MQTT/IR 경로면 기존 mqtt 어댑터 재사용 가능(온/오프/모드 토픽 규약만
정하면 됨). 클라우드 어댑터는 새 `internal/<adapter>` 필요.

---

## 이미 준비된 것 (하드웨어 전 완료)

- Zigbee(멀티갱·커튼·Aqara 버튼/decoupled), MQTT 브로커(addr 기반 토픽), HAP 브리지
  (조명·팬속도·커버·온도·습도 센서), go-matter 네이티브 제어 + 자체 커미셔닝,
  automation(mirror/button/threshold), 파일 영속화, systemd 유닛(`deploy/`).
- 시작용 최소 설정: **`configs/phase1-lights.yaml`**.
- 빌드/검증: `go build ./... && go test ./...` (양 모듈 그린, -race 클린).

## 실기기 도입 시 확인할 것 (코드 아님, 실측)

- Aqara H2: 조인 후 IEEE 주소, 2-gang endpoint 번호, (decoupled 쓸 때) 버튼 값 1/2/0.
- 전동블라인드: HomeKit 0=닫힘 vs Matter 0=열림 방향 최종 확인.
- 실링팬: 리모컨 RF 코드, 팬 속도 단계 매핑.
- 삼성 에어컨: 제어 경로(IR/Wi-Fi 키트/NASA) 결정.
