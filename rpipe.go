package main

import (
	"context"
	"flag"
	"fmt"
	"github.com/go-redis/redis/v8"
	"github.com/sng2c/rpipe/msgspec"
	"github.com/sng2c/rpipe/pipe"
	"github.com/sng2c/rpipe/secure"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"syscall"
)
import (
	log "github.com/sirupsen/logrus"
)

var ctx = context.Background()

const VERSION = "0.2.4"

type Str string

func (s Str) Or(defaultStr Str) Str {
	if s == "" {
		return defaultStr
	}
	return s
}
func main() {
	log.SetFormatter(&log.TextFormatter{FullTimestamp: true})
	flag.Usage = func() {
		_, _ = fmt.Fprintf(flag.CommandLine.Output(), "Rpipe V%s\n", VERSION)
		_, _ = fmt.Fprintf(flag.CommandLine.Output(), "Usage: %s [flags] [COMMAND...]\n", os.Args[0])
		_, _ = fmt.Fprintf(flag.CommandLine.Output(), "Flags:\n")
		flag.PrintDefaults()
	}

	var redisURL string
	var myChnName string
	var targetChnName string
	var verbose bool
	var nonsecure bool
	var pipeMode bool
	var blockSize int
	defaultBlockSize := 512 * 1024
	channelLineBufferMap := make(map[string][]byte)

	flag.BoolVar(&verbose, "verbose", false, "Verbose")
	flag.BoolVar(&verbose, "v", false, "Verbose")
	flag.StringVar(&redisURL, "redis", "redis://localhost:6379/0", "Redis URL")
	flag.StringVar(&redisURL, "r", "redis://localhost:6379/0", "Redis URL")
	flag.StringVar(&myChnName, "name", "", "My channel. Required")
	flag.StringVar(&myChnName, "n", "", "My channel. Required")
	flag.StringVar(&targetChnName, "target", targetChnName, "Target channel. No need to specify target channel in sending message.")
	flag.StringVar(&targetChnName, "t", targetChnName, "Target channel. No need to specify target channel in sending message.")
	flag.BoolVar(&nonsecure, "nonsecure", false, "Non-Secure rpipe.")
	flag.BoolVar(&pipeMode, "pipe", false, "Type and show data only. And process EOF.")
	flag.BoolVar(&pipeMode, "p", false, "Type and show data only. And process EOF.")
	flag.IntVar(&blockSize, "blocksize", defaultBlockSize, "blocksize in bytes")
	flag.Parse()

	if verbose {
		log.SetLevel(log.DebugLevel)
	} else {
		log.SetLevel(log.InfoLevel)
	}

	if myChnName == "" {
		flag.Usage()
		log.Fatalln("-name flag is required")
	}

	// blockSize in KiB
	if blockSize <= 0 {
		blockSize = defaultBlockSize
	}

	// check command
	command := flag.Args()

	var remoteCh <-chan *redis.Message
	var rdb *redis.Client

	// check pipemode
	if pipeMode {
		if targetChnName == "" {
			log.Fatalln("-name and -target flag is required")
		}
	}

	// check redis connection
	redisAddr, err := url.Parse(redisURL)
	if err != nil {
		log.Fatalf("Invalid REDIS url")
	}
	redisUsername := redisAddr.User.Username()
	redisPassword, _ := redisAddr.User.Password()
	redisDB := 0
	redisPath := redisAddr.Path
	if len(redisPath) > 0 {
		if redisPath[0] == '/' {
			redisPath = redisPath[1:]
		}
	}
	if len(redisPath) > 0 {
		redisDB, err = strconv.Atoi(redisPath)
		if err != nil {
			log.Fatalf("Invalid DB index '%s'", redisPath)
		}
	}

	redisOptions := redis.Options{
		Addr:     redisAddr.Host,
		Username: redisUsername,
		Password: redisPassword,
		DB:       redisDB,
	}

	// redis subscribe
	rdb = redis.NewClient(&redisOptions)
	_, err = rdb.Ping(ctx).Result()
	if err != nil {
		log.Fatalln("Redis Ping Fail", err)
	} else {
		pubsub := rdb.Subscribe(ctx, myChnName)
		defer func(pubsub *redis.PubSub) {
			_ = pubsub.Close()
		}(pubsub)
		remoteCh = pubsub.Channel()
	}

	var cryptor = secure.NewCryptor(rdb)
	err = cryptor.RegisterPubkey(ctx, myChnName)
	if err != nil {
		log.Fatalln("Pubkey Register Fail", err)
	}

	// signal notification
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	var spawnInfo *pipe.SpawnedInfo
	if len(command) > 0 {
		cmd := exec.Command(command[0], command[1:]...) //Just for testing, replace with your subProcess
		// pass Env
		cmd.Env = os.Environ()
		cmd.Env = append(cmd.Env, "RPIPE_NAME="+myChnName, "RPIPE_TARGET="+targetChnName)
		spawnInfo, err = pipe.Spawn(ctx, cmd)
		if err != nil {
			log.Fatalln("Spawn Error", err)
			return
		}
	}
	var fromLocalCh <-chan []byte
	var fromLocalErrorCh <-chan []byte
	var toLocalCh chan<- []byte

	if spawnInfo != nil {
		fromLocalCh = spawnInfo.Out
		fromLocalErrorCh = spawnInfo.Err
		toLocalCh = spawnInfo.In
	} else {
		if pipeMode {
			fromLocalCh = pipe.ReadLineBufferChannel(os.Stdin, blockSize, '\n')
			fromLocalErrorCh = make(chan []byte)
			toLocalCh = pipe.WriteLineChannel(os.Stdout)
		} else {
			fromLocalCh = pipe.ReadLineChannel(os.Stdin)
			fromLocalErrorCh = make(chan []byte)
			toLocalCh = pipe.WriteLineChannel(os.Stdout)
		}
	}

	_, _ = os.Stderr.WriteString(fmt.Sprintf("Rpipe V%s\n", VERSION))
	_, _ = os.Stderr.WriteString(fmt.Sprintf("  RPIPE_NAME      : %s\n", myChnName))
	_, _ = os.Stderr.WriteString(fmt.Sprintf("  RPIPE_TARGET    : %s\n", Str(targetChnName).Or("<None>")))

MainLoop:
	for {
		select {
		case data, ok := <-fromLocalErrorCh: // CHILD -> REDIS
			log.Debugln("case <-fromLocalErrorCh")
			if ok == false {
				log.Debugf("fromLocalErrorCh is closed\n")
				break MainLoop
			}
			_, _ = os.Stderr.Write(data)

		case data, ok := <-fromLocalCh: // CHILD -> REDIS
			log.Debugln("case <-fromLocalCh")
			if ok == false {
				log.Debugf("fromLocalCh is closed\n")
				break MainLoop
			}
			var appMsgs []*msgspec.ApplicationMsg

			if pipeMode {
				appMsg := &msgspec.ApplicationMsg{
					Name: targetChnName,
					Data: data,
				}
				appMsgs = append(appMsgs, appMsg)
			} else {
				log.Debugln(string(data))
				appMsg, err := msgspec.NewApplicationMsg(data)
				if err != nil {
					log.Warningln("Unmarshal Error from Local", err)
					continue MainLoop
				}
				// split
				appData := appMsg.Data
				for len(appData) >= blockSize {
					appMsgs = append(appMsgs, &msgspec.ApplicationMsg{Name: appMsg.Name, Data: appData[:blockSize]})
					appData = appData[blockSize:]
				}
				if len(appData) > 0 {
					appMsgs = append(appMsgs, &msgspec.ApplicationMsg{Name: appMsg.Name, Data: appData})
				}
			}
			log.Debugln(appMsgs)

			for i, appMsg := range appMsgs {
				msg := &msgspec.RpipeMsg{
					From: myChnName,
					To:   appMsg.Name,
					Data: appMsg.Data,
					Pipe: pipeMode,
				}

				if msg.To == "" {
					msg.To = targetChnName
				}

				if msg.To == "" {
					log.Warningln("No target in Msg")
					continue
				}

				if !nonsecure {
					symKey, err := cryptor.FetchSymkey(ctx, msg)
					if err != nil {
						if err == secure.ExpireError {
							// new symkey register
							log.Debugln("Register New Symkey", msg.SymkeyName())
							symKey, err = cryptor.RegisterNewOutboundSymkey(ctx, msg)
							if err != nil {
								log.Warningln("Register New Symkey Fail to Remote", err)
								continue MainLoop
							}
						} else {
							log.Warningln("Fetch Symkey Fail to Remote", err)
							continue MainLoop
						}
					}
					cryptedData, err := secure.EncryptMessage(symKey, msg.Data)
					if err != nil {
						log.Warningln("EncryptMessageFail Fail to Remote", err)
						continue MainLoop
					}
					msg.Data = cryptedData
					msg.Secured = true
				}
				msgJson := msg.Marshal()
				log.Debugf("[PUB-%s#%d] %s", msg.To, i, msgJson)
				rdb.Publish(ctx, msg.To, msgJson)
			}

		case <-sigs:
			log.Debugln("case <-sigs")
			break MainLoop

		case subMsg := <-remoteCh:
			log.Debugln("case <-remoteCh")

			payload := subMsg.Payload

			msg, err := msgspec.NewMsgFromBytes([]byte(payload))
			if err != nil {
				log.Warningln("Unmarshal Error from Remote", err)
				continue MainLoop
			}
			if pipeMode {
				if msg.From != targetChnName {
					log.Warningf("A message from %s is not targeted.", msg.From)
					continue MainLoop
				}
			}
			msg.To = subMsg.Channel

			log.Debugf("[SUB-%s] %s\n", msg.From, msg.Marshal())

			if msg.Control == 1 {
				err := cryptor.ResetInboundSymkey(ctx, msg)
				if err != nil {
					log.Warningln("ResetInboundSymkey", err)
				}
				continue MainLoop
			}
			if msg.Control == 2 {
				if pipeMode {
					log.Debugln("EOF received on pipemode", err)
					break MainLoop
				}
				continue MainLoop
			}

			// process

			if msg.From == "" {
				log.Warningln("No 'From' in message from Remote", err)
			}
			if msg.Secured {
				// Decrypt with symmetric key
				symKey, err := cryptor.FetchSymkey(ctx, msg)
				if err != nil {
					log.Warningln("Fetch Symkey Fail from Remote", err)
					continue MainLoop
				}
				decryptedData, err := secure.DecryptMessage(symKey, msg.Data)
				if err != nil {
					log.Warningln("Decrypt Fail from Remote", err)
					continue MainLoop
				}
				msg.Data = decryptedData
				msg.Secured = false
			}

			if pipeMode {
				// pipemode : feed as-is
				toLocalCh <- msg.Data
			} else {
				// non-pipemode : feed by line group by sessionId
				// scanlines
				lineBuf, ok := channelLineBufferMap[msg.From]
				if !ok {
					lineBuf = []byte{}
				}
				lineBuf = append(lineBuf, msg.Data...)
				var lines [][]byte
				lines, lineBuf, err = pipe.FeedLines(lineBuf, false)
				if err != nil {
					log.Warningln("Session reset", err)
					delete(channelLineBufferMap, msg.From)
					continue MainLoop
				}
				if len(lineBuf) == 0 {
					delete(channelLineBufferMap, msg.From)
				} else {
					channelLineBufferMap[msg.From] = lineBuf
				}
				// feed all
				for _, line := range lines {
					msg.Data = line
					appMsg := &msgspec.ApplicationMsg{
						Name: msg.From,
						Data: msg.Data,
					}
					toLocalCh <- append(appMsg.Encode(), '\n')
				}
			}

		}
	}
	if pipeMode {
		eofMsg := msgspec.RpipeMsg{
			From:    myChnName,
			To:      targetChnName,
			Control: 2,
		}
		eofMsgJson := eofMsg.Marshal()
		log.Debugf("[PUB-%s] %s", eofMsg.To, eofMsgJson)
		_, _ = rdb.Publish(ctx, eofMsg.To, eofMsgJson).Result()
	} else {
		for sid, buf := range channelLineBufferMap {
			log.Debugf("Remove an uncompleted line buffer for sid '%s' : %s\n", sid, string(buf))
		}
	}
	log.Debugln("Bye~")
}
