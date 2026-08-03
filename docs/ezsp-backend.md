# EZSP Zigbee 백엔드 (ZBDongle-E) 로드맵

> 목표: 우리 허브가 **P(zstack)와 E(EZSP) 두 Zigbee 동글을 모두 지원**. E는
> EFR32MG21(EmberZNet) 기반이라 P(TI CC2652, Z-Stack)와 프로토콜이 완전히 달라서,
> go-matter처럼 **바닥부터 EZSP over ASH**를 구현한다. 백엔드는 config
> `zigbee.backend: zstack|ezsp`로 선택. E는 추후 **Thread/Matter-over-Thread**까지 확장 여지.
> (설계 철학: 레이어별 + 스펙 테스트벡터 + 실HW 검증. 참고: Silicon Labs UG101/UG100.)

## 코드 구조
```
internal/zigbee/            ← zstack(P) 드라이버 (그대로)
internal/zigbee/ezsp/
├── ash/ash.go   ash_test.go   ← ASH v2 데이터링크 (프레이밍·CRC·stuffing·랜덤화)
├── ash/conn.go                ← ASH 시리얼 전송 (reset 핸드셰이크·send/recv·ack)
├── frame.go                   ← EZSP 프레임 (version 등) + frame ID
└── driver.go                  ← driver.Driver 구현
```

## 마일스톤

- **M0 ✅ (실HW 검증 완료)** — ASH 데이터링크 + EZSP version 핸드셰이크.
  - ASH: CCITT-16 CRC(스펙 벡터 RST `C0 38 BC 7E` / RSTACK `C1 02 02 9B 7B 7E` 일치),
    byte stuffing, 데이터 랜덤화(seed 0x42), reset(RST→RSTACK), DATA/ACK/NAK, **piggyback ack**.
  - **ZBDongle-E 실측: ezspVersion=13, stackType=2(EmberZNet), stackVersion=0x7440.**
- **M1 ✅ (실HW 검증 완료)** — **EZSP 확장(v8+) 프레임 포맷 + 응답/콜백 디먹스.**
  - 송신 `[seq][frame control 2B LE = 0x0100][frame id 2B LE][params]`. 수신도 같은 레이아웃이며
    frame control에 응답 비트(0x0080)가 선다. **version만 레거시 포맷**으로 부트스트랩한다
    (포맷 자체가 협상 대상이라 리더 루프 시작 전에 동기 처리).
  - **디먹스**: EZSP는 응답과 비동기 콜백(`stackStatus`/`trustCenterJoin`/`incomingMessage`)을
    한 스트림에 섞어 보낸다. frame control 비트가 아니라 **(sequence, frame id) 쌍**으로 매칭한다
    — zigpy/bellows와 같은 방식이고, 우리가 모델링하지 않은 플래그를 NCP가 세워도 안 깨진다.
  - 레이아웃·frame id는 zigpy/bellows(v8 `_ezsp_frame_tx/_rx`)와 zigbee-herdsman `EzspFrameID`로
    교차 검증한 뒤 실기 대조했다.
  - **ZBDongle-E 실측: `getNetworkParameters`가 v13 확장 포맷으로 왕복, status=0x93
    (`EMBER_NOT_JOINED` — 공장 초기 상태).** 인코딩이 틀렸다면 응답 자체가 없었을 것이므로,
    이 왕복이 프레임 레이아웃 전체에 대한 증거다.
- **M2 ✅ (실HW 검증 완료)** — 코디네이터 네트워크 기동.
  - **순서가 자유롭지 않다**: `addEndpoint`는 스택이 뜨기 **전에만** 받아들여지므로
    `networkInit`보다 먼저 호출한다. 애플리케이션 설정처럼 읽히지만 순서 제약이 있다.
  - `networkInit`(v8+는 `EmberNetworkInitBitmask` 2바이트를 받는다 — v4의 무인자
    `networkInit`/별도 `networkInitExtended` 구도가 아니다) → `getNetworkParameters`로
    상태 확인 → 없으면 `setInitialSecurityState` + `formNetwork`.
  - 보안 프로파일은 zigbee-herdsman Ember 어댑터와 동일하게 맞췄다(z2m으로 페어링되는
    기기는 우리와도 페어링된다): 해시된 TC 글로벌 링크키(`ZigBeeAlliance09`) +
    랜덤 네트워크키 + **REQUIRE_ENCRYPTED_KEY**(네트워크키를 평문으로 안 뿌림).
  - `formNetwork`는 요청 수락 시점에 반환한다. 실제 사용 가능 시점은 비동기
    `stackStatusHandler`의 **NETWORK_UP**이므로 그것을 기다린다 — M1 디먹스가 여기서 처음
    실전에 쓰인다.
  - 네트워크키는 `<storage>/network.json`(0600)에 기록한다. **권위 있는 사본은 동글
    플래시**이고 이 파일은 동글 분실 대비 기록이다.
  - **채널은 실측으로 고른다.** 802.15.4와 2.4GHz WiFi는 같은 대역을 쓴다.
    2026-09 조사: 집 WiFi ch10(2447~2467MHz), 옆집 ch2 40MHz(~2397~2437MHz)
    → Zigbee 15(2425)·20(2450)은 충돌, **25(2475)가 유일하게 깨끗** → `zigbee.channel: 25`.
  - **실측: 1회차 form → NETWORK_UP → panId 0x8d76 / ch25 / coordinator. 2회차는 재form
    없이 `networkInit`만으로 같은 네트워크 재개** (= 영속성 확인).
