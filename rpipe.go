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
		_, _ = fmt.Fprintf(flag.CommandLine.Output(), "Usage: %s [flags] COMMAND...\n", os.Args[0])
		_, _ = fmt.Fprintf(flag.CommandLine.Output(), "Flags:\n")
		flag.PrintDefaults()
	}

	var redisURL string
	var myChnName string
	var targetChnName string
	var verbose bool
	var secure bool

	flag.BoolVar(&verbose, "verbose", false, "Verbose")
	flag.StringVar(&redisURL, "redis", "redis://localhost:6379/0", "Redis URL")
	flag.StringVar(&myChnName, "name", "", "My channel")
	flag.StringVar(&targetChnName, "target", targetChnName, "Target channel. No need to specify target channel in sending message.")
	flag.BoolVar(&secure, "secure", false, "Secure messages.")
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
	var localCh <-chan string
	var readErrorCh <-chan string
	var writeCh chan<- string

	if spawnInfo != nil {
		localCh = spawnInfo.Out
		readErrorCh = spawnInfo.Err
		writeCh = spawnInfo.In
	} else {
		localCh = spawn.ReaderChannel(os.Stdin)
		readErrorCh = make(chan string)
		writeCh = spawn.WriterChannel(os.Stdout)
	}

	pipeMode := myChnName != "" && targetChnName != ""

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
	log.Printf("  secure    : %t\n", secure)
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
				//continue
			}
			var msg *messages.Msg
			if pipeMode {
				msg = &messages.Msg{
					From: myChnName,
					To:   targetChnName,
					Data: data,
				}
			} else {
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
			if secure {
				symKey, err := cryptor.FetchRemoteSymkey(ctx, msg)
				if err != nil {
					if err == messages.ExpireError {
						// new symkey register
						log.Debugln("Register New Symkey", msg.SymkeyName())
						symKey, err = cryptor.RegisterNewSymkeyForRemote(ctx, msg)
						if err != nil {
							log.Warningln("Register New Symkey Fail  to Remote", err)
							continue MainLoop
						}
						msg.Refresh = true
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
			log.Debugf("[PUB-%s] %s", msg.To, escapeNewLine(msg.Data))
			rdb.Publish(ctx, msg.To, msg.Marshal())

		case <-sigs:
			log.Debugln("case <-sigs")
			break MainLoop

		case subMsg := <-remoteCh:
			log.Debugln("case <-subsh")

			payload := subMsg.Payload

			msg, err := messages.NewMsgFromString(payload)
			if err != nil {
				log.Warningln("Unmarshal Error from Remote", err)
				continue MainLoop
			}
			msg.To = subMsg.Channel

			// process
			log.Debugf("[SUB-%s] %s\n", msg.From, escapeNewLine(msg.Data))
			if msg.From == "" {
				log.Warningln("No 'From' in message from Remote", err)
			}
			if msg.Secured {
				// Decrypt with symmetric key
				symKey, err := cryptor.FetchRemoteSymkey(ctx, msg)
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
				//msg.Secured = false
			}
			if pipeMode {
				writeCh <- msg.Data
			} else {
				writeCh <- msg.Marshal()
			}

		}
	}
	log.Debugln("Bye~")
}
