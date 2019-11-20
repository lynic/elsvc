package main

import (
	"context"
	"fmt"
)

var PluginObj Hello

type Hello struct{}

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
	fmt.Println("in Hello plugin init")
	PluginObj = Hello{}
}
