package main

import (
	"context"
	"fmt"

	"github.com/lynic/elsvc"
)

// go build -buildmode=plugin -o hello.so plugins/hello/hello.go
// go build -o hello.so plugins/hello2/main.go

var PluginObj Hello

const ModuleName = "hello"

type Hello struct{}

func (s Hello) ModuleName() string {
	return ModuleName
}

func (s *Hello) Init(context.Context) error {
	fmt.Println("Hello.1 init")
	return nil
}

func (s *Hello) Start(ctx context.Context) error {
	fmt.Println("Hello.1 start")
	select {
	case <-ctx.Done():
		fmt.Println("Hello.1 start quit")
		return nil
	}
}

func (s *Hello) Stop(context.Context) error {
	fmt.Println("Hello.1 stop")
	return nil
}

func init() {
	// fmt.Println("in Hello plugin init")
	PluginObj = Hello{}
}

func main() {
	// fmt.Println("in Hello plugin main")
	elsvc.StartPlugin(&PluginObj)
}
