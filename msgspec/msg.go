package msgspec

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
)

type RpipeMsg struct {
	From    string `json:"from,omitempty"`
	To      string `json:"to,omitempty"`
	Data    []byte `json:"data,omitempty"`
	Secured bool   `json:"sec,omitempty"`
	Control int    `json:"ctl,omitempty"` // 0: msg, 1: reset Symkey, 2: EOF
	Pipe    bool   `json:"pipe,omitempty"`
}

func (m *RpipeMsg) SymkeyName() string {
	return m.From + ":" + m.To
}
func (m *RpipeMsg) Marshal() []byte {
	j, err := json.Marshal(m)
	if err != nil {
		return nil
	}
	return j
}
func (m *RpipeMsg) NewReturnMsg() *RpipeMsg {
	return &RpipeMsg{From: m.To, To: m.From}
}

func NewMsgFromBytes(s []byte) (*RpipeMsg, error) {
	msg := RpipeMsg{}
	err := json.Unmarshal(s, &msg)
	if err != nil {
		return nil, err
	}
	return &msg, nil
}

type ApplicationMsg struct {
	Name string
	Data []byte
}

func NewApplicationMsg(s []byte) (*ApplicationMsg, error) {
	chunks := bytes.SplitN(s, []byte{':'}, 2)
	if len(chunks) != 2 {
		return nil, errors.New(fmt.Sprintf("Invalid ApplicationMsg format : %s", s))
	}

	msg := ApplicationMsg{
		Name: string(chunks[0]),
		Data: chunks[1],
	}
	return &msg, nil
}
func (m *ApplicationMsg) Encode() []byte {
	return bytes.Join([][]byte{[]byte(m.Name), m.Data}, []byte{':'})
}
