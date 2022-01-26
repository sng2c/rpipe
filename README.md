# rpipe

`rpipe` relays message between child process and redis pubsub channel.

1. `rpipe` subscribes Redis channel named HOSTNAME.
2. `rpipe` spawns child process as COMMAND.
3. `rpipe` publishes child's STDIO outputs to Redis.
4. `rpipe` passes messages from Redis into child's STDIN. 
5. `rpipe` works with STDIN, STDOUT pipe.
6. `rpipe` secures data with PKI encryption.

## Usage

```bash
$ ./rpipe -h
Usage: ./rpipe [flags] COMMAND...
Flags:
  -name string
    	My channel
  -redis string
    	Redis URL (default "redis://localhost:6379/0")
  -secure
    	Secure messages.
  -target string
    	Target channel. No need to specify target channel in sending message.
  -verbose
    	Verbose
```


## TO-DO
* ~~Secure (PKI)~~
* ~~STDIN/STDOUT processing~~
* Documentation