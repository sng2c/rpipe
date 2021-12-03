# rpipe

## Usage

```bash
$ rpipe -h
Usage: rpipe [-redis redis://...] [-name HOSTNAME] COMMAND ...
Flags:
  -name string
        Hostname (default "kakaoui-MacBookPro.local")
  -redis string
        Redis URL (default "redis://localhost:6379/0")
```

test_worker.pl
```perl
$host = $ARGV[0];
$|++;
print "$host:HELLO WORLD\n";
<STDIN>;
```

### Self consuming
```bash
$ go run rpipe.go -name ME perl test_worker.pl ME
2021/12/03 15:47:42 --> [ME] ME:HELLO WORLD
2021/12/03 15:47:42 <-- [ME] ME:HELLO WORLD
2021/12/03 15:47:42 EOF
2021/12/03 15:47:42 Bye~
2021/12/03 15:47:42 Command exited.
```

### Peer to Peer
```bash
$ go run rpipe.go -name ME perl test_worker.pl YOU
2021/12/03 15:53:52 [--> YOU] ME:HELLO WORLD
2021/12/03 15:54:05 [<-- ME] YOU:HELLO WORLD
2021/12/03 15:54:05 EOF
2021/12/03 15:54:05 Bye~
2021/12/03 15:54:05 Command exited.
$
```
```bash
$ go run rpipe.go -name YOU perl test_worker.pl ME
2021/12/03 15:54:05 [--> ME] YOU:HELLO WORLD
```
## TO-DO
* Secure (PKI)
* STDIN/STDOUT processing
