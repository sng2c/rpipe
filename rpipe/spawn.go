package rpipe

import (
	"bufio"
	"context"
	"io"
	"log"
	"os/exec"
)

func RecvChannel(rd io.Reader) <-chan string {
	recvch := make(chan string)
	go func() {
		defer close(recvch)
		scanner := bufio.NewScanner(rd)
		for scanner.Scan() {
			var line = scanner.Text()
			recvch <- line
		}
		log.Print("EOF")
	}()
	return recvch
}

func SendChannel(wr io.Writer) chan<- string {
	sendch := make(chan string)
	go func() {
		defer close(sendch)
		writer := bufio.NewWriter(wr)
		for {
			select {
			case data := <-sendch:
				_, err := writer.WriteString(data + "\n")
				if err != nil {
					log.Print(err)
				}
				err = writer.Flush()
				if err != nil {
					log.Print(err)
				}
			}
		}
	}()
	return sendch
}

type SpawnedInfo struct {
	Cmd           *exec.Cmd
	In            chan<- string
	Out           <-chan string
	Err           <-chan string
	CancelContext context.Context
}

func Spawn(ctx context.Context, cmd *exec.Cmd) (SpawnedInfo, error) {

	// STDOUT
	outPipe, err := cmd.StdoutPipe()
	if err != nil {
		return SpawnedInfo{}, err
	}
	outChan := RecvChannel(outPipe)

	// STDERR
	errPipe, err := cmd.StderrPipe()
	if err != nil {
		return SpawnedInfo{}, err
	}
	errChan := RecvChannel(errPipe)

	// STDIN
	inPipe, err := cmd.StdinPipe()
	if err != nil {
		return SpawnedInfo{}, err
	}
	inChan := SendChannel(inPipe)

	cancelCtx, cancel := context.WithCancel(ctx)
	go func() {
		err := cmd.Run()
		if err != nil {
			log.Print(err)
		}
		go func() {
			defer cancel()
			_ = cmd.Wait()
		}()
		log.Println("Command exited.")
	}()

	go func() {
		<-ctx.Done()
		log.Print("Cancel context and KILL")
		_ = cmd.Process.Kill()
	}()

	return SpawnedInfo{cmd, inChan, outChan, errChan, cancelCtx}, nil
}
