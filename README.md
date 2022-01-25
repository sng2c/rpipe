# rpipe

`rpipe` relays message between child process and redis pubsub channel.

1. `rpipe` subscribes Redis channel named HOSTNAME.
2. `rpipe` spawns child process as COMMAND.
3. `rpipe` publishes child's STDIO outputs to Redis.
4. `rpipe` passes messages from Redis into child's STDIN. 
5. `rpipe` works with STDIN, STDOUT pipe.

## Usage

```bash
$ ./rpipe -h
Usage: ./rpipe [flags] COMMAND...
Flags:
  -name string
        My channel name (default "sng2c-mac.local")
  -protocol string
        Protocols. 0:Non-secure (default "0")
  -redis string
        Redis URL (default "redis://localhost:6379/0")
  -target string
        Target channel name
  -verbose
        Verbose
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
$ go run rpipe.go -verbose -name ME perl test_worker.pl ME
Rpipe V0.1
  protocol  : 0
  name      : ME
  target    : 
  redis     : redis://localhost:6379/0
  verbose   : true
  Command   : [perl test_worker.pl ME]
DEBU[2022-01-25T15:42:52+09:00] case <-readErrorCh                           
INFO[2022-01-25T15:42:52+09:00] [STDERR] hello world!! RPIPE_PROTOCOL=0      
DEBU[2022-01-25T15:42:52+09:00] case <-readCh                                
DEBU[2022-01-25T15:42:52+09:00] PUB-ME 0:ME:HELLO_WORLD                      
DEBU[2022-01-25T15:42:52+09:00] case <-subsh                                 
DEBU[2022-01-25T15:42:52+09:00] SUB-ME 0:ME:HELLO_WORLD                      
DEBU[2022-01-25T15:42:52+09:00] case <-readCh                                
DEBU[2022-01-25T15:42:52+09:00] Bye~    
```

### Peer to Peer
```bash
$ Rpipe V0.1
  protocol  : 0
  name      : ME
  target    : 
  redis     : redis://localhost:6379/0
  verbose   : true
  Command   : [perl test_worker.pl YOU]
DEBU[2022-01-25T15:43:35+09:00] case <-readErrorCh                           
INFO[2022-01-25T15:43:35+09:00] [STDERR] hello world!! RPIPE_PROTOCOL=0      
DEBU[2022-01-25T15:43:35+09:00] case <-readCh                                
DEBU[2022-01-25T15:43:35+09:00] PUB-YOU 0:ME:HELLO_WORLD  
(waiting...)
DEBU[2022-01-25T15:44:37+09:00] case <-subsh                                 
DEBU[2022-01-25T15:44:37+09:00] SUB-ME 0:YOU:HELLO_WORLD                     
DEBU[2022-01-25T15:44:37+09:00] case <-readCh                                
DEBU[2022-01-25T15:44:37+09:00] Bye~   
```
```bash
$ go run rpipe.go -verbose -name YOU perl test_worker.pl ME
Rpipe V0.1
  protocol  : 0
  name      : YOU
  target    : 
  redis     : redis://localhost:6379/0
  verbose   : true
  Command   : [perl test_worker.pl ME]
DEBU[2022-01-25T15:44:37+09:00] case <-readErrorCh                           
INFO[2022-01-25T15:44:37+09:00] [STDERR] hello world!! RPIPE_PROTOCOL=0      
DEBU[2022-01-25T15:44:37+09:00] case <-readCh                                
DEBU[2022-01-25T15:44:37+09:00] PUB-ME 0:YOU:HELLO_WORLD  
(waiting)
```

### PIPE mode
```bash
$ go run rpipe.go -verbose -name YOU | perl -pe 's/^.+?://'
Rpipe V0.1
  protocol  : 0
  name      : YOU
  target    : 
  redis     : redis://localhost:6379/0
  verbose   : true
  Command   : <PIPE MODE>
(waiting...)
DEBU[2022-01-25T15:58:56+09:00] case <-subsh                                 
DEBU[2022-01-25T15:58:56+09:00] SUB-YOU 0:ME:HELLO                           
HELLO
(waiting...)
```

```bash
$ echo "HELLO" | go run rpipe.go -verbose -name ME -target YOU
Rpipe V0.1
  protocol  : 0
  name      : ME
  target    : YOU
  redis     : redis://localhost:6379/0
  verbose   : true
  Command   : <PIPE MODE>
DEBU[2022-01-25T15:55:34+09:00] case <-readCh                                
DEBU[2022-01-25T15:55:34+09:00] PUB-YOU 0:ME:HELLO                           
DEBU[2022-01-25T15:55:34+09:00] case <-readCh                                
DEBU[2022-01-25T15:55:34+09:00] readCh is closed                             
DEBU[2022-01-25T15:55:34+09:00] Bye~
```

## TO-DO
* Secure (PKI)
* ~~STDIN/STDOUT processing~~
