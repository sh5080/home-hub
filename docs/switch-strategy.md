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

## ⚠️ 2026-09-03 정정 — H2는 Zigbee가 아니라 **Matter로 붙였다**

이 문서의 위쪽 전략(coupled / decoupled / decoupled-cycle)은 **H2를 Zigbee로 붙인다는
전제**로 쓰였다. 실제로는 H2가 **Zigbee/Thread 듀얼이고 Thread(Matter)로 출고**되며,
거실 스위치를 **Matter로 Apple Home에 직접** 붙였다. 그 결과:

- **허브는 H2를 거치지 않는다.** 조명(L1~L3)과 버튼은 Apple TV가 소유한다.
- 따라서 `decoupled: true` / `endpoint:` 같은 **Zigbee용 config는 H2에 쓰이지 않는다**.
  (커튼 등 진짜 Zigbee 기기가 생기면 그때 유효하다.)
- 벽 버튼으로 허브 기기를 제어하는 것은 **Apple Home 자동화**가 담당한다.

### Matter로 붙였을 때 실제 노출 형태 (거실 4-gang, WS-K04E 계열)

- **전등 3개**(L1~L3 = 릴레이 3채널) + **버튼 4개**(패들 4개)로 따로 올라온다.
- **L4는 릴레이가 없는 버튼 전용**이다(버튼 4 / 채널 3). 그래서 리셋 제스처(릴레이 버튼 10번
  연타)도 L4로는 안 되고 L1~L3으로 해야 한다.
- 버튼은 **상태 없는 Matter Generic Switch**라 Home 앱에서 조작할 수 없다(정상).
  그리고 이 기기는 **단일 누름만 지원** — 두 번/길게 누르기 옵션이 안 나온다.
- **토글은 Home 앱 기본 자동화로 불가**(상태 지정만 가능). `자동화 → 단축어로 변환` 후
  `가정 상태 가져오기` + `만약`으로 구현했다. 대안: 부하 없는 L3(상태 있는 전등)의
  켜짐/꺼짐을 트리거로 쓰면 조건문 없이 토글이 된다(릴레이 딸깍 소리는 감수).

### 실제 동작 중인 경로 (거실)

```
H2 L4 누름 → Matter/Thread → Apple TV(보더라우터+자동화) → HAP over LAN
          → Pi home-hub → internal/rf → MQTT → ESP32+CC1101 → 447MHz → 거실 실링팬
```

Apple TV는 ESP32와 직접 통신하지 않는다. RF 구간은 허브만 할 수 있어 Pi가 반드시 들어간다.

## 일괄소등 (2026-09-03 실측)

**거실 H2는 일괄소등 회로에 물려 있다.** 내리면 스위치 자체의 전원이 끊겨 Thread에서 이탈한다.

- **단일 차단은 안전.** 복구 후 1~2분간 "응답 없음" → 채널이 하나씩 토글되는 것처럼 보이는
  **재동기화** 구간을 거쳐 정합이 맞는다. 이때 Home 앱에서 조작하지 말 것.
- **반복 차단(연속 2회 이상)은 공장초기화를 유발한다.** 실제로 페어링이 날아가 재커미셔닝했다.
- 결론: **일괄소등은 그대로 써도 된다.** 배선 공사 불필요. 딸깍거리지만 말 것.

## 실측으로 확정할 것 (방별 공통)

- [x] ~~각 방 H2 **gang 수**~~ — 거실 = 4-gang ×2 (버튼 4 / 채널 3). 나머지 방 미확인.
- [ ] ~~각 H2의 **IEEE 주소**~~ — Matter로 붙였으므로 **불필요**. 대신 **Matter 설정 코드**를
      기기 본체 스티커에서 찍어둘 것(설명서를 버려도 본체에 있다).
- [x] ~~decoupled 게이트의 **버튼 이벤트 값**~~ — Matter Generic Switch, **단일 누름만** 지원.
- [x] ~~어느 방이 **일괄소등 회로**에 물려 있는지~~ — 안방 L1 + **거실 H2 확정**. 나머지 미확인.
- [ ] 작업방 블라인드 제어 경로(Matter 네이티브 vs HomeKit 위임).

## 현재 게이팅 하드웨어

- **Zigbee 코디네이터 동글**(CC2652, 예 Sonoff ZBDongle-P) — 조명 전체의 전제. **미연결**.
- ESP32 + RF 송·수신기 — 안방 실링팬 전용. 미보유.
- 동글/ESP32 도착 전에도 **HAP + MQTT mock**으로 소프트웨어 전 체인 검증 가능
  (허브는 Zigbee 없이도 기동됨).
