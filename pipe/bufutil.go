package pipe

import (
	"bufio"
	"bytes"
	log "github.com/sirupsen/logrus"
	"io"
)

func ScanLines(buf []byte, atEOF bool) ([][]byte, []byte, error) {
	var lines [][]byte
	var adv = -1
	var token []byte
	var err error

	for adv != 0 {
		adv, token, err = bufio.ScanLines(buf, atEOF)
		if err != nil {
			return nil, nil, err
		}
		if adv != 0 {
			lines = append(lines, token)
			buf = buf[adv:]
		}
	}
	return lines, buf, nil
}

func ReadLineChannel(rd io.Reader) <-chan []byte {
	recvch := make(chan []byte)
	go func() {
		defer close(recvch)
		scanner := bufio.NewScanner(rd)
		for scanner.Scan() {
			var line = scanner.Bytes()
			recvch <- append(line, '\n')
		}
	}()
	return recvch
}

func ReadLineBufferChannel(rd io.Reader, blockSize int, delim byte) <-chan []byte {
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
