package elsvc

import (
	context "context"
	"encoding/json"
	fmt "fmt"

	"github.com/hashicorp/go-plugin"
	"github.com/lynic/elsvc/proto"
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc"
)

func StartPlugin(pl PluginIntf) error {
	//load plugin
	pluginMap := map[string]plugin.Plugin{
		PluginMapKey: &GRPCPlugin{
			PluginServer: &PluginServer{PluginImpl: pl},
		},
	}
	plugin.Serve(&plugin.ServeConfig{
		HandshakeConfig: HandshakeConf(),
		Plugins:         pluginMap,
		// A non-nil value here enables gRPC serving for this plugin...
		GRPCServer: plugin.DefaultGRPCServer,
	})
	return nil
}

type PluginServer struct {
	PluginImpl  PluginIntf
	broker      *plugin.GRPCBroker
	cancelStart context.CancelFunc
	conn        *grpc.ClientConn
	client      proto.PluginSvcClient
	chans       map[string]chan interface{}
}

func (s *PluginServer) handler(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return nil
		// if this msg wants to send out
		// send from grpc tunnel
		case v := <-s.chans[ChanKeyService]:
			msg := v.(MsgBase)
			req, err := MsgReq(msg)
			if err != nil {
				logrus.Errorf("failed to convert %+v to pbReq: %s", msg, err.Error())
				continue
			}
			resp, err := s.client.Request(context.Background(), req)
			if err != nil {
				logrus.Errorf("failed to to get response from req: %s", err.Error())
				continue
			}
			msg.SetRequestBytes(resp.Response)
		}
	}
	return nil
}

func (s *PluginServer) startWrapper(ctx context.Context) error {
	err := s.PluginImpl.Start(ctx)
	msg := NewMsg(s.PluginImpl.ModuleName(), MsgStartError)
	msg.SetRequest(map[string]interface{}{"error": err})
	req, _ := MsgReq(msg)
	_, err = s.client.Request(context.Background(), req)
	if err != nil {
		return err
	}
	return nil
}

func (s *PluginServer) Request(ctx context.Context, req *proto.MsgRequest) (*proto.MsgResponse, error) {
	fmt.Printf("Request: %+v", *req)
	switch req.Type {
	case MsgFuncName:
		name := s.PluginImpl.ModuleName()
		msg := NewMsg(name, req.Type)
		msg.SetResponse(map[string]interface{}{"name": name})
		resp, err := MsgResp(msg)
		if err != nil {
			return nil, err
		}
		return resp, nil
	case MsgFuncInit:
		resp := &proto.MsgResponse{}
		conf := &PluginConfig{}
		err := json.Unmarshal(req.Request, conf)
		if err != nil {
			data, _ := json.Marshal(map[string]interface{}{"error": err.Error()})
			resp.Response = data
			return resp, nil
		}
		ctx := context.WithValue(context.Background(), CtxKeyConfig, conf)
		err = s.PluginImpl.Init(ctx)
		if err != nil {
			data, _ := json.Marshal(map[string]interface{}{"error": err.Error()})
			resp.Response = data
			return resp, nil
		}
		data, _ := json.Marshal(map[string]interface{}{"error": ""})
		resp.Response = data
		return resp, nil
	case MsgFuncStart:
		msg, _ := ReqMsg(req)
		// create bidirection grpc connection
		brokerID := uint32(msg.GetRequest()["brokerID"].(float64))
		// return &proto.MsgResponse{Response: []byte(fmt.Sprintf("{\"brokerid\": \"%s\"}", brokerID))}, nil
		conn, err := s.broker.Dial(brokerID)
		if err != nil {
			return nil, err
		}
		s.conn = conn
		s.client = proto.NewPluginSvcClient(conn)

		// create chans for plugin
		s.chans = make(map[string]chan interface{})
		s.chans[s.PluginImpl.ModuleName()] = make(chan interface{}, defaultChanLength)
		s.chans[ChanKeyService] = make(chan interface{}, defaultChanLength)

		// create ctx for start
		ctx, cancel := context.WithCancel(context.WithValue(context.Background(), CtxKeyChans, s.chans))
		s.cancelStart = cancel
		// start chan handler
		go s.handler(ctx)
		// go plugin.start here, result will be send through MsgStartError
		go s.startWrapper(ctx)
		return &proto.MsgResponse{}, nil
	case MsgFuncStop:
		err := s.PluginImpl.Stop(context.Background())
		resp := &proto.MsgResponse{}
		if err != nil {
			data, _ := json.Marshal(map[string]interface{}{"error": err.Error()})
			resp.Response = data
			return resp, nil
		}
		data, _ := json.Marshal(map[string]interface{}{"error": ""})
		resp.Response = data
		return resp, nil
	case MsgCtxDone:
		// cancel from start
		s.cancelStart()
		s.conn.Close()
	}
	return &proto.MsgResponse{}, nil
}
