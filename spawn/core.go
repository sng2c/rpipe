package spawn

import (
	"bufio"
	"bytes"
	"context"
	log "github.com/sirupsen/logrus"
	"io"
	"os/exec"
)

func ReaderChannel(rd io.Reader, blockSize int) <-chan []byte {
	return ReaderBufferChannel(rd, blockSize, '\n')
}

func ReaderBufferChannel(rd io.Reader, blockSize int, delim byte) <-chan []byte {
	recvch := make(chan []byte)
	go func() {
		defer close(recvch)
		full := []byte{}
		for {
			buf := make([]byte, blockSize)
			hasRead, err := rd.Read(buf)
			if err != nil {
				break // EOF
			}
			full = append(full, buf[:hasRead]...)

			for {
				found := bytes.IndexByte(full, delim)
				if found == -1 {
					if len(full) >= blockSize {
						// flush
						recvch <- full[:blockSize]
						full = full[blockSize:]
					}
					break
				} else {
					// flush
					recvch <- full[:found+1]
					full = full[found+1:]
				}
			}
		}
		if len(full) > 0 {
			//flush
			recvch <- full
		}
	}()
	return recvch
}
func WriterChannel(wr io.Writer) chan<- []byte {
	sendch := make(chan []byte)
	go func() {
		defer close(sendch)
		writer := bufio.NewWriter(wr)
		for {
			select {
			case data := <-sendch:
				_, err := writer.Write(data)
				if err != nil {
					log.Debug(err)
				}
				err = writer.Flush()
				if err != nil {
					log.Debug(err)
				}
			}
		}
	}()
	return sendch
}

type SpawnedInfo struct {
	Cmd           *exec.Cmd
	In            chan<- []byte
	Out           <-chan []byte
	Err           <-chan []byte
	CancelContext context.Context
}

func Spawn(ctx context.Context, blockSize int, cmd *exec.Cmd) (*SpawnedInfo, error) {

	// STDOUT
	outPipe, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	outChan := ReaderChannel(outPipe, blockSize)

	// STDERR
	errPipe, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}
	errChan := ReaderChannel(errPipe, blockSize)

	// STDIN
	inPipe, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	inChan := WriterChannel(inPipe)

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
