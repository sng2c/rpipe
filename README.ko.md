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

1. `rpipe`는 `-name`으로 지정한 이름의 Redis 채널을 구독합니다.
2. 자식 프로세스를 실행하거나, stdin/stdout으로 직접 동작합니다.
3. 자식 프로세스의 stdout을 `-target` Redis 채널로 publish합니다.
4. Redis에서 수신한 메시지를 자식 프로세스의 stdin으로 전달합니다.

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

Go 1.24 이상 필요.

```bash
git clone https://github.com/sng2c/rpipe.git
cd rpipe
go build -o rpipe .
```

## 사용법

```
Rpipe V1.0.2
Usage: rpipe [flags] [COMMAND...]
Flags:
  -blocksize int
    	blocksize in bytes (default 524288)
  -c	Chat mode: send as 'TARGET<message' (or '<message' if -target set), receive as 'SENDER>message'.
  -chat
    	Chat mode: send as 'TARGET<message' (or '<message' if -target set), receive as 'SENDER>message'.
  -n string
    	My channel. Required
  -name string
    	My channel. Required
  -nonsecure
    	Non-Secure rpipe.
  -r string
    	Redis URL (default "redis://localhost:6379/0")
  -redis string
    	Redis URL (default "redis://localhost:6379/0")
  -t string
    	Target channel. No need to specify target channel in sending message.
  -target string
    	Target channel. No need to specify target channel in sending message.
  -v	Verbose
  -verbose
    	Verbose
```

## 모드

### 파이프 모드 (기본값)

원시 바이너리 전송. 메시지 형식 불필요. 입력 스트림이 닫히면 자동으로 EOF 신호를 전송해 수신 측을 종료합니다.

```bash
# 파일 전송
cat file.tar.gz | rpipe -name alice -target bob

# 수신 측
rpipe -name bob -target alice > file.tar.gz
```

### 채팅 모드 (`-chat` / `-c`)

송신 형식: `TARGET<message` — TARGET 채널로 전송.
`-target` 지정 시 `<message` (대상 생략) 형식으로 보내면 `-target`으로 자동 채워짐.
수신 메시지는 `SENDER>message` 형식으로 stdout에 출력.

```bash
# 일대일 채팅: -target 지정 후 <메시지 입력
rpipe -name alice -target bob -chat
# 입력:  <hello       → bob에게 전송
# 출력: bob>hi        ← bob이 보낸 메시지

# 멀티채널: 메시지마다 대상 직접 지정
rpipe -name alice -chat
# 입력:  bob<hello    → bob에게 전송
# 입력:  carol<hi     → carol에게 전송
# 출력: bob>hey       ← bob이 보낸 메시지
```

### 커맨드 모드

자식 프로세스를 래핑합니다. 자식의 stdout은 Redis로 publish되고, Redis 수신 메시지는 자식의 stdin으로 전달됩니다.

```bash
rpipe -name alice -target bob ./my-program
```

자식 프로세스에는 다음 환경변수가 전달됩니다:
- `RPIPE_NAME` — 이 노드의 채널 이름
- `RPIPE_TARGET` — 대상 채널 이름

## 예시

### 파일 전송

**수신 측:**
```bash
rpipe -name receiver -target sender > received.tar.gz
```

**송신 측:**
```bash
cat archive.tar.gz | rpipe -name sender -target receiver
```

### 일대일 채팅

**Node A:**
```bash
rpipe -name alice -target bob -chat
```

**Node B:**
```bash
rpipe -name bob -target alice -chat
```

Node A에서 `<hello` 입력 → Node B에서 `alice>hello` 수신.

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

## 환경변수

| 변수           | 설정 주체 | 설명                    |
|----------------|-----------|-------------------------|
| `RPIPE_NAME`   | rpipe     | 이 노드의 채널 이름     |
| `RPIPE_TARGET` | rpipe     | 대상 채널 이름          |

## 라이선스

[LICENSE](LICENSE) 파일을 참조하세요.
