package pipe

import (
	"reflect"
	"testing"
)

func TestScanLines(t *testing.T) {
	type args struct {
		buf   []byte
		atEOF bool
	}
	tests := []struct {
		name    string
		args    args
		want    [][]byte
		want1   []byte
		wantErr bool
	}{
		// TODO: Add test cases.
		{
			name:    "test",
			args:    args{[]byte("abc\nabc\nabc"), false},
			want:    [][]byte{[]byte("abc"), []byte("abc")},
			want1:   []byte("abc"),
			wantErr: false,
		},
		{
			name:    "test",
			args:    args{[]byte("abc\nabc\nabc"), true},
			want:    [][]byte{[]byte("abc"), []byte("abc"), []byte("abc")},
			want1:   []byte{},
			wantErr: false,
		},
		{
			name:    "test2",
			args:    args{[]byte("abc\nabc\nabc\n"), false},
			want:    [][]byte{[]byte("abc"), []byte("abc"), []byte("abc")},
			want1:   []byte{},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, got1, err := ScanLines(tt.args.buf, tt.args.atEOF)
			if (err != nil) != tt.wantErr {
				t.Errorf("ScanLines() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ScanLines() got = %v, want %v", got, tt.want)
			}
			if !reflect.DeepEqual(got1, tt.want1) {
				t.Errorf("ScanLines() got1 = %v, want %v", got1, tt.want1)
			}
		})
	}
}
