package elsvc

import (
	context "context"
	"encoding/json"
	fmt "fmt"

	"github.com/sirupsen/logrus"
)

func StartGOPlugin(configPath string) error {
	svc := &Service{}
	logrus.Info("init service")
	err := svc.Init(configPath)
	if err != nil {
		logrus.Error(err)
		return err
	}
	logrus.Info("start service")
	err = svc.Start()
	if err != nil {
		logrus.Error(err)
		return err
	}
	return nil
}

func NewMsg(msgTo, msgType string) MsgBase {
	msg := MsgBase{
		MsgTo:       msgTo,
		MsgType:     msgType,
		MsgRequest:  make(map[string]interface{}),
		MsgResponse: make(chan map[string]interface{}, 1),
		TTL:         64,
	}
	return msg
}

func SendMsg(ctx context.Context, msg MessageIntf) error {
	chans := ctx.Value(CtxKeyChans).(map[string]chan interface{})
	to := msg.To()
	if _, ok := chans[to]; !ok {
		if _, exist := chans[ChanKeyService]; !exist {
			return fmt.Errorf("no channel %s to send %v", to, msg)
		}
		chans[ChanKeyService] <- msg
		return nil
	}
	chans[to] <- msg
	return nil
}

func WaitMsg(ctx context.Context, chanName string) chan interface{} {
	chans := ctx.Value(CtxKeyChans).(map[string]chan interface{})
	if _, ok := chans[chanName]; !ok {
		msg := MsgBase{}
		resp := map[string]interface{}{
			"error": fmt.Errorf("chan %s not ready yet", chanName),
		}
		msg.SetResponse(resp)
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

// func GetChans(ctx context.Context) map[string]chan interface{} {
// 	return ctx.Value(CtxKeyChans).(map[string]chan interface{})
// }

// func GetChan(ctx context.Context, chanName string) chan interface{} {
// 	chans := GetChans(ctx)
// 	return chans[chanName]
// }
