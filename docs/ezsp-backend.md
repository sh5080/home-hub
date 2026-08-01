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
- **M2** — 네트워크: `networkInit` → (없으면 `formNetwork`, 네트워크키 영속화) →
  `getNetworkParameters`. 어댑터 엔드포인트 + 클러스터 등록.
- **M3** — `permitJoining(cfg.PermitJoin)` + `trustCenterJoinHandler`(조인 감지) →
  Aqara decoupled 설정(zstack 드라이버 로직 재사용).
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
