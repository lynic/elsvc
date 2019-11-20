package elsvc

import "context"

type PluginIntf interface {
	ModuleName() string
	Init(context.Context) error
	Start(context.Context) error
	Stop(context.Context) error
}

type MessageIntf interface {
	To() string
	From() string
	ID() string
	Type() string
	SetRequest(interface{}) error
	GetRequest() interface{}
	SetResponse(interface{}) error
	GetResponse() interface{}
}