- **M2.5(다음)** — `setPolicy(TRUST_CENTER_POLICY)`. ⚠️ v8+에서 결정값이
  `EzspDecisionId`(1바이트)에서 **`EzspDecisionBitmask`(2바이트)**로 바뀌었다. 조인 정책은
  M3에서 다루므로 M2에서는 일부러 건드리지 않았다. `setExtendedSecurityBitmask`
  (JOINER_GLOBAL_LINK_KEY)도 여기서 같이.
- **M3 (코드 완료, 조인 미검증)** — `permitJoining` + `trustCenterJoinHandler`.
  - `setPolicy(TRUST_CENTER_POLICY, ALLOW_JOINS|ALLOW_UNSECURED_REJOIN)`. ⚠️ v8+에서 결정값이
    `EzspDecisionId`(1바이트) → **`EzspDecisionBitmask`(2바이트)**로 바뀌었다. 자료가 엇갈려서
    2바이트를 먼저 보내고 거부되면 1바이트로 폴백하게 짰는데, **실기에서 2바이트가 통과**했다.
  - `permitJoining(0xFF)`(무기한) + 60초마다 재확인 하트비트. 하트비트는 조인창 유지 겸
    **NCP 링크 생존 증명**이다 — 아무 일도 안 일어날 때 "기기가 시도를 안 한 것"과 "우리 링크가
    죽은 것"을 구분할 수 없어서 넣었다.
  - `leaveNetwork` + `zigbee.forceForm`(⚠️ 파괴적). 채널을 바꾸는 유일한 수단이라 넣었다.
  - `--log debug` 플래그(`cmd/hub`): 프로토콜 브링업엔 원시 프레임과 모델링하지 않은 콜백까지
    보여야 한다.
  - **조인 자체는 아직 실기 검증 안 됨.** 아래 "Aqara H2가 조인하지 않은 이유" 참고.
- **M4/M5 — 보류.** Aqara H2가 전부 Matter로 갔고 전동커튼도 Zigbee가 아닐 가능성이 커서,
  **현재 확정된 Zigbee 기기가 하나도 없다.** 새 Zigbee 기기가 생기면 그때 재개한다.

## Aqara H2가 조인하지 않은 이유 (2026-09-02/03)

코디네이터는 정상이었다. `addEndpoint` SUCCESS, `networkInit` SUCCESS, 조인 정책 적용,
`EMBER_NETWORK_OPENED`, 하트비트 정상. 그런데 **조인 시도가 단 한 건도 없었다**(실패한 시도조차).
아래를 다 시험했지만 전부 무반응이었다:

| 시험 | 결과 |
|---|---|
| 채널 25 / 거리 있음 | 무반응 |
| 채널 25 / 동글을 스위치 옆으로 | 무반응 |
| 채널 15 + 출력 20dBm / 근접 | 무반응 |

**원인: H2는 Aqara Home 앱 프로비저닝을 거쳐야 서드파티 코디네이터에 조인한다.**
기기가 "Zigbee 모드"라고 앱에 표시되어도, 앱에서 프로토콜 선택 절차를 끝내지 않으면
열린 네트워크가 있어도 조인하지 않는다. (H2는 **Thread 모드로 출고**되고, 프로토콜 전환은
앱에서만 가능하다.)

**교훈: 새 Zigbee 기기를 붙이기 전에 "제조사 앱 프로비저닝이 선행돼야 하는 기기인가"를 먼저
확인할 것.** 이걸 모르면 코디네이터 쪽을 몇 시간 파게 된다.
- **M4** — `Apply`: `sendUnicast`로 ZCL on/off + Window Covering 송신.
- **M5** — `incomingMessageHandler` → ZCL 리포트 디코드(on/off, 커버 위치, Aqara multistate)
  → 버스 이벤트. **zstack 드라이버의 ZCL 인코딩/파싱(plain []byte 함수들)을 공용 `zcl` 헬퍼로
  추출해 양 백엔드 공유** 권장.
- **M6 (미래)** — 동글을 멀티프로토콜 RCP(또는 OpenThread RCP)로 재flash + otbr +
  **go-matter Thread 커미셔닝** → Matter-over-Thread. (E를 산 진짜 이유 = 이 확장성)

## 지금 쓰는 법 (M1)

`configs/ezsp-test.yaml`을 그대로 쓴다:

```bash
ls /dev/cu.usbserial-*                              # 동글 포트 확인 후 config에 반영
go run ./cmd/hub --config configs/ezsp-test.yaml
```

⚠️ 이 config는 HomeKit 이름·포트·PIN을 **운영(Pi) 브리지와 일부러 다르게** 뒀다.
같으면 Home 앱에서 두 브리지가 충돌한다.

기대 로그:

```
ezsp NCP connected  ezspVersion=13 stackType=2 stackVersion=0x7440
ezsp NCP holds no network yet  status=0x93
```

현재는 연결 + version + 네트워크 상태 조회까지. 네트워크 생성과 조명 제어(M2~M5)는 다음 단계.

### 동글은 개발이 끝날 때까지 Mac에 둔다

M2~M5는 초 단위 반복이 필요한 프로토콜 개발이라, 동글이 Pi에 있으면 매 반복이 배포 +
운영 재시작(=HomeKit·팬 끊김)이 된다. M5 완료 후 Pi로 옮긴다. Zigbee 네트워크(PAN id,
네트워크 키, 채널)는 **NCP 플래시**에 있으므로 페어링은 동글을 따라 이동한다 — 허브 쪽
디바이스 테이블만 옮기면 된다. M2의 영속화는 이 분리를 전제로 설계할 것.

## 참고 — P와의 관계
P(zstack)는 완성돼 있어 **지금 조명이 급하면 P 동글로 즉시** 가능. E는 병행 개발해서
완성되면 config 한 줄로 전환. 두 백엔드 다 `Name()="zigbee"`라 나머지 배선은 동일.
