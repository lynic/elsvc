package elsvc

import (
	context "context"
	fmt "fmt"
	"os/exec"

	"github.com/hashicorp/go-plugin"
	"github.com/lynic/elsvc/proto"
	"google.golang.org/grpc"
)

const PluginMapKey = "elplugin"

type pluginRunner struct {
	PluginName   string
	svcClient    proto.PluginSvcClient
	broker       *plugin.GRPCBroker
	chanRPC      *grpc.Server
	pluginClient *plugin.Client
	binaryPath   string
	logger       *Logger
	recvChan     chan interface{} // receive msg from pluginserver
	// pluginConfig PluginConfig
}

func (s *pluginRunner) Load(pc PluginConfig) error {
	s.logger = NewModLogger(fmt.Sprintf("pluginRunner.%s", pc.Type))
	// s.pluginConfig = pc
	//find binary
	binaryPath := findLatestSO(pc.Type, pc.PluginPath())
	if binaryPath == "" {
		return fmt.Errorf("couldn't find plugin binary for %s", pc.Type)
	}
	s.binaryPath = binaryPath

	//load plugin
	pluginMap := map[string]plugin.Plugin{
		PluginMapKey: &GRPCPlugin{},
	}

	client := plugin.NewClient(&plugin.ClientConfig{
		HandshakeConfig:  HandshakeConf(),
		Plugins:          pluginMap,
		Cmd:              exec.Command(binaryPath),
		Logger:           NewModLogger("hcplugin").hclogger,
		AllowedProtocols: []plugin.Protocol{plugin.ProtocolGRPC},
	})
	s.pluginClient = client

	// Connect via RPC
	rpcClient, err := client.Client()
	if err != nil {
		s.logger.Error("failed to get rpcClient: %v", err)
		return err
	}
	// Request the plugin
	raw, err := rpcClient.Dispense(PluginMapKey)
	if err != nil {
		s.logger.Error("failed to request the plugin: %v", err)
		return err
	}
	pluginClient := raw.(PluginClient)
	s.svcClient = pluginClient.client
	s.broker = pluginClient.broker
	s.recvChan = make(chan interface{}, defaultChanLength)
	// s.logger = NewModLogger(fmt.Sprintf("pluginRunner.%s", s.Name()))
	// set env
	for k, v := range pc.EnvMap {
		err := s.SetEnv(k, v)
		if err != nil {
			s.logger.Error("failed to setenv '%s: %s': %v", k, v, err)
			return err
		}
	}
	return nil
}

func (s pluginRunner) SetEnv(key, value string) error {
	msg := NewMsg("", MsgSetEnv)
	msg.SetRequest(map[string]interface{}{
		"key":   key,
		"value": value,
	})
	req, err := msgReq(msg)
	if err != nil {
		return err
	}
	resp, err := s.svcClient.Request(context.Background(), req)
	if err != nil {
		return err
	}
	rmsg, err := respMsg(resp)
	if err != nil {
		return err
	}
	if v, ok := rmsg.GetResponse()["error"]; ok && v != nil {
		return v.(error)
	}
	return nil
}

func (s *pluginRunner) Name() string {
	if s.PluginName != "" {
		return s.PluginName
	}
	msg := NewMsg("", MsgFuncName)
	req, err := msgReq(msg)
	if err != nil {
		return ""
	}
	resp, err := s.svcClient.Request(context.Background(), req)
	if err != nil {
		return ""
	}
	rmsg, _ := respMsg(resp)
	s.PluginName = rmsg.GetResponse()["name"].(string)
	return s.PluginName
}

func (s *pluginRunner) Init(ctx context.Context) error {
	conf := GetConfig(ctx)
	msg := NewMsg(s.Name(), MsgFuncInit)
	msg.SetRequest(conf)
	req, err := msgReq(msg)
	if err != nil {
		return err
	}
	resp, err := s.svcClient.Request(context.Background(), req)
	if err != nil {
		return err
	}
	rmsg, _ := respMsg(resp)
	errInt := rmsg.GetResponse()["error"]
	if errInt != nil {
		return errInt.(error)
	}
	return nil
}

//receiver Func
func (s *pluginRunner) serverFunc(opts []grpc.ServerOption) *grpc.Server {
	s.chanRPC = grpc.NewServer(opts...)
	proto.RegisterPluginSvcServer(s.chanRPC, s)
	return s.chanRPC
}

