# 안방 조명일체형 실링팬 제어 설계 (BLE / FanLamp Pro)

> **제어 방식 확정: BLE (앱 = FanLamp Pro).** RF가 아니라 BLE 컨트롤러 내장 유닛이다.
> ESP32(BLE 내장) + ESPHome `ble_adv_controller`가 앱을 그대로 흉내내고, 결과를 허브의
> MQTT 규약으로 노출한다. **RF 송·수신 모듈·납땜·코드 캡처 불필요.** 상위 순서는
> `docs/ROLLOUT.md`, 방별 전략은 `docs/switch-strategy.md` 참고.

## 1. 확정된 사실 (관찰 + 앱)

- 유닛은 **BLE 컨트롤러 내장** (아이폰 **FanLamp Pro** 앱으로 조명·팬 독립 제어됨).
- **조명** = **L1 전원 필요** — L1이 꺼지면 앱으로도 조명이 안 켜짐 (조명은 L1 스위치 회로).
- **팬** = **L1과 무관하게 항상 동작** — 팬 + BLE 모듈은 **상시전원**.
- FanLamp Pro는 BLE **advertising 기반** 프로토콜(연결 없이 광고 패킷 전송)이라, ESPHome
  커뮤니티가 리버스 엔지니어링한 `ble_adv_controller`(NicoIIT)가 `encoding: fanlamp_pro`로
  지원한다.

## 2. 아키텍처

```
[iPhone Home] ─┐
[Apple TV]     ├─ HAP ─▶ [home-hub] ─ MQTT(home/bedroom/light|fan) ─▶ [ESP32]
[벽 L2/L3(선택)]┘                                                         │ BLE advertising
                                                                          ▼
                                                          [실링팬 FanLamp Pro 컨트롤러]
                                                              조명(L1 전원 필요) / 팬(상시)
```

- ESP32는 **RF 모듈 없이 내장 BLE**로 광고 패킷을 쏜다. 안방에 USB 전원으로 상주.
- 허브 MQTT 어댑터는 이미 완성 → ESP32가 `<addr>/set` 구독, `<addr>/state` 발행만 맞추면 됨.

## 3. 필요 하드웨어 (최소)

- **ESP32 (BLE 내장) 1개** — `ESP32-WROOM-32 DevKitC`(권장) / C3 / S3. **ESP32-S2 금지(BLE 없음).**
- **그게 전부.** RF 송·수신 모듈, 아두이노, 납땜 불필요.
- BLE 사거리상 ESP32는 **안방에** 둔다(USB 전원).

## 4. Aqara H2(4-gang) 역할 — BLE 기준으로 단순화

| 게이트 | 모드 | 역할 |
|---|---|---|
| **L1** | coupled(기본) | **조명 전원**. 벽 스위치로 유지. 조명은 L1 ON일 때만(앱/BLE도 동일). 일괄소등 포함. |
| **L2** | (선택) decoupled | 벽에서도 조명 제어하고 싶으면 버튼으로 → 허브 → BLE. 안 쓰면 미사용. |
| **L3** | (선택) decoupled | 벽에서 팬 제어용. 안 쓰면 미사용. |
| **L4** | 여유 | — |

> **결정: 벽 L2/L3도 사용.** L2=조명 순환, L3=팬 순환. `cycle` 규칙 **구현 완료**(빌드+테스트
> green, `internal/automation/rules.go` CycleRule). config:
>
> ```yaml
> # L2: 조명 순환 off→3단(50)→6단(100)→off, L1 전원 끊기면 리셋
> - {type: cycle, src: bed_btn_light, press: single, dst: bed_ceiling_light, states: [0, 50, 100], power: bed_fan_power}
> # L3: 팬 순환 off→3단→6단→off
> - {type: cycle, src: bed_btn_fan,   press: single, dst: bed_ceiling_fan,   states: [0, 50, 100], power: bed_fan_power}
> ```
>
> states는 레벨(0=off). 조명 "1번/2번"이 밝기 2단인지 색온도 2종인지는 페어링 후 확정 —
> 색온도 순환이 필요하면 states 확장(현재는 레벨 순환).

## 5. HomeKit 노출 — 구현됨(디바이스 `features`로 opt-in)

조명이 W/Y/N(색온도) + G/B/R(RGB) + 밝기까지 되는 풀컬러라, HomeKit도 그에 맞춰 노출한다.
단순 다운라이트(거실 등)에 슬라이더가 붙지 않도록 **디바이스별 `features`로 선택**한다:

