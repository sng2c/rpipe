# rpipe

`rpipe` bridges a local process (or stdin/stdout) with Redis pub/sub channels, enabling distributed message passing between processes across machines — with end-to-end encryption.

## How it works

```
      Process A (machine 1)                         Process B (machine 2)
    +-----------------------+                     +-----------------------+
    |    COMMAND stdout     |---- channel "B" --->|     COMMAND stdin     |
    |    COMMAND stdin      |<--- channel "A" ----|     COMMAND stdout    |
    +-----------------------+                     +-----------------------+
      rpipe -name A -target B cmd                   rpipe -name B -target A cmd
```

1. `rpipe` subscribes to a Redis channel with its own name (`-name`)
2. Spawns (or acts as) a child process
3. Publishes the child's stdout to the target Redis channel (`-target`)
4. Feeds incoming Redis messages into the child's stdin

Messages are encrypted with PKI (RSA key exchange + AES symmetric encryption) by default.

## Installation

### Download binary

Download the latest binary from the [Releases](https://github.com/sng2c/rpipe/releases) page.

Available platforms:
- `linux/amd64`
- `darwin/amd64`
- `darwin/arm64` (Apple Silicon)
- `windows/amd64`

### Build from source

Requires Go 1.24+.

```bash
git clone https://github.com/sng2c/rpipe.git
cd rpipe
go build -o rpipe .
```

## Usage

```
Rpipe V1.0.5
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

## Modes

### Pipe mode (default)

Raw binary transfer. No message format required. Automatically sends an EOF signal when the input stream closes, terminating the receiver.

```bash
# Send a file to remote
cat file.tar.gz | rpipe -name alice -target bob

# Receive on the other side
rpipe -name bob -target alice > file.tar.gz
```

### Chat mode (`-chat` / `-c`)

Send format: `TARGET<message` — delivers to the TARGET channel.
When `-target` is set, use `<message` (target omitted) and the target is filled automatically.
Received messages are printed to stdout as `SENDER>message`.

```bash
# One-to-one chat: set -target and use <message
rpipe -name alice -target bob -chat
# Input:  <hello       → sent to bob
# Output: bob>hi       ← message from bob

# Multi-channel: specify target per message
rpipe -name alice -chat
# Input:  bob<hello    → sent to bob
# Input:  carol<hi     → sent to carol
# Output: bob>hey      ← message from bob
```

### Command mode

Wraps a child process. The child's stdout is published to Redis; incoming Redis messages are fed to the child's stdin.

```bash
rpipe -name alice -target bob ./my-program
```

The child process receives two environment variables:
- `RPIPE_NAME` — this node's channel name
- `RPIPE_TARGET` — the target channel name

## Examples

### File transfer

**Receiver:**
```bash
rpipe -name receiver -target sender > received.tar.gz
```

**Sender:**
```bash
cat archive.tar.gz | rpipe -name sender -target receiver
```

### Two-node chat

**Node A:**
```bash
rpipe -name alice -target bob -chat
```

**Node B:**
```bash
rpipe -name bob -target alice -chat
```

Type `<hello` on node A — node B receives `alice>hello`.

### Remote command execution

**Server (bob):**
```bash
rpipe -name bob -target alice -chat bash
```

**Client (alice):**
```bash
rpipe -name alice -target bob -chat
# Type: bob<ls -la
# Output: alice>total 12\n...
```

### Custom Redis

```bash
rpipe -name alice -target bob -redis redis://user:password@myredis.host:6379/1
```

## Security

By default, rpipe uses end-to-end encryption:

1. Each node registers its RSA public key in Redis on startup
2. A symmetric AES key is negotiated per channel pair
3. All message payloads are AES-encrypted

Use `-nonsecure` to disable encryption (e.g. for debugging or trusted networks).

### Cipher details (v1.1.0+)

| Layer       | Algorithm            |
|-------------|----------------------|
| Key exchange| RSA-2048 + OAEP (SHA-256) |
| Symmetric   | AES-256-GCM          |
| Key TTL     | 1 hour (auto-rotated) |

### Breaking change: v1.1.0 is incompatible with older versions

v1.1.0 upgraded the encryption algorithms (PKCS1v15 → OAEP, AES-128-CFB → AES-256-GCM).
All nodes communicating with each other must run the same major version.
**Upgrade all nodes simultaneously.**

## Environment variables

| Variable       | Description                                  |
|----------------|----------------------------------------------|
| `RPIPE_REDIS`  | Redis URL (corresponds to `-redis` flag)     |
| `RPIPE_NAME`   | My channel name (corresponds to `-name` flag)|
| `RPIPE_TARGET` | Target channel (corresponds to `-target` flag)|

## License

See [LICENSE](LICENSE).
