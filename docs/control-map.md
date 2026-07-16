# 통합 제어 맵 (Control Map)

> 집 전체 기기를 어떤 경로로 제어하고 HomeKit에 어떻게 노출하는지의 마스터 정리.
> 방별 스위치는 `docs/switch-strategy.md`, 안방 실링팬은 `docs/bedroom-fan-light.md`,
> 도입 순서는 `docs/ROLLOUT.md` 참고.

## 제어 경로 한눈에

| 기기 | 위치 | 제어 경로 | 하드웨어 | HomeKit 노출 | 상태 |
|---|---|---|---|---|---|
| Aqara H2 조명 5개 | 거실2·주방·안방·작업방 | Zigbee | **Zigbee 동글** | Lightbulb | 코드완료 · 동글 필요 |
| 안방 조명실링팬 | 안방 | **BLE**(FanLamp Pro)→ESP32→MQTT | **ESP32 #1** | Light + Fan | 코드완료 · ESP32+variant |
| 벽 L2/L3 (안방) | 안방 | Zigbee decoupled → 규칙 → MQTT | (H2) | — | **cycle 규칙 구현 필요** |
| 거실 로슬러 실링팬 | 거실 | **RF**→ESP32→MQTT | **ESP32 #2 + RF모듈** | Fan (+조명?) | RF 캡처+예제 필요 |
| 거실 무빙쇼파 | 거실 | **RF**→ESP32→MQTT | ESP32 #2 + RF모듈 | 스위치/커버(확인) | RF 캡처+모델링 |
| 삼성 에어컨 ×2 | — | **SmartThings** 클라우드 | **`internal/smartthings`(신규)** | HeaterCooler | **결정: 어댑터 구축, AC 먼저** |
| 삼성 가전(세탁·건조·식세·냉장×2·청소) | 집 전체 | SmartThings | (추후 ST 어댑터 확장) | 제한적 | **추후** (지금은 ST 앱) |
| 작업방 전동블라인드 | 작업방 | Matter (go-matter) | Apple TV/native | Cover | 코드완료 |
| **외부/내부 원격** | — | **Apple TV = HomeKit home hub** | Apple TV 4K(보유) | 전체 | **설정 필요** |

## 하드웨어 쇼핑 리스트

1. **Zigbee 코디네이터 동글 ×1** — CC2652(예: Sonoff ZBDongle-P). 조명 전체의 전제.
2. **ESP32-WROOM-32 ×2** — #1 안방(BLE 전용, 모듈 없음), #2 거실(RF).
3. **433MHz RF 송신+수신 모듈 1세트** — 거실 ESP32용. 리모컨 라벨이 없어 주파수 미상 →
   **433 우선**, RX로 안 잡히면 315MHz. 정 애매하면 CC1101(가변).
4. **Apple TV 4K** — 이미 보유. 구매 아님, 설정만.

> 안방 BLE는 ESP32 내장 라디오라 **RF 모듈 불필요**. RF 모듈은 **거실(로슬러 팬 + 쇼파)** 에만.

## 외부/내부 원격 제어 → Apple TV 필수

iPhone으로 **집 밖에서도** 제어하려면 Apple TV 4K가 **HomeKit home hub** 역할을 해야 한다:
1. Apple TV를 집 **iCloud(홈) 계정**으로 로그인 → Home 앱에서 "홈 허브 연결됨" 확인.
2. 그러면 외부에서 iPhone Home 앱 제어 + 집 비었을 때 자동화 실행 + Thread(블라인드)까지 커버.
3. **허브(Go) 코드는 변경 없음** — HAP만 서빙하면 Apple TV가 알아서 게이트웨이.

## 삼성 SmartThings (에어컨 2대 + 가전들)

**대응 카테고리 문제:** HomeKit엔 세탁기/건조기/식기세척기/냉장고/청소기에 맞는 액세서리
카테고리가 **없다**. 억지로 스위치로 노출해도 UX가 나쁨 → 이들은 **SmartThings 앱을 그대로**
쓰는 걸 권장(SmartThings 자체가 클라우드라 외부 원격도 이미 됨).

**결정(2026-07-22): SmartThings 클라우드 어댑터 구축 — 에어컨 먼저, 나머지 가전은 추후.**

### `internal/smartthings` 어댑터 설계 (신규, Phase 4)
- **인증:** SmartThings **PAT**(Personal Access Token) 또는 OAuth. ⚠️ 2024말부터 신규 PAT는
  스코프 제한/단기 만료 정책 → 발급 시 현재 정책 확인. 개인용이면 PAT가 가장 단순.
