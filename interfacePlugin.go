package elsvc

import "context"

type PluginIntf interface {
	ModuleName() string
	Init(context.Context) error
	Start(context.Context) error
	Stop(context.Context) error
}

// type MessageIntf interface {
// 	To() string
// 	From() string
// 	ID() string
// 	Type() string
// 	GetRequest() map[string]interface{}
// 	SetRequest(map[string]interface{}) error
// 	GetResponse() map[string]interface{}
// 	SetResponse(map[string]interface{}) error
// }
