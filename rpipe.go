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
	"regexp"
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

var verPat = regexp.MustCompile(`^([0-9]+):(.+)$`)
var ver0MsgPat = regexp.MustCompile(`^([a-zA-Z0-9\\._-]+):(.+)$`)

func main() {
	flag.Usage = func() {
		_, _ = fmt.Fprintf(flag.CommandLine.Output(), "Usage: %s [flags] COMMAND...\n", os.Args[0])
		_, _ = fmt.Fprintf(flag.CommandLine.Output(), "Flags:\n")
		flag.PrintDefaults()
	}

	systemHostname, err := os.Hostname()
	if err != nil {
		log.Fatalln("Can not get Hostname", err)
	}

	var protocol string
	var redisURL string
	var myChnName string
	var targetChnName string
	var verbose bool
	var strip bool
	flag.BoolVar(&verbose, "verbose", false, "Verbose")
	flag.BoolVar(&strip, "strip", false, "Strip target name in received message")
	flag.StringVar(&protocol, "protocol", "0", "Protocols. 0:Non-secure")
	flag.StringVar(&redisURL, "redis", "redis://localhost:6379/0", "Redis URL")
	flag.StringVar(&myChnName, "name", systemHostname, "My channel name")
	flag.StringVar(&targetChnName, "target", targetChnName, "Target channel name")
	flag.Parse()

	if verbose {
		log.SetLevel(log.DebugLevel)
	} else {
		log.SetLevel(log.InfoLevel)
	}

	// check command
	command := flag.Args()

	// check protocol
	switch protocol {
	case "0":
	default:
		log.Fatalf("Not supported protocol '%s'", protocol)
	}

	var subCh <-chan *redis.Message
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
		log.Fatalf("Redis Ping Fail", err)
	} else {
		pubsub := rdb.Subscribe(ctx, myChnName)
		defer pubsub.Close()
		subCh = pubsub.Channel()
	}

	// signal notification
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	var spawnInfo *spawn.SpawnedInfo
	if len(command) > 0 {
		cmd := exec.Command(command[0], command[1:]...) //Just for testing, replace with your subProcess
		// pass Env
		cmd.Env = os.Environ()
		cmd.Env = append(cmd.Env, "RPIPE_PROTOCOL="+protocol)

		spawnInfo, err = spawn.Spawn(ctx, cmd)
		if err != nil {
			log.Fatalln("Spawn Error", err)
			return
		}
	}

	var readCh <-chan string
	var readErrorCh <-chan string
	var writeCh chan<- string

	if spawnInfo != nil {
		readCh = spawnInfo.Out
		readErrorCh = spawnInfo.Err
		writeCh = spawnInfo.In
	} else {
		readCh = spawn.ReaderChannel(os.Stdin)
		readErrorCh = make(chan string)
		writeCh = spawn.WriterChannel(os.Stdout)
	}

	log.SetFormatter(&easy.Formatter{
		LogFormat: "%msg%",
	})
	log.Printf("Rpipe V%s\n", VERSION)
	log.Printf("  protocol  : %s\n", protocol)
	log.Printf("  name      : %s\n", myChnName)
	if targetChnName != "" {
		log.Printf("  target    : %s\n", targetChnName)
	} else {
		log.Printf("  target    : <None>\n")
	}

	log.Printf("  redis     : %s\n", redisURL)
	log.Printf("  verbose   : %t\n", verbose)
	log.Printf("  strip     : %t\n", strip)
	if spawnInfo != nil {
		log.Printf("  Command   : %v\n", command)
	} else {
		log.Printf("  Command   : <PIPE MODE>\n")
	}
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

		case data, ok := <-readCh: // CHILD -> REDIS
			log.Debugln("case <-readCh")
			if ok == false {
				log.Debugf("readCh is closed\n")
				break MainLoop
				//continue
			} else {
				var targetChn string
				var payload string

				if targetChnName != "" {
					targetChn = targetChnName
					payload = protocol + ":" + myChnName + ":" + data
				} else {
					// "TARGET:PAYLOAD" -> "0:SOURCE:PAYLOAD" -> [TARGET]
					matched := ver0MsgPat.FindStringSubmatch(data)
					if len(matched) != 3 {
						log.Warningf("No target channel in '%s' (pub ver0)", data)
						continue MainLoop
					} else {
						targetChn = matched[1]
						payload = protocol + ":" + myChnName + ":" + matched[2]
					}
				}
				log.Debugf("[PUB-%s] %s", targetChn, escapeNewLine(payload))
				//log.Infof("PUB-%s %d", targetChn, len(payload))
				rdb.Publish(ctx, targetChn, payload)
			}

		case <-sigs:
			log.Debugln("case <-sigs")
			break MainLoop

		case msg := <-subCh:
			log.Debugln("case <-subsh")

			data := msg.Payload
			matched := verPat.FindStringSubmatch(data)
			if len(matched) != 3 {
				log.Warningf("Invalid format '%s' (sub)", data)
				continue MainLoop
			}
			data_protocol := matched[1] // for extending protocols
			body := matched[2]
			log.Debugf("[SUB-%s] %s\n", msg.Channel, escapeNewLine(data))
			//log.Printf("SUB-%s %d\n", msg.Channel, len(data))
			switch data_protocol {
			case "0":
				if strip {
					matched = ver0MsgPat.FindStringSubmatch(body)
					if len(matched) != 3 {
						log.Warningf("No target channel in '%s' (pub ver0)", data)
					} else {
						body = matched[2]
					}
					writeCh <- body
				}else{
					if ver0MsgPat.MatchString(body) == false {
						log.Warningf("Invalid format '%s' (sub ver0)", body)
					}
					writeCh <- body
				}


			default:
				log.Warningf("Not supported protocol '%s' (sub)", data_protocol)
				continue MainLoop
			}
		}
	}
	log.Debugln("Bye~")
}