- **API:** `GET /v1/devices`(목록), `GET /v1/devices/{id}/status`(전체 상태),
  `POST /v1/devices/{id}/commands`(명령). 로컬 허브는 NAT 뒤라 웹훅 구독이 어려움 →
  **폴링**(status N초마다)로 시작.
- **AC capability → HAP HeaterCooler 매핑:**
  `switch`→Active, `airConditionerMode`(cool/heat/auto/dry/wind)→TargetHeaterCoolerState,
  `coolingSetpoint`/`thermostatCoolingSetpoint`→CoolingThresholdTemperature,
  `temperatureMeasurement`→CurrentTemperature, `airConditionerFanMode`→RotationSpeed,
  (지원 시) swing→SwingMode.
- **선행 필요:** (1) homekit 어댑터에 **HeaterCooler 서비스 추가**(현재 Light/Fan/Cover/Sensor만),
  (2) 사용자 **ST 토큰** 발급, (3) `GET /devices`로 **에어컨 deviceId + capability** 확인.
- **config 스케치:**
  ```yaml
  smartthings:
    token: !env ST_TOKEN
    poll: 30s
  devices:
    - {id: ac_living, name: "거실 에어컨", integration: smartthings,
       addr: "<st-device-uuid>", type: ac}
    - {id: ac_bedroom, name: "안방 에어컨", integration: smartthings,
       addr: "<st-device-uuid>", type: ac}
  ```
- 코드는 하드웨어 없이 **mock HTTP로 단위 테스트 가능** → 토큰 오기 전 스캐폴딩 선행 OK.

## 궁극 목표(확정 2026-07-22): "커스텀 home-hub = 단일 통합 서버, Apple Home으로 외부까지"

**home-hub(Go)가 THE 단일 통합 서버.** ST·HomeKit·Zigbee·BLE·Matter·MQTT를 전부 어댑터로
통합한다. UI/제어/원격/자동화 = **Apple Home + Apple TV**. **웹 대시보드 불필요**(사용자 확인).
**제품 Home Assistant는 도입 안 함**(그 경우에만 허브 무게중심이 HA로 이동 — 해당 없음).

**핵심 구분 — 통합(integration) vs 표현(presentation):**
- **통합(허브):** 허브는 모든 프로토콜을 말할 수 있어 **모든 기기 상태를 안다.** ✓ 목표 달성.
- **표현(Apple Home):** 허브가 HomeKit(HAP)으로 노출 → **HomeKit 카테고리는 Apple이 고정.**
  - **카테고리 있음** → Apple Home 완전 제어·자동화·외부원격: 조명, 안방 BLE 팬/조명,
    거실 RF 팬/쇼파, **에어컨(HeaterCooler)**, 블라인드, 커튼, 센서, 스위치. → 제어 대상 전부 포함.
  - **카테고리 없음** → Apple Home 표시 불가(세탁·건조·식세·냉장·청소). **Apple의 한계**(허브 아님).

**최종 그림:**
- 제어 대상 대부분 → HomeKit → **Apple Home 제어·자동화·외부원격.** 웹페이지 불필요. ✓
- 백색가전 → **SmartThings 앱 병용**(HomeKit 카테고리 없음).
- 에어컨 → ST 어댑터로 HomeKit HeaterCooler 노출.
- (선택/해킹) 백색가전 "가동중"만 Apple Home에 → occupancy/contact 센서 프록시(제어X).

**Apple Home 자동화로 충분?** → 예. HomeKit 노출 기기는 Apple Home 자동화로 제어, Apple TV가
홈허브라 외부 실행까지. 별도 서버 UI 불필요.

## 거실 무빙쇼파 — 모델링 확인 필요

리모컨 기능(등받이/발받침 up·down, 프리셋 등)을 먼저 파악 → HomeKit엔:
- 단순 up/down이면 **Cover**(위치%)로, 개별 모터 여러 개면 **스위치 여러 개**로.
- RF 캡처 시 각 버튼 코드 기록.

## 남은 확인/결정

- [ ] **삼성 에어컨** 경로 결정: ST 어댑터(A) / Matter 지원 확인(B) / 앱 유지(C).
- [ ] 나머지 삼성 가전: SmartThings 앱 유지(권장) 확정.
- [ ] **RF 주파수**(로슬러 팬·쇼파): 433 우선 캡처.
- [ ] **무빙쇼파 리모컨 기능** 목록 → HomeKit 모델(cover/switch) 결정.
- [ ] 로슬러 팬: 조명 유무, 속도 단수.
- [ ] 안방 벽 **L2/L3 cycle 규칙** 구현(조명 states는 BLE 엔티티 확정 후).
- [ ] Zigbee 동글 · ESP32×2 · RF모듈 구매.
