# Tuya LAN 프로토콜 3.5 — 조사 결과

> 거실 IR 블래스터(무빙쇼파용)를 **클라우드 없이** 제어하기 위한 사전 조사.
> 결론: 기존 Go 라이브러리로는 불가능하고, 3.5를 직접 구현해야 한다.

## 왜 직접 구현해야 하나

| 라이브러리 | 언어 | 지원 |
|---|---|---|
| GoTuya | Go | **3.3까지** — 우리 기기(3.5)에 못 씀 |
| tinytuya | Python | 3.1~3.5 전부 |
| tuya-local | Python | 3.5 지원 |

허브가 Go 단일 바이너리라 Python 라이브러리를 끌어올 수 없다. 3.5 구현은
`internal/tuya`에 새로 만든다. 한 번 만들면 이후 Tuya 기기 전부에 재사용된다.

## 기기 발견 (구현 완료 · 실기 검증됨)

Tuya 기기는 자기 존재를 UDP로 브로드캐스트한다. 포트 6666은 평문(3.1),
**6667은 암호화**인데 키가 모든 기기 공통이라 local key 없이도 읽힌다.

```
UDP_KEY = MD5("yGAdlopoPVldABfn")
```

브로드캐스트에서 얻는 것: `ip`, `gwId`(device id), `productKey`, **`version`**,
`active`. 이 중 `version`이 프로토콜 분기를 결정하므로 가장 먼저 확인할 값이다.

## 3.5 프레임 (실측 검증 완료)

```
00006699 UUUU SSSSSSSS MMMMMMMM LLLLLLLL (IV*12) DD..DD (TAG*16) 00009966
```

| 오프셋 | 크기 | 필드 |
|---|---|---|
| 0 | 4 | prefix `0x00006699` |
| 4 | **2** | reserved (항상 0x0000) |
| 6 | 4 | sequence |
| 10 | 4 | command |
| 14 | 4 | length — **IV부터 TAG까지**, suffix 미포함 |
| 18 | 12 | IV / nonce (패킷마다 다름) |
| 30 | N | ciphertext (패딩 없음, GCM 스트림) |
| 30+N | 16 | GCM 인증 태그 |
| 끝 | 4 | suffix `0x00009966` |

**AAD = frame[4:18]** (reserved + seq + cmd + length, 14바이트).

> ⚠️ **자료 충돌 주의.** DeepWiki 등 일부 문서는 reserved를 4바이트로 적어
> 헤더를 20바이트로 본다. **틀렸다.** 실제 브로드캐스트 236바이트를 두 해석에
> 대입한 결과:
>
> | 해석 | cmd | length | 18/20+length+4 | GCM |
> |---|---|---|---|---|
> | reserved 2B (정답) | 0x13 (19) | 214 | **236 = 실제** | **태그 검증 통과** |
> | reserved 4B | 0x130000 | 14081784 | 14081808 ≠ 236 | InvalidTag |
>
> 태그가 검증됐다는 건 AAD 범위까지 정확하다는 암호학적 증거다.

3.5는 GCM이 기밀성과 무결성을 동시에 제공하므로 **3.4까지 있던 별도 HMAC이
프레임에서 사라졌다**. 태그 검증 실패나 연결 끊김은 핸드셰이크 재시도로 처리한다.

## 세션 키 협상 (미검증 — local key 확보 후 확인 필요)

3.4와 3.5 공통으로 연결 시 3-way 핸드셰이크를 한다. 프레임 형식만 다르다.

```
C→D  0x03  SESS_KEY_NEG_START   client_nonce (16B 랜덤)
D→C  0x04  SESS_KEY_NEG_RESP    device_nonce(16B) || HMAC-SHA256(local_key, client_nonce)
C→D  0x05  SESS_KEY_NEG_FINISH  HMAC-SHA256(local_key, device_nonce)
```

클라이언트는 2단계에서 받은 HMAC이 `HMAC-SHA256(local_key, client_nonce)`와
같은지 검증한 뒤 3단계를 보낸다. 이후 모든 통신은 **session key**로 암호화한다.

세션 키 유도가 버전마다 다르다:

```python
tmp = bytes(a ^ b for a, b in zip(device_nonce, client_nonce))

# 3.4
session_key = AES_ECB_encrypt(local_key, tmp)                       # 16B 전체

# 3.5
buf = AES_GCM(local_key, iv=client_nonce[:12], plaintext=tmp)       # IV||ct||tag
session_key = buf[12:28]                                            # nonce 직후 16B
```

⚠️ 3.5 유도식은 문서 기반이고 **아직 실기 검증을 못 했다**(local key가 없어서).
구현 시 핸드셰이크가 실패하면 여기를 먼저 의심할 것.

## 남은 선행 작업 (사용자)

1. **소파 리모컨 학습** — Smart Life 앱에서 IR 블래스터에 DIY/수동 학습으로
   버튼(등받이·발받침 up/down)을 등록하고, **앱에서 실제로 소파가 움직이는 것까지**
   확인. 여기서 막히면 그 아래 단계는 의미가 없다.
2. **local key 발급** — iot.tuya.com 가입 → Cloud 프로젝트 생성 → Smart Life 계정
   QR 연동 → 기기 목록에서 device id + **local key** 확인.

## 구현 계획 (키 확보 후)

1. `internal/tuya/frame.go` — 3.5 프레임 pack/unpack (위 레이아웃, 검증 완료분)
2. `internal/tuya/session.go` — 3-way 핸드셰이크 + 세션 키 유도
3. `internal/tuya/driver.go` — `driver.Driver` 구현, DP 명령 발행
4. IR 코드 재생 — Tuya는 학습 코드를 자체 압축 포맷(base64)으로 주고받으므로
   그 포맷 해독이 별도로 필요하다. 학습된 키를 그대로 참조해 쏘는 방식이면
   해독 없이도 될 수 있어, 실제 DP 트래픽을 본 뒤 판단한다.

## HomeKit 노출

무빙쇼파는 등받이·발받침이 **각각 독립 모터**라 Cover의 위치 %가 맞지 않는다.
스위치 여러 개(등받이 up/down, 발받침 up/down) 또는 각 동작을 순간 트리거로
노출하는 편이 자연스럽다. 리모컨 기능을 확정한 뒤 정한다.

## 물리적 제약 (중요)

Tuya든 자작 ESP32든 **마지막 구간은 적외선이라 벽을 통과하지 못한다.** WiFi는
허브→블래스터 구간에만 쓰인다. 따라서 블래스터는 **소파가 보이는 거실**에 두고
시야를 가리지 않아야 한다. RF(팬·블라인드)처럼 아무 데나 둘 수 없다.
