package pipe

import (
	"context"
	log "github.com/sirupsen/logrus"
	"os/exec"
)

type SpawnedInfo struct {
	Cmd           *exec.Cmd
	In            chan<- []byte
	Out           <-chan []byte
	Err           <-chan []byte
	CancelContext context.Context
}

func Spawn(ctx context.Context, cmd *exec.Cmd) (*SpawnedInfo, error) {

	// STDOUT
	outPipe, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	outChan := ReadLineChannel(outPipe)

	// STDERR
	errPipe, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}
	errChan := ReadLineChannel(errPipe)

	// STDIN
	inPipe, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	inChan := WriteLineChannel(inPipe)

	cancelCtx, cancel := context.WithCancel(ctx)
	go func() {
		err = cmd.Run()
		if err != nil {
			log.Debugln(err)
		}
		go func() {
			defer cancel()
			_ = cmd.Wait()
		}()
		log.Debugln("Command exited.")
	}()

	go func() {
		<-ctx.Done()
		log.Debugln("Cancel context and KILL")
		_ = cmd.Process.Kill()
	}()

	return &SpawnedInfo{cmd, inChan, outChan, errChan, cancelCtx}, nil
}
