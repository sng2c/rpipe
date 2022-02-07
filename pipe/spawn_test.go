package pipe

import (
	"bytes"
	"context"
	"log"
	"os/exec"
	"reflect"
	"strings"
	"testing"
)

func _spawn_read(cmd *exec.Cmd) ([]byte, error) {
	info, err := Spawn(context.Background(), cmd)
	if err != nil {
		return nil, err
	}
	result := <-info.Out
	return result, nil
}
func _spawn_write(data []byte) ([]byte, error) {
	rinfo, err := Spawn(context.Background(), exec.Command("nc", "-l", "59999"))
	if err != nil {
		return nil, err
	}

	ctx := context.Background()
	info, err := Spawn(ctx, exec.Command("nc", "localhost", "59999"))
	if err != nil {
		return nil, err
	}
	info.In <- data
	log.Println("sent", data)

	result := <-rinfo.Out
	log.Println("recv", result)
	return result, nil
}

func Test__spawn_read(t *testing.T) {
	type args struct {
		cmd *exec.Cmd
	}
	tests := []struct {
		name    string
		args    args
		want    []byte
		wantErr bool
	}{
		// TODO: Add test cases.
		{name: "echo", args: args{
			exec.Command("echo", "HELLO"),
		}, want: []byte("HELLO\n"), wantErr: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := _spawn_read(tt.args.cmd)
			if (err != nil) != tt.wantErr {
				t.Errorf("_spawn_read() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("_spawn_read() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test__spawn_write(t *testing.T) {
	type args struct {
		data []byte
	}
	tests := []struct {
		name    string
		args    args
		want    []byte
		wantErr bool
	}{
		// TODO: Add test cases.
		{name: "nc pipe", args: args{
			[]byte("WORLD\n"),
		}, want: []byte("WORLD\n"), wantErr: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := _spawn_write(tt.args.data)
			if (err != nil) != tt.wantErr {
				t.Errorf("_spawn_write() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("_spawn_write() got = %v, want %v", got, tt.want)
			}
		})
	}
}
func consume(ch <-chan []byte) [][]byte {
	var result [][]byte
	for {
		if b, ok := <-ch; ok {
			result = append(result, b)
		} else {
			return result
		}
	}
}
func TestReaderBufferChannel(t *testing.T) {
	type args struct {
		rd      string
		bufsize int
		delim   byte
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		// TODO: Add test cases.
		{
			name: "delim and bufsize",
			args: args{
				rd:      "ABCDEF\nHIJKLMN\n2",
				bufsize: 4,
				delim:   '\n',
			},
			want: "ABCD_EF\n_HIJK_LMN\n_2",
		},
		{
			name: "delim",
			args: args{
				rd:      "ABCDEF\nHIJKLMN\n2",
				bufsize: 2000,
				delim:   '\n',
			},
			want: "ABCDEF\n_HIJKLMN\n_2",
		},
		{
			name: "buf",
			args: args{
				rd:      "ABCDEF\nHIJKLMN\n2",
				bufsize: 3,
				delim:   '\t',
			},
			want: "ABC_DEF_\nHI_JKL_MN\n_2",
		},
		{
			name: "buf2",
			args: args{
				rd:      "111\n222\n333\n444\n555\n",
				bufsize: 13,
				delim:   '\t',
			},
			want: "111\n222\n333\n4_44\n555\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := consume(messages.ReaderBufferChannel(bytes.NewReader([]byte(tt.args.rd)), tt.args.bufsize, tt.args.delim)); string(bytes.Join(got, []byte("_")))!=tt.want {
				t.Errorf("ReaderBufferChannel() = %s, want %v",
					strings.Replace(string(bytes.Join(got, []byte("_"))), "\n", "\\n", -1),
					strings.Replace(tt.want, "\n", "\\n", -1),
					)
			}
		})
	}
}
