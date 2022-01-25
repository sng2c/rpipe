# rpipe

`rpipe` relays message between child process and redis pubsub channel.

1. `rpipe` subscribes Redis channel named HOSTNAME.
2. `rpipe` spawns child process as COMMAND.
3. `rpipe` publishes child's STDIO outputs to Redis.
4. `rpipe` passes messages from Redis into child's STDIN. 

## Usage

```bash
$ spawn -h
Usage: spawn [-redis redis://...] [-name HOSTNAME] COMMAND ...
Flags:
  -name string
        Hostname (default "kakaoui-MacBookPro.local")
  -protocol string
        Protocols. 0:Non-secure (default "0")
  -redis string
        Redis URL (default "redis://localhost:6379/0")
```

### test_worker.pl
Protocol "0" needs to specify target pubsub channel.
```perl
$chn = $ARGV[0];
$|++;
print "$chn:HELLO WORLD\n";
<STDIN>;
```

### Self consuming
```bash
$ go run spawn.go -name ME perl test_worker.pl ME
2021/12/03 15:47:42 --> [ME] 0:ME:HELLO WORLD
2021/12/03 15:47:42 <-- [ME] 0:ME:HELLO WORLD
2021/12/03 15:47:42 EOF
2021/12/03 15:47:42 Bye~
2021/12/03 15:47:42 Command exited.
```

### Peer to Peer
```bash
$ go run spawn.go -name ME perl test_worker.pl YOU
2021/12/03 15:53:52 [--> YOU] 0:ME:HELLO WORLD
2021/12/03 15:54:05 [<-- ME] 0:YOU:HELLO WORLD
2021/12/03 15:54:05 EOF
2021/12/03 15:54:05 Bye~
2021/12/03 15:54:05 Command exited.
$
```
```bash
$ go run spawn.go -name YOU perl test_worker.pl ME
2021/12/03 15:54:05 [--> ME] 0:YOU:HELLO WORLD
```


## TO-DO
* Secure (PKI)
* STDIN/STDOUT processing
