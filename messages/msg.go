package messages

import "encoding/json"

type Msg struct {
	From     string `json:"from"`
	To       string `json:"to"`
	Data     string `json:"data"`
	Secured  bool   `json:"sec"`
	Refresh  bool   `json:"ref"`
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
