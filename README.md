# rpipe

`rpipe` bridges a local process (or stdin/stdout) with Redis pub/sub channels, enabling distributed message passing between processes across machines — with end-to-end encryption.

## How it works

```
  Process A (machine 1)                         Process B (machine 2)
  ┌─────────────────────┐                       ┌─────────────────────┐
  │   COMMAND stdout    │──► channel "B" ──────►│   COMMAND stdin     │
  │   COMMAND stdin     │◄────── channel "A" ◄──│   COMMAND stdout    │
  └─────────────────────┘                       └─────────────────────┘
    rpipe -name A -target B cmd             rpipe -name B -target A cmd
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
Usage: rpipe [flags] [COMMAND...]

Flags:
  -name,  -n  string   My channel name (required)
  -target,-t  string   Target channel to send messages to
  -redis, -r  string   Redis URL (default: redis://localhost:6379/0)
  -chat,  -c           Chat mode: structured NAME:DATA format messaging (default: pipe mode)
  -nonsecure           Disable encryption
  -blocksize  int      Block size in bytes (default: 524288 = 512 KiB)
  -verbose,-v          Enable debug logging
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

Messages use `NAME:DATA` format, grouped by sender.

```bash
# Send: write "target:data" lines to stdin
echo "bob:hello" | rpipe -name alice -target bob -chat

# Receive: reads lines in "sender:data" format from stdout
rpipe -name alice -chat | while IFS= read -r line; do echo "got: $line"; done
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

Type `bob:hello` on node A — node B receives `alice:hello`.

### Remote command execution

**Server (bob):**
```bash
rpipe -name bob -target alice -chat bash
```

**Client (alice):**
```bash
rpipe -name alice -target bob -chat
# Type: bob:ls -la
# Output: alice:total 12\n...
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

## Environment variables

| Variable       | Set by | Description              |
|----------------|--------|--------------------------|
| `RPIPE_NAME`   | rpipe  | This node's channel name |
| `RPIPE_TARGET` | rpipe  | The target channel name  |

## License

See [LICENSE](LICENSE).
