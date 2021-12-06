package main

import (
	"context"
	"flag"
	"fmt"
	"github.com/go-redis/redis/v8"
	"log"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"regexp"
	"rpipe/rpipe"
	"strconv"
	"strings"
	"syscall"
)

var ctx = context.Background()

func escapeNewLine(s string) string {
	return strings.Replace(s, "\n", "\\n", -1)
}

func main() {
	flag.Usage = func() {
		_, _ = fmt.Fprintf(flag.CommandLine.Output(), "Usage: %s [-redis redis://...] [-name HOSTNAME] COMMAND ...\n", os.Args[0])
		_, _ = fmt.Fprintf(flag.CommandLine.Output(), "Flags:\n")
		flag.PrintDefaults()
	}

	hostname, err := os.Hostname()
	if err != nil {
		hostname = "RANDOM"
	}
	var protocol string
	var redisURL string
	var hostName string
	flag.StringVar(&protocol, "protocol", "0", "Protocols. 0:Non-secure")
	flag.StringVar(&redisURL, "redis", "redis://localhost:6379/0", "Redis URL")
	flag.StringVar(&hostName, "name", hostname, "Hostname")
	flag.Parse()

	// check command
	command := flag.Args()
	if len(command) == 0 {
		flag.Usage()
		return
	}

	// check protocol
	switch protocol {
	case "0":
	default:
		log.Fatalf("Not supported protocol '%s'", protocol)
	}

	// check redisURL
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
	rdb := redis.NewClient(&redisOptions)
	pubsub := rdb.Subscribe(ctx, hostName)
	defer pubsub.Close()
	subch := pubsub.Channel()

	// signal notification
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	cmd := exec.Command(command[0], command[1:]...) //Just for testing, replace with your subProcess
	// pass Env
	cmd.Env = os.Environ()
	cmd.Env = append(cmd.Env, "RPIPE_PROTOCOL="+protocol)

	spawn, err := rpipe.Spawn(ctx, cmd)
	if err != nil {
		log.Fatal(err)
		return
	}
	verPat := regexp.MustCompile(`^([0-9]+):(.+)$`)
	ver0MsgPat := regexp.MustCompile(`^([a-zA-Z0-9_-]+):(.+)$`)

	MainLoop:
	for {
		select {
		case <-sigs:
			break MainLoop

		case <-spawn.CancelContext.Done():
			break MainLoop

		case data, ok := <-spawn.Err: // CHILD -> REDIS
			if ok == true {
				log.Printf("[STDERR] %s", data)
			}
			continue
		case data, ok := <-spawn.Out: // CHILD -> REDIS
			if ok == false {
				break MainLoop
			}

			// "TARGET:PAYLOAD" -> "0:SOURCE:PAYLOAD" -> [TARGET]
			matched := ver0MsgPat.FindStringSubmatch(data)
			if len(matched) != 3 {
				log.Printf("Invalid format '%s' (pub ver0)", data)
				continue MainLoop
			}

			chn := matched[1]
			payload := protocol + ":" + hostName + ":" + matched[2]
			log.Printf("[--> %s] %s", chn, escapeNewLine(payload))
			rdb.Publish(ctx, chn, payload)

		case msg := <-subch:
			data := msg.Payload
			matched := verPat.FindStringSubmatch(data)
			if len(matched) != 3 {
				log.Printf("Invalid format '%s' (sub)", data)
				continue MainLoop
			}
			data_protocol := matched[1] // for extending protocols
			body := matched[2]
			log.Printf("[<-- %s] %s\n", msg.Channel, escapeNewLine(data))
			switch data_protocol {
			case "0":
				if ver0MsgPat.MatchString(body) == false {
					log.Printf("Invalid format '%s' (sub ver0)", body)
					continue MainLoop
				}
				spawn.In <- body
			default:
				log.Printf("Not supported protocol '%s' (sub)", data_protocol)
				continue MainLoop
			}
		}
	}
	log.Print("Bye~")
}
