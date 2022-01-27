package messages

import "encoding/json"

type Msg struct {
	From    string `json:"from,omitempty"`
	To      string `json:"to,omitempty"`
	Data    string `json:"data,omitempty"`
	Secured bool   `json:"sec,omitempty"`
	Control int    `json:"ctl,omitempty"` // 0: msg, 1: reset Symkey, 2: EOF
}

func (m *Msg) SymkeyName() string {
	return m.From + ":" + m.To
}
func (m *Msg) Marshal() string {
	j, err := json.Marshal(m)
	if err != nil {
		return ""
	}
	return string(j)
}

func NewMsgFromString(s string) (*Msg, error) {
	msg := Msg{}
	err := json.Unmarshal([]byte(s), &msg)
	if err != nil {
		return nil, err
	}
	return &msg, nil
}
