package elsvc

import (
	context "context"
	"encoding/json"
	"os"

	"github.com/hashicorp/go-plugin"
	"github.com/lynic/elsvc/proto"
	"google.golang.org/grpc"
)

type pluginServer struct {
	PluginImpl  PluginIntf
	broker      *plugin.GRPCBroker
	cancelStart context.CancelFunc
	conn        *grpc.ClientConn
	client      proto.PluginSvcClient
	chans       map[string]chan interface{}
	logger      *Logger
}

func (s *pluginServer) handler(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return nil
		// if this msg wants to send out
		// send from grpc tunnel
		case v := <-s.chans[ChanKeyService]:
			s.logger.Debug("Recv msg from outchan %+v", v)
			msg, ok := v.(MsgBase)
			if !ok {
				s.logger.Error("failed to convert to MsgBase: %+v", v)
				continue
			}
			req, err := msgReq(msg)
			if err != nil {
				s.logger.Error("failed to convert %+v to pbReq: %s", msg, err.Error())
				continue
			}
			resp, err := s.client.Request(context.Background(), req)
			if err != nil {
				s.logger.Error("failed to to get response from req: %s", err.Error())
				continue
			}
			msg.SetRequestBytes(resp.Response)
		}
	}
	return nil
}

func (s *pluginServer) startWrapper(ctx context.Context) error {
	err := s.PluginImpl.Start(ctx)
	msg := NewMsg(s.PluginImpl.ModuleName(), MsgStartError)
	msg.SetRequest(map[string]interface{}{"error": err})
	req, _ := msgReq(msg)
	_, err = s.client.Request(context.Background(), req)
	if err != nil {
		return err
	}
	return nil
}

//Receive message from pluginRunner
func (s *pluginServer) Request(ctx context.Context, req *proto.MsgRequest) (*proto.MsgResponse, error) {
	switch req.Type {
	case MsgFuncName:
		s.logger.Debug("recv MsgFuncName request %+v", req)
		name := s.PluginImpl.ModuleName()
		msg := NewMsg(name, req.Type)
		msg.SetResponse(map[string]interface{}{"name": name})
		resp, err := msgResp(msg)
		if err != nil {
			return nil, err
		}
		s.logger.Debug("send MsgFuncName response %+v", resp)
		return resp, nil
	case MsgSetEnv:
		s.logger.Debug("recv MsgSetEnv request %+v", req)
		envKV := make(map[string]string)
		err := json.Unmarshal(req.Request, &envKV)
		if err != nil {
			s.logger.Error("failed to convert '%+v' to msg: %v", req, err)
		}
		msg := NewMsg(req.To, req.Type)
		err = os.Setenv(envKV["key"], envKV["value"])
		if err != nil {
			s.logger.Error("failed to setenv '%+v' to msg: %v", envKV, err)
		}
		msg.SetResponse(map[string]interface{}{"error": err})
		resp, err := msgResp(msg)
		if err != nil {
			s.logger.Error("failed to convert '%+v' to resp: %v", msg, err)
		}
		return resp, nil
	case MsgFuncInit:
		s.logger.Debug("Recv init req: %+v", req)
		resp := &proto.MsgResponse{}
		conf := make(map[string]interface{})
		err := json.Unmarshal(req.Request, &conf)
		if err != nil {
			data, _ := json.Marshal(map[string]interface{}{"error": err.Error()})
			resp.Response = data
			return resp, nil
		}
		s.logger.Debug("Init config content: %+v", conf)
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
		s.logger.Debug("Recv start req: %+v", req)
		msg, _ := reqMsg(req)
		// create bidirection grpc connection
		brokerID := uint32(msg.GetRequest()["brokerID"].(float64))
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
		ctx := context.WithValue(context.Background(), CtxKeyInchan, s.chans[s.PluginImpl.ModuleName()])
		ctx = context.WithValue(ctx, CtxKeyOutchan, s.chans[ChanKeyService])
		ctx, cancel := context.WithCancel(ctx)
		s.cancelStart = cancel
		// start chan handler
		go s.handler(ctx)
		// go plugin.start here, result will be send through MsgStartError
		go s.startWrapper(ctx)
		return &proto.MsgResponse{}, nil
	case MsgFuncStop:
		s.logger.Debug("Recv stop req: %+v", req)
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
		s.logger.Debug("Recv ctxDone req: %+v", req)
		// cancel from start
		s.cancelStart()
		s.conn.Close()
	default:
		s.logger.Debug("Recv req to inChan: %+v", req)
		// receive inchan message
		msg, err := reqMsg(req)
		if err != nil {
			err := s.logger.Error("failed to convert req: %+v", req)
			resp := &proto.MsgResponse{}
			data, _ := json.Marshal(map[string]interface{}{"error": err.Error()})
			resp.Response = data
			return resp, nil
		}
		s.chans[s.PluginImpl.ModuleName()] <- msg
	}
	return &proto.MsgResponse{}, nil
}
