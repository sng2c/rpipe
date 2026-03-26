# rpipe

`rpipe`는 로컬 프로세스(또는 stdin/stdout)와 Redis pub/sub 채널을 연결하여, 머신 간 분산 메시지 전달을 종단간 암호화와 함께 제공합니다.

## 동작 원리

```
      Process A (machine 1)                         Process B (machine 2)
    +-----------------------+                     +-----------------------+
    |    COMMAND stdout     |---- channel "B" --->|     COMMAND stdin     |
    |    COMMAND stdin      |<--- channel "A" ----|     COMMAND stdout    |
    +-----------------------+                     +-----------------------+
      rpipe -name A -target B cmd                   rpipe -name B -target A cmd
```

1. `rpipe`는 자신의 이름(`-name`)으로 Redis 채널을 구독합니다
2. 자식 프로세스를 실행하거나 stdin/stdout으로 동작합니다
3. 자식 프로세스의 stdout을 대상 Redis 채널(`-target`)에 발행합니다
4. Redis로 수신된 메시지를 자식 프로세스의 stdin으로 전달합니다

메시지는 기본적으로 PKI(RSA 키 교환 + AES 대칭 암호화)로 암호화됩니다.

## 설치

### 바이너리 다운로드

[Releases](https://github.com/sng2c/rpipe/releases) 페이지에서 최신 바이너리를 다운로드하세요.

지원 플랫폼:
- `linux/amd64`
- `darwin/amd64`
- `darwin/arm64` (Apple Silicon)
- `windows/amd64`

### 소스 빌드

Go 1.24 이상이 필요합니다.

```bash
git clone https://github.com/sng2c/rpipe.git
cd rpipe
go build -o rpipe .
```

## 사용법

```
Rpipe V1.1.0
Usage: rpipe [flags] [COMMAND...]
Flags:
  -blocksize int
    	blocksize in bytes (default 524288)
  -c	Chat mode: send as 'TARGET<message' (or '<message' if -target set), receive as 'SENDER>message'.
  -chat
    	Chat mode: send as 'TARGET<message' (or '<message' if -target set), receive as 'SENDER>message'.
  -n string
    	My channel name (env: RPIPE_NAME)
  -name string
    	My channel name (env: RPIPE_NAME)
  -nonsecure
    	Non-Secure rpipe.
  -r string
    	Redis URL (env: RPIPE_REDIS, default: redis://localhost:6379/0) (default "redis://localhost:6379/0")
  -redis string
    	Redis URL (env: RPIPE_REDIS, default: redis://localhost:6379/0) (default "redis://localhost:6379/0")
  -t string
    	Target channel (env: RPIPE_TARGET).
  -target string
    	Target channel (env: RPIPE_TARGET).
  -v	Verbose
  -verbose
    	Verbose
Environment variables:
  RPIPE_REDIS   Corresponds to -redis flag
  RPIPE_NAME    Corresponds to -name flag
  RPIPE_TARGET  Corresponds to -target flag
```

## 모드

### 파이프 모드 (기본값)

원시 바이너리 전송. 메시지 형식이 필요 없습니다. 입력 스트림이 닫히면 자동으로 EOF 신호를 보내 수신자를 종료합니다.

```bash
# 파일 전송
cat file.tar.gz | rpipe -name alice -target bob

# 반대편에서 수신
rpipe -name bob -target alice > file.tar.gz
```

### 채팅 모드 (`-chat` / `-c`)

송신 형식: `TARGET<message` — TARGET 채널로 전달합니다.
`-target`이 설정된 경우 `<message`만 입력하면 타겟이 자동으로 채워집니다.
수신 메시지는 `SENDER>message` 형식으로 stdout에 출력됩니다.

```bash
# 1:1 채팅: -target 설정 후 <message 입력
rpipe -name alice -target bob -chat
# 입력:  <hello       → bob으로 전송
# 출력:  bob>hi       ← bob으로부터 수신

# 멀티채널: 메시지마다 타겟 지정
rpipe -name alice -chat
# 입력:  bob<hello    → bob으로 전송
# 입력:  carol<hi     → carol로 전송
# 출력:  bob>hey      ← bob으로부터 수신
```

### 커맨드 모드

자식 프로세스를 감쌉니다. 자식 프로세스의 stdout이 Redis에 발행되고, Redis로 수신된 메시지가 자식 프로세스의 stdin으로 전달됩니다.

```bash
rpipe -name alice -target bob ./my-program
```

자식 프로세스는 두 가지 환경변수를 받습니다:
- `RPIPE_NAME` — 이 노드의 채널 이름
- `RPIPE_TARGET` — 대상 채널 이름

## 예제

### 파일 전송

**수신측:**
```bash
rpipe -name receiver -target sender > received.tar.gz
```

**송신측:**
```bash
cat archive.tar.gz | rpipe -name sender -target receiver
```

### 양방향 채팅

**노드 A:**
```bash
rpipe -name alice -target bob -chat
```

**노드 B:**
```bash
rpipe -name bob -target alice -chat
```

노드 A에서 `<hello` 입력 → 노드 B에 `alice>hello`로 수신됩니다.

### 원격 명령 실행

**서버 (bob):**
```bash
rpipe -name bob -target alice -chat bash
```

**클라이언트 (alice):**
```bash
rpipe -name alice -target bob -chat
# 입력: bob<ls -la
# 출력: alice>total 12\n...
```

### 커스텀 Redis

```bash
rpipe -name alice -target bob -redis redis://user:password@myredis.host:6379/1
```

## 보안

기본적으로 종단간 암호화가 적용됩니다:

1. 시작 시 각 노드가 RSA 공개키를 Redis에 등록
2. 채널 쌍별로 AES 대칭키 협상
3. 모든 메시지 페이로드를 AES 암호화

신뢰할 수 있는 네트워크나 디버깅 시 `-nonsecure`로 암호화를 비활성화할 수 있습니다.

### 암호화 상세 (v1.1.0+)

| 계층       | 알고리즘                        |
|------------|---------------------------------|
| 키 교환    | RSA-2048 + OAEP (SHA-256)       |
| 대칭 암호  | AES-256-GCM                     |
| 키 TTL     | 1시간 (자동 교체)               |

### 키 교체

대칭키는 1시간 후 만료됩니다. 만료 시:

1. 송신자가 수신자에게 알림(Control=1)을 보내 캐시된 키를 지우게 함
2. 새 대칭키를 협상해 Redis에 저장

수신자 측에서 AES 복호화 실패 시(예: 키 교체 중 레이스 컨디션), 캐시를 무효화하고 Redis에서 자동으로 재시도합니다.

### 호환성 주의: v1.1.0은 이전 버전과 호환되지 않습니다

v1.1.0에서 암호화 알고리즘이 변경되었습니다 (PKCS1v15 → OAEP, AES-128-CFB → AES-256-GCM).
서로 통신하는 모든 노드는 같은 메이저 버전을 사용해야 합니다.
**모든 노드를 동시에 업그레이드하세요.**

## 환경변수

| 변수           | 설명                                            |
|----------------|-------------------------------------------------|
| `RPIPE_REDIS`  | Redis URL (`-redis` 플래그에 대응)              |
| `RPIPE_NAME`   | 내 채널 이름 (`-name` 플래그에 대응)            |
| `RPIPE_TARGET` | 대상 채널 이름 (`-target` 플래그에 대응)        |

## 라이선스

[LICENSE](LICENSE) 파일을 참조하세요.
