package elsvc

import (
	context "context"
	"encoding/json"

	"github.com/hashicorp/go-plugin"
	"github.com/lynic/elsvc/proto"
	"google.golang.org/grpc"
)

const (
	MsgFuncName   = "func_modulename"
	MsgFuncInit   = "func_init"
	MsgFuncStart  = "func_start"
	MsgFuncStop   = "func_stop"
	MsgStartError = "start_error"
	MsgCtxDone    = "ctx_done"
)

// This is the implementation of plugin.GRPCPlugin so we can serve/consume this.
type GRPCPlugin struct {
	// GRPCPlugin must still implement the Plugin interface
	plugin.Plugin
	// Concrete implementation, written in Go. This is only used for plugins
	// that are written in Go.
	PluginServer *PluginServer
}

func (p *GRPCPlugin) GRPCServer(broker *plugin.GRPCBroker, s *grpc.Server) error {
	p.PluginServer.broker = broker
	proto.RegisterPluginSvcServer(s, p.PluginServer)
	return nil
}

func (p *GRPCPlugin) GRPCClient(ctx context.Context, broker *plugin.GRPCBroker, c *grpc.ClientConn) (interface{}, error) {
	client := proto.NewPluginSvcClient(c)
	return PluginClient{client: client, broker: broker}, nil
}

type PluginClient struct {
	client proto.PluginSvcClient
	broker *plugin.GRPCBroker
}

// func (*PluginServer) Log(ctx context.Context, req *proto.MsgLog) (*proto.MsgResponse, error) {
// 	fmt.Printf("Request: %+v", *req)
// 	return &proto.MsgResponse{}, nil
// }
// func (*PluginServer) Cancel(ctx context.Context, req *proto.MsgRequest) (*proto.MsgResponse, error) {
// 	fmt.Printf("Request: %+v", *req)
// 	return &proto.MsgResponse{}, nil
// }

func ReqMsg(req *proto.MsgRequest) (MsgBase, error) {
	msg := NewMsg(req.To, req.Type)
	msg.MsgFrom = req.From
	msg.MsgId = req.Id
	msg.TTL = int64(req.Ttl)
	msg.SetRequestBytes(req.Request)
	return msg, nil
}

func MsgReq(msg MsgBase) (*proto.MsgRequest, error) {
	ret := &proto.MsgRequest{
		Id:      msg.ID(),
		From:    msg.From(),
		To:      msg.To(),
		Type:    msg.Type(),
		Ttl:     int64(msg.TTL),
		Request: make([]byte, 0),
	}
	if msg.GetRequest() != nil {
		data, err := json.Marshal(msg.GetRequest())
		if err != nil {
			return nil, err
		}
		ret.Request = data
	}
	return ret, nil
}

func RespMsg(resp *proto.MsgResponse) (MsgBase, error) {
	msg := NewMsg(resp.To, resp.Type)
	msg.MsgFrom = resp.From
	msg.MsgId = resp.Id
	msg.SetResponseBytes(resp.Response)
	return msg, nil
}

func MsgResp(msg MsgBase) (*proto.MsgResponse, error) {
	resp := &proto.MsgResponse{}
	resp.Id = msg.ID()
	resp.From = msg.From()
	resp.To = msg.To()
	resp.Type = msg.Type()
	resp.Response = msg.GetResponseBytes()
	return resp, nil
}

func HandshakeConf() plugin.HandshakeConfig {
	return plugin.HandshakeConfig{
		ProtocolVersion:  1,
		MagicCookieKey:   "EL_GRPCPLUGIN",
		MagicCookieValue: "laiyakuaihuoa",
	}
}
