# 방별 스마트 스위치 전략 (Aqara H2)

> 각 방 Aqara H2의 **역할(전략)**을 명문화한 문서. config(`configs/*.yaml`)는 이 전략에
> 1:1로 대응하도록 작성한다. 안방 실링팬 상세는 `docs/bedroom-fan-light.md`, 도입 순서는
> `docs/ROLLOUT.md` 참고.

## 전략(strategy) 용어

| 전략 | 뜻 | 허브 다운 시 | 용도 |
|---|---|---|---|
| **coupled-load** | 패들이 자기 릴레이(부하)를 직접 on/off. Aqara H2 기본 모드. | 정상 동작(물리 스위치) | 일반 다운라이트/간접등 |
| **decoupled-button** | 패들이 부하와 분리, 누르면 Zigbee **이벤트만** 발생 → 허브 규칙이 임의 기기 제어. | 패들 무동작 | RF 실링팬, 블라인드, 씬 |
| **decoupled-cycle** | decoupled + **순환 상태머신**(off→A→B→off). | 패들 무동작 | 실링팬 조명/팬 다단 순환 |

> 원칙: **로컬 폴백이 중요한 조명은 coupled**, 허브가 "번역"해야 하는(RF·Matter·다단) 것은
> **decoupled**. 한 H2 안에서 게이트마다 섞어 쓸 수 있다(안방이 그 예).

## 방별 전략 표

> gang 수·게이트별 배선은 **실측으로 확정**한다(아래 `?`/TBD). 확정되면 이 표와 config를
> 함께 갱신.

| 방 | H2 | gang | 게이트 전략 | HomeKit 노출 | Phase |
|---|---|---|---|---|---|
| **거실 A** | #1 | 2-gang(TBD) | L1 coupled=메인등, L2 coupled=간접등 | 전구 2 | 1 |
| **거실 B** | #2 | ?-gang(TBD) | L1 coupled=메인등, (L2 coupled=간접/보조 TBD) | 전구 1~2 | 1 |
| **주방** | #3 | ?-gang(TBD) | L1 coupled=주방등, (L2 TBD) | 전구 1~2 | 1 |
| **작업방** | #4 | ?-gang(TBD) | L1 coupled=작업방등, (L2 TBD). 블라인드는 **별개**(Matter) | 전구 + 블라인드 | 1 / 2 |
| **안방** | #5 | **4-gang** | L1 **coupled**=조명 전원(+일괄소등). 팬/조명 자체는 **BLE(FanLamp Pro)**로 제어(ESP32). L2/L3 decoupled는 "벽에서도 누르고 싶을 때"만 **선택**. L4 여유 | 조명 + 팬 (+선택 L1 전원 스위치) | 3 |

## 게이트별 규칙 매핑(개념)

- **coupled-load** → config에 그냥 `type: light` 디바이스로 등록(endpoint로 게이트 구분).
  자동화 규칙 불필요(패들이 곧 릴레이).
- **decoupled-button** → 그 게이트를 `decoupled: true` + `type: switch`로 등록하고,
  `rules:`에 `button`/`cycle` 규칙으로 대상 기기 제어.
- **decoupled-cycle** → `decoupled: true` + `rules:`에 `cycle`(신규 타입, `docs/bedroom-fan-light.md` §6).

## 안방 예시(확정) — config 스케치

> 제어는 **BLE(FanLamp Pro) → ESP32 → MQTT**. 조명/팬은 mqtt 디바이스로 등록(ESP32가
> BLE↔MQTT 변환). L1은 조명 전원(coupled)으로 유지. 상세는 `docs/bedroom-fan-light.md`.

```yaml
devices:
  # L1: 조명 전원(coupled). 허브는 상태만 읽어 전원복구 감지.
  - {id: bed_fan_power, name: "안방 실링팬 전원", integration: zigbee,
     addr: "0x00158d00xxxxxxx5", type: switch, endpoint: 1}
  # 조명/팬 = ESP32 BLE 브리지 (FanLamp Pro)
  - {id: bed_ceiling_light, name: "안방 실링팬 조명", integration: mqtt,
     addr: "home/bedroom/light", type: light}
  - {id: bed_ceiling_fan,   name: "안방 실링팬 팬",   integration: mqtt,
     addr: "home/bedroom/fan",   type: fan}

  # (선택) 벽에서도 제어하고 싶을 때만 L2/L3를 decoupled 버튼으로:
  # - {id: bed_btn_light, integration: zigbee, addr: "0x...5", type: switch, endpoint: 2, decoupled: true}
  # - {id: bed_btn_fan,   integration: zigbee, addr: "0x...5", type: switch, endpoint: 3, decoupled: true}

rules: []   # BLE+HomeKit로 직접 제어 시 규칙 불필요.
            # 벽 버튼을 쓰면 button/cycle 규칙 추가(docs/bedroom-fan-light.md §4).
```

## 실측으로 확정할 것 (방별 공통)

- [ ] 각 방 H2 **gang 수**와 게이트별 부하(메인/간접/기타).
- [ ] 각 H2의 **IEEE 주소**(페어링 조인 로그 `zigbee node joined ieee=0x...`).
- [ ] decoupled 게이트의 **버튼 이벤트 값**(single/double/hold) — 로그로 확인.
- [ ] 어느 방이 **일괄소등 회로**에 물려 있는지(안방 L1 확정, 나머지 확인).
- [ ] 작업방 블라인드 제어 경로(Matter 네이티브 vs HomeKit 위임).

## 현재 게이팅 하드웨어

- **Zigbee 코디네이터 동글**(CC2652, 예 Sonoff ZBDongle-P) — 조명 전체의 전제. **미연결**.
- ESP32 + RF 송·수신기 — 안방 실링팬 전용. 미보유.
- 동글/ESP32 도착 전에도 **HAP + MQTT mock**으로 소프트웨어 전 체인 검증 가능
  (허브는 Zigbee 없이도 기동됨).
