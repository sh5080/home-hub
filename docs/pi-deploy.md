# 라즈베리파이 배포

> Mac을 덮으면 허브가 죽어 아이폰에서 조작이 안 되는 문제를 없애기 위해,
> 허브를 **Raspberry Pi 3 Model B v1.2**에서 상시 구동한다.
> 하드웨어 배경은 `docs/rf-bridge.md`, 기기 매핑은 `docs/control-map.md` 참고.

## 왜 이 형태인가

- **바이너리만 던진다.** Pi에서 `go build`를 하지 않는다 — 1GB RAM에서 Go 빌드는
  스왑을 때려 SD 수명을 깎고 오래 걸린다. Mac에서 크로스컴파일해 scp 한다.
- **Docker 안 쓴다.** 1GB에 오버헤드를 얹을 이유가 없고, 순수 Go 단일 정적
  바이너리라 컨테이너가 해결해 줄 의존성 문제 자체가 없다.
- **`CGO_ENABLED=0`.** cgo가 딸려오면 Pi의 glibc 버전에 묶인다. 지금 트리는 cgo
  의존이 없으므로 완전 정적으로 빌드된다(약 10MB).

## 배치

| | 값 |
|---|---|
| 호스트 / SSH 키 | `deploy/local.mk`의 `PI_HOST` / `PI_KEY` — **git 제외**(접속 정보는 커밋하지 않는다) |
| OS | Raspberry Pi OS (bookworm) **32bit / armv7l** |
| 바이너리 | `/var/lib/homehub/bin/hub` |
| 설정 | `/var/lib/homehub/configs/pi.yaml` ← 로컬 `configs/pi.yaml` (템플릿: `configs/pi.yaml.example`) |
| HAP 상태 | `/var/lib/homehub/hap-data` — **지우면 아이폰 재페어링 필요** |
| 서비스 | `homehub.service` (`deploy/homehub.service`) |
| 포트 | HAP 51826 / MQTT 1883 / health 8086 |

`configs/pi.yaml`은 git 제외다 — HAP pin이 이 브리지와 페어링할 수 있는 자격증명이라
공개 레포에 올리지 않는다. 템플릿을 복사해 pin만 실제 값으로 바꿔 쓴다.

**wlan0는 꺼져 있다**(`nmcli radio wifi off`). 유선·무선이 같은 서브넷에 동시에
올라와 있으면 mDNS가 IP 두 개를 광고해 HAP 페어링이 꼬일 수 있다. 되살리려면
`sudo nmcli radio wifi on`.

## 배포

```bash
make deploy          # 크로스컴파일 → scp → systemctl restart → healthz 확인
```

처음이면 `deploy/local.mk`부터 만든다(git 제외):

```make
PI_HOST = <user>@<pi-ip>
PI_KEY  = ~/path/to/key.pem
```

수동으로 하면:

```bash
make pi                                                # hub-arm 생성
scp -i $PI_KEY hub-arm         $PI_HOST:/var/lib/homehub/bin/hub
scp -i $PI_KEY configs/pi.yaml $PI_HOST:/var/lib/homehub/configs/pi.yaml
ssh -i $PI_KEY $PI_HOST 'sudo systemctl restart homehub'
```

서비스 유닛을 고쳤을 때만:

```bash
scp -i $PI_KEY deploy/homehub.service $PI_HOST:/tmp/
ssh -i $PI_KEY $PI_HOST \
  'sudo cp /tmp/homehub.service /etc/systemd/system/ && sudo systemctl daemon-reload && sudo systemctl restart homehub'
```

## 점검

```bash
ssh ... 'systemctl is-active homehub'                # active
ssh ... 'curl -s localhost:8086/healthz'             # {"status":"ok"}
ssh ... 'journalctl -u homehub -f'                   # 실시간 로그
ssh ... 'ss -tn | grep 1883'                         # ESP32 브리지가 붙어 있는지
dns-sd -B _hap._tcp .                                # Mac에서: Home_Hub 광고 확인
```

`ss`에 ESP32가 안 보이면 브리지가 허브를 못 찾는 것 —
`esp32/rf_bridge/secrets.h`의 `MQTT_HOST`가 Pi IP인지 확인하고 재플래시.

## 최초 셋업 때 한 것 (재구축용 기록)

```bash
sudo ufw allow 1883/tcp                       # 22/51826/5353은 이미 열려 있었음
mkdir -p /var/lib/homehub/{bin,configs,hap-data,zigbee-data}
sudo nmcli radio wifi off
sudo cp deploy/homehub.service /etc/systemd/system/
sudo systemctl enable --now homehub
```

5월 초기 구축 때 이미 되어 있던 것: `/tmp` tmpfs 128M, journald 50M 상한,
ufw active, SSH 키 인증. SD 수명 보호용이라 유지할 것.

## 알아둘 것

- **Pi IP가 바뀌면 ESP32가 조용히 죽는다.** 펌웨어에 허브 IP가 하드코딩이라, 에러 없이
  팬만 안 먹는 형태로 고장난다. 그래서 Pi는 **NetworkManager 고정 IP**를 쓴다:

  ```bash
  sudo nmcli con mod "Wired connection 1" ipv4.method manual \
    ipv4.addresses 192.168.0.x/24 ipv4.gateway 192.168.0.1 ipv4.dns 192.168.0.1
  sudo nmcli con up "Wired connection 1"     # 주소가 바뀌므로 SSH가 끊긴다(정상)
  ```

  주소는 **공유기 DHCP 풀 바깥**에서 고른다(TP-Link 기본 풀은 `.100~.199`이므로 `.50`대).
  풀 안쪽에 고정 IP를 박으면 공유기가 같은 주소를 다른 기기에 내줄 여지가 남는다.
  고르기 전에 `ping`으로 비어 있는지 확인할 것. 공유기에서 DHCP 예약을 거는 방법도
  동등하게 유효하다 — 그쪽을 쓰면 Pi는 DHCP 그대로 두면 된다.
- **메모리 cgroup 컨트롤러가 꺼져 있다.** 유닛에 `MemoryMax`를 써도 무시된다
  (`deploy/homehub.service` 주석 참고). 실사용 30MB 안팎이라 지금은 문제없음.
- **Zigbee 동글은 Pi USB에 물리적으로 꽂혀 있어야 한다**(허브가 시리얼을 직접 연다).
  반면 **ESP32는 Pi와 선으로 연결되지 않는다** — WiFi/MQTT로만 붙으므로 전파가
  닿는 위치의 USB 충전기에 꽂으면 된다.
- 동글을 붙일 때 `pi.yaml`의 `zigbee.port`는 `/dev/ttyUSB0`이 아니라
  `/dev/serial/by-id/usb-ITEAD_*`를 쓸 것(USB 순서가 바뀌어도 안 흔들린다).
