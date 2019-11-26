package main

import (
	"context"

	"github.com/lynic/elsvc"
)

// go build -buildmode=plugin -o hello.so plugins/hello/hello.go
// go build -o hello.so plugins/hello2/main.go

var PluginObj Hello

// var logger = elsvc.NewModLogger("hello")

const ModuleName = "hello"

type Hello struct {
	Name string `json:"name"`
}

func (s Hello) ModuleName() string {
	return ModuleName
}

func (s *Hello) Init(ctx context.Context) error {
	err := elsvc.LoadConfig(ctx, s)
	if err != nil {
		return err
	}
	elsvc.Info("Hello.%s init", s.Name)
	return nil
}

func (s *Hello) Start(ctx context.Context) error {
	elsvc.Info("Hello.1 start")
	// send msg to self channel
	msg := elsvc.NewMsg(s.ModuleName(), "hello_printname")
	msg.MsgTo = s.ModuleName()
	// msg.MsgType = ""
	elsvc.SendMsg(ctx, msg)
	for {
		select {
		case <-ctx.Done():
			elsvc.Info("Hello.1 start quit")
			return nil
		case v := <-elsvc.InChan(ctx):
			_, ok := v.(elsvc.MsgBase)
			if !ok {
				elsvc.Error("receive invalid msg %+v", v)
			}
			elsvc.Info("hello, my name is %s", s.Name)
		}
	}
}

func (s *Hello) Stop(context.Context) error {
	elsvc.Info("Hello.1 stop")
	return nil
}

func init() {
	// fmt.Println("in Hello plugin init")
	PluginObj = Hello{}
}

//main() only needed for plugin_mode=hcplugin
func main() {
	// fmt.Println("in Hello plugin main")
	elsvc.StartPlugin(&PluginObj)
}
