package rpipe

import (
	"bufio"
	"bytes"
	log "github.com/sirupsen/logrus"
	"io"
)

func ReadLineChannel(rd io.Reader, blockSize int) <-chan []byte {
	return ReadBufferChannel(rd, blockSize, '\n')
}

func ReadBufferChannel(rd io.Reader, blockSize int, delim byte) <-chan []byte {
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
func WriteLineChannel(wr io.Writer) chan<- []byte {
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