//Receive msg from pluginserver, and send to recvChan for further use
func (s *pluginRunner) Request(ctx context.Context, req *proto.MsgRequest) (*proto.MsgResponse, error) {
	s.logger.Debug("Recv request from pluginServer: %v", req)
	switch req.Type {
	case MsgStartError:
		msg, err := reqMsg(req)
		if err != nil {
			s.logger.Error("failed to convert req %+v: %v", req, err)
		}
		msg.SetResponse(map[string]interface{}{
			"plugin": s.Name(),
			"error":  msg.GetRequest()["error"].(error),
		})
		s.recvChan <- msg
	default:
		msg, err := reqMsg(req)
		if err != nil {
			s.logger.Error("failed to convert req %+v: %v", req, err)
		}
		// SendMsg(s.ctx, msg)
		s.recvChan <- msg
	}
	return &proto.MsgResponse{}, nil
}

//Receive msg from chan and send to plugin
func (s *pluginRunner) chanHandler(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			msg := NewMsg(s.Name(), MsgCtxDone)
			req, _ := msgReq(msg)
			_, err := s.svcClient.Request(context.Background(), req)
			if err != nil {
				s.logger.Error("failed to send %v for plugin %s", req, s.Name())
			}
			return nil
		case v := <-InChan(ctx):
			s.logger.Debug("Recv msg from inChan: %+v", v)
			// handle message to send to pluginserver
			msg, ok := v.(MsgBase)
			if !ok {
				s.logger.Error("failed to convert req: %+v", v)
				continue
			}
			req, _ := msgReq(msg)
			_, err := s.svcClient.Request(context.Background(), req)
			if err != nil {
				s.logger.Error("failed to send msg to pluginserver: %v", err)
			}
		case v := <-s.recvChan:
			// handler message from pluginserver
			// MsgStartError is for pluginrunner
			// other message should route to outChan
			msg, ok := v.(MsgBase)
			if !ok {
				s.logger.Error("[%s] !!Check!! recvChan invalid msg: %+v", s.Name(), v)
				continue
			}
			switch msg.Type() {
			case MsgStartError:
				err := msg.GetResponse()["error"].(error)
				s.logger.Debug("plugin %s start() return: %v", s.Name(), err)
				// send out message to service
				msg.MsgTo = ChanKeyService
				SendMsg(ctx, msg)
			default:
				if msg.To() == s.Name() {
					s.logger.Error("recv an unknown message %+v", msg)
					continue
				}
				s.logger.Debug("routing msg '%+v' for plugin %s", msg, s.Name())
				err := SendMsg(ctx, msg)
				if err != nil {
					s.logger.Error("failed to send '%+v' for plugin %s", msg, s.Name())
				}
			}
		}
	}
	return nil
}

//Start send start request to pluginserver
func (s *pluginRunner) Start(ctx context.Context) error {
	//start rpcChan
	brokerID := s.broker.NextId()
	go s.broker.AcceptAndServe(brokerID, s.serverFunc)
	// s.inChan = InChan(ctx)
	// s.outChan = OutChan(ctx)

	//run plugin.start
	msg := NewMsg(s.Name(), MsgFuncStart)
	msg.SetRequest(map[string]interface{}{"brokerID": brokerID})
	req, err := msgReq(msg)
	if err != nil {
		return err
	}
	// send start plugin request
	resp, err := s.svcClient.Request(context.Background(), req)
	if err != nil {
		return err
	}
	// Response is error message
	rmsg, _ := respMsg(resp)
	errInt := rmsg.GetResponse()["error"]
	if errInt != nil {
		return errInt.(error)
	}
	// start chanHandler
	go s.chanHandler(ctx)
	return nil
}

func (s *pluginRunner) Stop(ctx context.Context) error {
	//run plugin.stop
	msg := NewMsg(s.Name(), MsgFuncStop)
	req, err := msgReq(msg)
	if err != nil {
		return err
	}
	resp, err := s.svcClient.Request(context.Background(), req)
	rmsg, _ := respMsg(resp)
	errIntf := rmsg.GetResponse()["error"]
	if errIntf != nil {
		return errIntf.(error)
	}
	//stop chanRPC
	if s.chanRPC != nil {
		s.chanRPC.Stop()
	}
	//stop plugin process
	s.pluginClient.Kill()
	return nil
}
