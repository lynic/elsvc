package elsvc

import (
	"context"
	"encoding/json"
	"fmt"
)

const (
	CtxKeyChans  = "chans"
	CtxKeyConfig = "config"
)

const (
	MsgTypeErr  = "msg_error"
	MsgTypeStop = "msg_stop"
)

type MsgBase struct {
	MsgId       string
	MsgFrom     string
	MsgTo       string
	MsgType     string
	MsgRequest  interface{}
	MsgResponse chan interface{}
	TTL         int
}

func (s MsgBase) ID() string {
	return s.MsgId
}

func (s MsgBase) From() string {
	return s.MsgFrom
}

func (s MsgBase) To() string {
	return s.MsgTo
}

func (s MsgBase) Type() string {
	return s.MsgType
}

func (s *MsgBase) SetRequest(req interface{}) error {
	s.MsgRequest = req
	return nil
}

func (s *MsgBase) GetRequest() interface{} {
	return s.MsgRequest
}

func (s *MsgBase) SetResponse(resp interface{}) error {
	// if s.MsgResponse == nil {
	// 	s.MsgResponse = make(chan interface{}, 1)
	// }
	s.MsgResponse <- resp
	return nil
}

func (s *MsgBase) GetResponse() interface{} {
	// if s.MsgResponse == nil {
	// 	s.MsgResponse = make(chan interface{}, 1)
	// }
	return <-s.MsgResponse
}

func (s *MsgBase) Expired() bool {
	return s.TTL <= 0
}

func (s *MsgBase) DeTTL() {
	s.TTL--
}

func NewMsg(msgTo, msgType string) MsgBase {
	msg := MsgBase{
		MsgTo:       msgTo,
		MsgType:     msgType,
		MsgResponse: make(chan interface{}, 1),
		TTL:         defaultChanLength,
	}
	return msg
}

func SendToChan(ctx context.Context, chanName string, msg MessageIntf) error {
	chans := ctx.Value(CtxKeyChans).(map[string]chan interface{})
	if _, ok := chans[chanName]; !ok {
		return fmt.Errorf("no channel %s to send %v", chanName, msg)
	}
	chans[chanName] <- msg
	return nil
}

func SendMsg(ctx context.Context, msg MessageIntf) error {
	chans := ctx.Value(CtxKeyChans).(map[string]chan interface{})
	to := msg.To()
	if _, ok := chans[to]; !ok {
		return fmt.Errorf("no channel %s to send %v", to, msg)
	}
	chans[to] <- msg
	return nil
}

func WaitMsg(ctx context.Context, chanName string) chan interface{} {
	chans := ctx.Value(CtxKeyChans).(map[string]chan interface{})
	if _, ok := chans[chanName]; !ok {
		msg := MsgBase{}
		msg.SetResponse(fmt.Errorf("chan %s not ready yet", chanName))
		ret := make(chan interface{}, 1)
		ret <- msg
		return ret
	}
	return chans[chanName]
}

func GetConfig(ctx context.Context) map[string]interface{} {
	ret := make(map[string]interface{})
	v := ctx.Value(CtxKeyConfig)
	if v == nil {
		return ret
	}
	return v.(map[string]interface{})
}

//LoadConfig load config to a struct
func LoadConfig(ctx context.Context, v interface{}) error {
	conf := ctx.Value(CtxKeyConfig)
	if v == nil {
		return fmt.Errorf("no config in context")
	}
	data, _ := json.Marshal(conf)
	err := json.Unmarshal(data, v)
	if err != nil {
		return err
	}
	return nil
}

func GetChans(ctx context.Context) map[string]chan interface{} {
	return ctx.Value(CtxKeyChans).(map[string]chan interface{})
}

func GetChan(ctx context.Context, chanName string) chan interface{} {
	chans := GetChans(ctx)
	return chans[chanName]
}
