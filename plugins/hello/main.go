package main

import (
	"context"
	"os"

	"github.com/lynic/elsvc"
)

// CGO_ENABLED=linux CGO_ENABLED=0 go build -buildmode=plugin -o hello.so plugins/hello/main.go
// CGO_ENABLED=linux CGO_ENABLED=0 go build -o hello.so plugins/hello/main.go

var PluginObj Hello

const ModuleName = "hello"

type Hello struct {
	Name string `json:"name"`
}

func (s Hello) ModuleName() string {
	return ModuleName
}

func (s *Hello) Init(ctx context.Context) error {
	elsvc.Info("env helloenv is %s", os.Getenv("helloenv"))
	err := elsvc.LoadConfig(ctx, s)
	if err != nil {
		return err
	}
	elsvc.Info("Hello.%s init", s.Name)
	return nil
}

func (s *Hello) Start(ctx context.Context) error {
	elsvc.Info("Hello.%s start", s.Name)
	// send msg to self channel
	msg := elsvc.NewMsg(s.ModuleName(), "hello_printname")
	msg.MsgTo = s.ModuleName()
	elsvc.SendMsg(ctx, msg)
	for {
		select {
		case <-ctx.Done():
			elsvc.Info("Hello.%s start quit", s.Name)
			return nil
		case v := <-elsvc.InChan(ctx):
			msg, ok := v.(elsvc.MsgBase)
			if !ok {
				elsvc.Error("receive invalid msg %+v", v)
			}
			switch msg.Type() {
			case "hello_printname":
				elsvc.Info("hello, my name is %s", s.Name)
				continue
			default:
				elsvc.Error("receive msg with unknown type: %+v", msg)
			}

		}
	}
}

func (s *Hello) Stop(context.Context) error {
	elsvc.Info("Hello.%s stop", s.Name)
	return nil
}

func init() {
	PluginObj = Hello{}
}

//main() only needed for plugin_mode=hcplugin
func main() {
	elsvc.StartPlugin(&PluginObj)
}
