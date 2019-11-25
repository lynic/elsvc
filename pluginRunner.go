package elsvc

import (
	context "context"
	fmt "fmt"
	"os/exec"

	"github.com/hashicorp/go-plugin"
	"github.com/lynic/elsvc/proto"
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc"
)

const PluginMapKey = "elplugin"

type PluginRunner struct {
	PluginName   string
	svcClient    proto.PluginSvcClient
	broker       *plugin.GRPCBroker
	chanRPC      *grpc.Server
	pluginClient *plugin.Client
	binaryPath   string
	recvChan     chan interface{}
}

func (s *PluginRunner) Load(pc PluginConfig) error {
	//find binary
	binaryPath := FindLatestSO(pc.Type, pc.PluginPath())
	if binaryPath == "" {
		return fmt.Errorf("cound't find plugin binary for %s", pc.Type)
	}
	s.binaryPath = binaryPath

	//load plugin
	pluginMap := map[string]plugin.Plugin{
		PluginMapKey: &GRPCPlugin{},
	}

	client := plugin.NewClient(&plugin.ClientConfig{
		HandshakeConfig: HandshakeConf(),
		Plugins:         pluginMap,
		Cmd:             exec.Command(binaryPath),
		// Logger:           logger,
		AllowedProtocols: []plugin.Protocol{plugin.ProtocolGRPC},
	})
	s.pluginClient = client

	// Connect via RPC
	rpcClient, err := client.Client()
	if err != nil {
		return err
	}
	// Request the plugin
	raw, err := rpcClient.Dispense(PluginMapKey)
	if err != nil {
		return err
	}
	pluginClient := raw.(PluginClient)
	s.svcClient = pluginClient.client
	s.broker = pluginClient.broker
	s.recvChan = make(chan interface{}, defaultChanLength)
	return nil
}

func (s PluginRunner) Name() string {
	if s.PluginName != "" {
		return s.PluginName
	}
	msg := NewMsg("", MsgFuncName)
	req, err := MsgReq(msg)
	if err != nil {
		return ""
	}
	resp, err := s.svcClient.Request(context.Background(), req)
	if err != nil {
		return ""
	}
	rmsg, _ := RespMsg(resp)
	s.PluginName = rmsg.GetResponse()["name"].(string)
	return s.PluginName
}

func (s *PluginRunner) Init(ctx context.Context) error {
	conf := GetConfig(ctx)
	msg := NewMsg(s.Name(), MsgFuncInit)
	msg.SetRequest(conf)
	req, err := MsgReq(msg)
	if err != nil {
		return err
	}
	resp, err := s.svcClient.Request(context.Background(), req)
	if err != nil {
		return err
	}
	rmsg, _ := RespMsg(resp)
	if rmsg.GetResponse()["error"] != nil {
		return msg.GetResponse()["error"].(error)
	}
	return nil
}

//Receive msg from plugin
func (s *PluginRunner) Request(ctx context.Context, req *proto.MsgRequest) (*proto.MsgResponse, error) {
	fmt.Printf("Request: %+v", *req)
	switch req.Type {
	case MsgStartError:
		msg := NewMsg(req.To, req.Type)
		if len(req.Request) == 0 {
			msg.SetResponse(map[string]interface{}{})
		} else {
			msg.SetResponse(map[string]interface{}{"error": fmt.Errorf(string(req.Request))})
		}
		s.recvChan <- msg
	}
	return &proto.MsgResponse{}, nil
}

//receiver Func
func (s *PluginRunner) serverFunc(opts []grpc.ServerOption) *grpc.Server {
	s.chanRPC = grpc.NewServer(opts...)
	proto.RegisterPluginSvcServer(s.chanRPC, s)
	return s.chanRPC
}

func (s *PluginRunner) Start(ctx context.Context) error {
	//start rpcChan
	brokerID := s.broker.NextId()
	go s.broker.AcceptAndServe(brokerID, s.serverFunc)

	//run plugin.start
	msg := NewMsg(s.Name(), MsgFuncStart)
	msg.SetRequest(map[string]interface{}{"brokerID": brokerID})
	req, err := MsgReq(msg)
	if err != nil {
		return err
	}
	// will hang here?
	resp, err := s.svcClient.Request(context.Background(), req)
	if err != nil {
		return err
	}
	// Response is error message
	rmsg, _ := RespMsg(resp)
	if rmsg.GetResponse()["error"] != nil {
		return rmsg.GetResponse()["error"].(error)
	}
	for {
		select {
		case <-ctx.Done():
			msg := NewMsg(s.Name(), MsgCtxDone)
			req, _ := MsgReq(msg)
			_, err := s.svcClient.Request(context.Background(), req)
			if err != nil {
				logrus.Errorf("failed to send %v for plugin %s", req, s.Name())
			}
			return nil
		case v := <-s.recvChan:
			switch v.(type) {
			case MsgBase:
				msg := v.(MsgBase)
				switch msg.Type() {
				case MsgStartError:
					if msg.GetResponse() == nil {
						return nil
					}
					return msg.GetResponse()["error"].(error)
				}
			}
		}
	}
	return nil
}

func (s *PluginRunner) Stop(ctx context.Context) error {
	//run plugin.stop
	msg := NewMsg(s.Name(), MsgFuncStop)
	req, err := MsgReq(msg)
	if err != nil {
		return err
	}
	resp, err := s.svcClient.Request(context.Background(), req)
	if len(resp.Response) != 0 {
		return fmt.Errorf("%s", string(resp.Response))
	}
	//stop chanRPC
	if s.chanRPC != nil {
		s.chanRPC.Stop()
	}
	//stop plugin process
	s.pluginClient.Kill()
	return nil
}