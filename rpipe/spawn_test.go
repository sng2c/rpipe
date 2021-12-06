package rpipe

import (
	"context"
	"log"
	"os/exec"
	"reflect"
	"testing"
)

func _spawn_read(cmd *exec.Cmd) (string, error) {
	info, err := Spawn(context.Background(), cmd)
	if err != nil {
		return "", err
	}
	result := <-info.Out
	return result, nil
}
func _spawn_write(data string) (string, error) {
	rinfo, err := Spawn(context.Background(), exec.Command("nc","-l", "59999"))
	if err != nil {
		return "", err
	}

	ctx := context.Background()
	info, err := Spawn(ctx, exec.Command("nc", "localhost", "59999"))
	if err != nil {
		return "", err
	}
	info.In <- data
	log.Println("sent",data)

	result := <-rinfo.Out
	log.Println("recv",result)
	return result, nil
}

func Test__spawn_read(t *testing.T) {
	type args struct {
		cmd *exec.Cmd
	}
	tests := []struct {
		name    string
		args    args
		want    string
		wantErr bool
	}{
		// TODO: Add test cases.
		{name: "echo", args: args{
			exec.Command("echo","HELLO"),
		}, want: "HELLO", wantErr: false},
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
		data string
	}
	tests := []struct {
		name    string
		args    args
		want    string
		wantErr bool
	}{
		// TODO: Add test cases.
		{name: "nc pipe", args: args{
			"WORLD",
		}, want: "WORLD", wantErr: false},
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