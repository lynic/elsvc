package elsvc

import (
	context "context"
	"encoding/json"
	fmt "fmt"

	"github.com/hashicorp/go-plugin"
)

func StartService(configPath string) error {
	svc := &Service{}
	logger.Info("Init service")
	err := svc.Init(configPath)
	if err != nil {
		logger.Error(err.Error())
		return err
	}
	logger.Info("Start service")
	err = svc.Start()
	if err != nil {
		logger.Error(err.Error())
		return err
	}
	return nil
}

//StartPlugin only used for hcplugin mode
//call this function in main()
func StartPlugin(pl PluginIntf) error {
	//setup logger
	setupLoggerPlugin()
	//load plugin
	pluginMap := map[string]plugin.Plugin{
		PluginMapKey: &GRPCPlugin{
			PluginServer: &pluginServer{
				PluginImpl: pl,
				logger:     NewModLogger(fmt.Sprintf("pluginServer.%s", pl.ModuleName())),
			},
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

func NewMsg(msgTo, msgType string) MsgBase {
	msg := MsgBase{
		MsgTo:       msgTo,
		MsgType:     msgType,
		MsgRequest:  make(map[string]interface{}),
		MsgResponse: make(chan map[string]interface{}, 1),
		TTL:         64,
	}
	return msg
}

func SendMsg(ctx context.Context, msg interface{}) error {
	OutChan(ctx) <- msg
	return nil
}

func OutChan(ctx context.Context) chan interface{} {
	return ctx.Value(CtxKeyOutchan).(chan interface{})
}

func InChan(ctx context.Context) chan interface{} {
	return ctx.Value(CtxKeyInchan).(chan interface{})
}

func GetConfig(ctx context.Context) map[string]interface{} {
	ret := make(map[string]interface{})
	v := ctx.Value(CtxKeyConfig)
	if v == nil {
		return ret
	}
	return v.(map[string]interface{})
}

//LoadConfig load config to a struct
func LoadConfig(ctx context.Context, v interface{}) error {
	conf := ctx.Value(CtxKeyConfig)
	if v == nil {
		return fmt.Errorf("no config in context")
	}
	data, _ := json.Marshal(conf)
	err := json.Unmarshal(data, v)
	if err != nil {
		return err
	}
	return nil
}
