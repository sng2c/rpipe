package main

import (
	"context"
	"flag"
	"fmt"
	"github.com/go-redis/redis/v8"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"rpipe/messages"
	"rpipe/spawn"
	"strconv"
	"strings"
	"syscall"
)
import (
	log "github.com/sirupsen/logrus"
	easy "github.com/t-tomalak/logrus-easy-formatter"
)

var ctx = context.Background()

func escapeNewLine(s string) string {
	return strings.Replace(s, "\n", "\\n", -1)
}

const VERSION = "0.1"

func main() {
	flag.Usage = func() {
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

	flag.BoolVar(&verbose, "verbose", false, "Verbose")
	flag.StringVar(&redisURL, "redis", "redis://localhost:6379/0", "Redis URL")
	flag.StringVar(&myChnName, "name", "", "My channel. Required")
	flag.StringVar(&targetChnName, "target", targetChnName, "Target channel. No need to specify target channel in sending message.")
	flag.BoolVar(&nonsecure, "nonsecure", false, "Non-Secure messages.")
	flag.BoolVar(&pipeMode, "pipe", false, "Type and show data only. And process EOF.")
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

	// check command
	command := flag.Args()

	var remoteCh <-chan *redis.Message
	var rdb *redis.Client

	// check pipemode
	if pipeMode {
		if myChnName == "" || targetChnName == "" {
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
		defer pubsub.Close()
		remoteCh = pubsub.Channel()
	}

	var cryptor = messages.NewCryptor(rdb)
	err = cryptor.RegisterPubkey(ctx, myChnName)
	if err != nil {
		log.Fatalln("Pubkey Register Fail", err)
	}

	// signal notification
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	var spawnInfo *spawn.SpawnedInfo
	if len(command) > 0 {
		cmd := exec.Command(command[0], command[1:]...) //Just for testing, replace with your subProcess
		// pass Env
		cmd.Env = os.Environ()
		//cmd.Env = append(cmd.Env, "RPIPE_PROTOCOL="+proto)
		spawnInfo, err = spawn.Spawn(ctx, cmd)
		if err != nil {
			log.Fatalln("Spawn Error", err)
			return
		}
	}
	var localCh <-chan []byte
	var readErrorCh <-chan []byte
	var writeCh chan<- []byte

	if spawnInfo != nil {
		localCh = spawnInfo.Out
		readErrorCh = spawnInfo.Err
		writeCh = spawnInfo.In
	} else {
		localCh = spawn.ReaderChannel(os.Stdin)
		readErrorCh = make(chan []byte)
		writeCh = spawn.WriterChannel(os.Stdout)
	}

	log.SetFormatter(&easy.Formatter{
		LogFormat: "%msg%",
	})
	log.Printf("Rpipe V%s\n", VERSION)
	log.Printf("  name      : %s\n", myChnName)
	if targetChnName != "" {
		log.Printf("  target    : %s\n", targetChnName)
	} else {
		log.Printf("  target    : <None>\n")
	}

	log.Printf("  redis     : %s\n", redisURL)
	log.Printf("  verbose   : %t\n", verbose)
	log.Printf("  nonsecure : %t\n", nonsecure)
	log.Printf("  command   : %v\n", command)
	log.Printf("  pipemode  : %t\n", pipeMode)
	log.SetFormatter(&log.TextFormatter{FullTimestamp: true})

MainLoop:
	for {
		select {
		case data, ok := <-readErrorCh: // CHILD -> REDIS
			log.Debugln("case <-readErrorCh")
			if ok == false {
				log.Debugf("readErrorCh is closed\n")
				continue
			}

			log.Infof("[ERR] %s", data)

		case data, ok := <-localCh: // CHILD -> REDIS
			log.Debugln("case <-localCh")
			if ok == false {
				log.Debugf("localCh is closed\n")
				break MainLoop
			}
			var msg *messages.Msg
			if pipeMode {
				msg = &messages.Msg{
					From: myChnName,
					To:   targetChnName,
					Data: data,
				}
			} else {
				log.Debugln(string(data))
				msg, err = messages.NewMsgFromString(data)
				if err != nil {
					log.Warningln("Unmarshal Error from Local", err)
					continue MainLoop
				}
			}
			msg.From = myChnName
			if targetChnName != "" {
				msg.To = targetChnName
			}
			if pipeMode {
				msg.To = targetChnName
			}
			if !nonsecure {
				symKey, err := cryptor.FetchSymkey(ctx, msg)
				if err != nil {
					if err == messages.ExpireError {
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
				cryptedData, err := messages.EncryptMessage(symKey, msg.Data)
				if err != nil {
					log.Warningln("EncryptMessageFail Fail to Remote", err)
					continue MainLoop
				}
				msg.Data = cryptedData
				msg.Secured = true
			}
			msgJson := msg.Marshal()
			log.Debugf("[PUB-%s] %s", msg.To, msgJson)
			rdb.Publish(ctx, msg.To, msgJson)

		case <-sigs:
			log.Debugln("case <-sigs")
			break MainLoop

		case subMsg := <-remoteCh:
			log.Debugln("case <-subsh")

			payload := subMsg.Payload

			msg, err := messages.NewMsgFromString([]byte(payload))
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
				decryptedData, err := messages.DecryptMessage(symKey, msg.Data)
				if err != nil {
					log.Warningln("Decrypt Fail from Remote", err)
					continue MainLoop
				}
				msg.Data = decryptedData
				msg.Secured = false
			}
			if pipeMode {
				writeCh <- msg.Data
			} else {
				writeCh <- append(msg.Marshal(), '\n')
			}

		}
	}
	if pipeMode {
		eofMsg := messages.Msg{
			From:    myChnName,
			To:      targetChnName,
			Control: 2,
		}
		eofMsgJson := eofMsg.Marshal()
		log.Debugf("[PUB-%s] %s", eofMsg.To, eofMsgJson)
		_, _ = rdb.Publish(ctx, eofMsg.To, eofMsgJson).Result()
	}
	log.Debugln("Bye~")
}
