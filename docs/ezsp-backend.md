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
- **M1** — **EZSP 확장(v8+) 프레임 포맷.** v13은 version 이후 모든 커맨드에 2바이트 frame id +
  16비트 frame control을 씀. version만 레거시 포맷으로 부트스트랩. + getValue/getConfig 등 기본.
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

## 지금 쓰는 법 (M0)
```yaml
zigbee:
  port: /dev/cu.usbserial-2110   # ZBDongle-E
  backend: ezsp
  storage: ""                    # dev
  permitJoin: false
```
현재는 연결+version까지. 조명 제어(M2~M5)는 다음 단계.

## 참고 — P와의 관계
P(zstack)는 완성돼 있어 **지금 조명이 급하면 P 동글로 즉시** 가능. E는 병행 개발해서
완성되면 config 한 줄로 전환. 두 백엔드 다 `Name()="zigbee"`라 나머지 배선은 동일.