```yaml
- {id: bed_ceiling_light, integration: mqtt, addr: home/bedroom/light, type: light,
   features: [brightness, colortemp, color]}   # On + 밝기 + 색온도 + RGB(Hue/Sat)
- {id: bed_ceiling_fan,   integration: mqtt, addr: home/bedroom/fan,   type: fan,
   features: [direction]}                       # On + RotationSpeed(6단) + 정/역회전
```

| 앱 기능 | HomeKit | feature |
|---|---|---|
| 조명 on/off | Lightbulb.On | (기본) |
| 조명 밝기 | Brightness | `brightness` |
| 조명 W/Y/N 색온도 | ColorTemperature(mireds) | `colortemp` |
| 조명 G/B/R 색상 | Hue + Saturation | `color` |
| 팬 on/off·속도 | On + RotationSpeed | (기본) |
| 팬 정/역회전 | RotationDirection | `direction` |
| 취침/무드등 모드 | (HomeKit 카테고리 없음) | — Apple Home 씬/자동화로 대체 |

> **단서:** ① 위 매핑은 코드에 구현됨(빌드+테스트 green). 실제 조명이 각 기능을 지원하는지는
> **페어링 후 확정**(ble_adv_controller가 그 variant에서 밝기/CCT/RGB를 노출하는지). ② 취침·무드등
> 같은 앱 전용 동적 모드는 HomeKit에 대응 개념이 없어 씬/자동화로 흉내. ③ BLE는 단방향이라
> HomeKit은 **마지막 명령 상태**만 표시(리모컨/앱 조작은 미반영) — 허브 위주 조작 권장.
- (선택) **안방 실링팬 조명 전원(L1)** — 스위치. 조명 전원 자체를 홈킷에서 끄고 싶을 때.

## 6. ESP32 설정 (ESPHome) — 초안

전체 초안: **`examples/esp32/fanlamp_pro_ble.yaml`**. 요지:

```yaml
external_components:
  - source: github://NicoIIT/esphome-ble-adv-controller
ble_adv_handler:
ble_adv_controller:
  - id: fanlamp
    encoding: fanlamp_pro
    variant: v3          # ★ 페어링/리스닝으로 확정 (v1/v2/v3)
    duration: 200
light: [ {platform: ble_adv_controller, ble_adv_controller_id: fanlamp, ...} ]
fan:   [ {platform: ble_adv_controller, ble_adv_controller_id: fanlamp, ...} ]
```

허브 MQTT 규약(`home/bedroom/light|fan` `/set`·`/state`)으로의 변환은 기존 RF 예제와 같은
`on_json_message` 패턴을 재사용(자세한 건 예제 파일).

## 7. 페어링 & variant 확정 (유일한 실측 미지수)

FanLamp Pro의 세부 인코딩은 **v1/v2/v3** 중 하나다. 확정 절차:
1. ESP32에 위 설정 flash.
2. **variant 찾기(둘 중 하나):**
   - (간단) v3부터 시도 → 유닛 전원 방금 넣고 수 초 내 **Pair** 버튼 실행 → 조명 반응하면 성공.
     반응 없으면 v2, v1 순으로.
   - (정확) ESP32를 리스닝 모드로 두고 **FanLamp Pro 앱에서 버튼**을 누르면 컴포넌트가 광고
     패킷을 디코드해 **encoding/variant/forced_id**를 로그로 알려줌 → 그대로 설정에 박음.
3. variant 확정 후 조명/팬 각각 on/off·속도 검증.

## 8. 남은 확인 사항

- [ ] FanLamp Pro **variant(v1/v2/v3)** — 페어링/리스닝으로 확정.
- [ ] 조명이 **조광/색온도** 지원 모델인지(단순 on/off인지) → HomeKit 밝기/CCT 노출 결정.
- [ ] 팬 **6단** BLE 매핑 확인, 방향(정/역회전) 지원 여부.
- [ ] L1 OFF→ON(전원복구) 시 조명 BLE 상태 재동기 필요 여부(팬은 상시라 무관).
- [ ] 벽 L2/L3를 실제로 쓸지(쓰면 decoupled + button 규칙 추가).
- [ ] **나머지 리모컨 4개**: 각각 전용 앱(BLE) 있는지 → 있으면 같은 방식, 없으면 RF 별도 검토.

## 부록 A — RF 폴백 (앱 없는 다른 기기용)

앱이 없는 순수 RF 기기(다른 리모컨 중 일부)는 여전히 RF 경로가 필요하다: ESP32 + 433MHz
송·수신 모듈(주파수 라벨 없으면 **433 우선**, 안 되면 CC1101 가변), `rc-switch`로 캡처 →
ESPHome `remote_transmitter`. 이 경우만 `examples/esp32/ceiling_fan.yaml`(RF 예제) 사용.
