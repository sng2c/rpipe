package main

import (
	"context"
	"flag"
	"fmt"
	"github.com/go-redis/redis/v8"
	"github.com/logrusorgru/aurora"
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

func escape(s string) string {
	return strings.Replace(s, "\n", "\\n", -1)
}
func parseMessage(s string) ([]string, error) {
	return strings.SplitAfter(s, ":"), nil
}

type Message struct {
	Channel string
	From    string
	Payload string
}

var opts2 struct{}
var opts struct {
	Name string `long:"name" short:"n" required:"false" name:"Hostname"`
}

func main() {
	flag.Usage = func() {
		_, _ = fmt.Fprintf(flag.CommandLine.Output(), aurora.White("Usage: %s [-redis redis://...] [-name HOSTNAME] COMMAND ...\n").String(), os.Args[0])
		_, _ = fmt.Fprintf(flag.CommandLine.Output(), aurora.White("Flags:\n").String())
		flag.PrintDefaults()
	}

	hostname, err := os.Hostname()
	if err != nil {
		hostname = "RANDOM"
	}
	redisurl := flag.String("redis", "redis://localhost:6379/0", "Redis URL")
	myname := flag.String("name", hostname, "Hostname")
	flag.Parse()
	command := flag.Args()
	if len(command) == 0 {
		flag.Usage()
		return
	}

	redisUrl, err := url.Parse(*redisurl)
	if err != nil {
		log.Fatalf("Invalid REDIS url")
	}
	redisUsername := redisUrl.User.Username()
	redisPassword, _ := redisUrl.User.Password()

	redisDB := 0
	redisPath := redisUrl.Path
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
		Addr:     redisUrl.Host,
		Username: redisUsername,
		Password: redisPassword,
		DB:       redisDB,
	}

	// signal notification
	//command := strings.Split("perl test_worker.pl", " ")

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	// redis subscribe
	rdb := redis.NewClient(&redisOptions)
	pubsub := rdb.Subscribe(ctx, *myname)
	defer pubsub.Close()
	subch := pubsub.Channel()

	cmd := exec.Command(command[0], command[1:]...) //Just for testing, replace with your subProcess
	//cmd := exec.Command("echo", "test_worker.pl") //Just for testing, replace with your subProcess
	spawn, err := rpipe.Spawn(ctx, cmd)
	if err != nil {
		log.Fatal(err)
		return
	}
	msgpat := regexp.MustCompile(`^([a-zA-Z0-9_-]+):(.+)$`)
MAIN_LOOP:
	for {
		select {
		case <-sigs:
			break MAIN_LOOP
		case data, ok := <-spawn.Recv:
			if ok == false {
				break MAIN_LOOP
			}
			// TO:PAYLOAD
			// 정규식으로 체크 할것.
			matched := msgpat.FindStringSubmatch(data)
			if len(matched) != 3 {
				log.Printf("Invalid format '%s'", data)
				continue
			}
			chn := matched[1]
			payload := *myname + ":" + matched[2]
			log.Printf("--> [%s] %s\n", chn, escape(payload))
			rdb.Publish(ctx, chn, payload)
		case msg := <-subch:
			// FROM:PAYLOAD
			log.Printf("<-- [%s] %s\n", msg.Channel, escape(msg.Payload))
			if msgpat.MatchString(msg.Payload) == false {
				log.Printf("Invalid format '%s'", msg.Payload)
				continue
			}
			spawn.Send <- msg.Payload
		case <-spawn.CancelContext.Done():
			break MAIN_LOOP
		}
	}
	log.Print("Bye~")
}
