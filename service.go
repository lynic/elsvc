package elsvc

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/ghodss/yaml"
	"github.com/pkg/errors"
)

const (
	defaultChanLength = 1000
	ChanKeyService    = "common"
	minGoroutineNum   = 3 //1 for go-plugin, 1 for service, 1 for exitSignal
)

const (
	PluginModeGO = "goplugin"
	PluginModeHC = "hcplugin"
)

const (
	RunModeJob = "job"
	RunModeSvc = "service"
)

const (
	MsgConfigReload = "reload_config"
	MsgUnloadPlugin = "unload_plugin"
	MsgLoadPlugin   = "load_plugin"
	MsgListPlugins  = "list_plugins"
)

type PluginConfig struct {
	Type      string                 `json:"type"`
	PluginDir string                 `json:"plugin_path"`
	ConfMap   map[string]interface{} `json:"config"`
	EnvMap    map[string]string      `json:"env"`
}

func (s PluginConfig) PluginPath() string {
	if s.PluginDir == "" {
		return "./"
	}
	return s.PluginDir
}

func (s PluginConfig) Config() map[string]interface{} {
	if s.ConfMap == nil {
		return make(map[string]interface{})
	}
	return s.ConfMap
}

// func (s PluginConfig) ChanLen() int {
// 	// TODO should I make it default to 1?
// 	if s.ChanLength == 0 {
// 		return defaultChanLength
// 	}
// 	return s.ChanLength
// }

type ServiceConfig struct {
	LogLevel   string         `json:"log_level"`
	PluginMode string         `json:"plugin_mode"`
	RunMode    string         `json:"run_mode"`
	Plugins    []PluginConfig `json:"plugins"`
}

type Service struct {
	LoadedPlugins map[string]PluginLoaderIntf // since plugin couldn't unload, reload plugin will lookup here
	Plugins       map[string]PluginLoaderIntf
	Chans         map[string]chan interface{}
	cancelFuncs   map[string]context.CancelFunc
	config        *ServiceConfig
	logger        *Logger
}

func (s *Service) GetChan(name string, opts ...interface{}) chan interface{} {
	if _, ok := s.Chans[name]; !ok {
		chanLength := defaultChanLength
		if len(opts) != 0 {
			if v, ok := opts[0].(int); ok {
				chanLength = v
			}
		}
		s.Chans[name] = make(chan interface{}, chanLength)
	}
	return s.Chans[name]
}

func (s *Service) LoadConfig(configPath string) error {
	// load passed config file first
	if configPath == "" {
		// if empty, load env passed config file
		configPath = os.Getenv("CONFIGPATH")
		if configPath == "" {
			// if both empty, return err
			return fmt.Errorf("env CONFIGPATH is empty")
		}
	}
	data, err := ioutil.ReadFile(configPath)
	if err != nil {
		return err
	}
	confObj := &ServiceConfig{}
	err = yaml.Unmarshal(data, confObj)
	if err != nil {
		return err
	}
	//pluginMode
	if confObj.PluginMode == "" {
		confObj.PluginMode = "goplugin"
	}
	if confObj.PluginMode != PluginModeGO && confObj.PluginMode != PluginModeHC {
		return fmt.Errorf("Plugin mode %s is neither %s nor %s", confObj.PluginMode, PluginModeGO, PluginModeHC)
	}
	//runMode
	if confObj.RunMode == "" {
		confObj.RunMode = RunModeSvc
	}
	if confObj.RunMode != RunModeSvc && confObj.RunMode != RunModeJob {
		return fmt.Errorf("Run mode %s is neither %s nor %s", confObj.RunMode, RunModeSvc, RunModeJob)
	}
	//logLevel
	if confObj.LogLevel == "" {
		confObj.LogLevel = LogDebugLevel
	}
	err = logger.SetLogLevel(confObj.LogLevel)
	if err != nil {
		logger.Error("failed to set logLevel to %s: %v", confObj.LogLevel, err)
		return err
	}
	s.config = confObj
	return nil
}

func (s *Service) InitPlugin(pc PluginConfig) error {
	// init plugin
	s.logger.Info("Initing plugin %s", pc.Type)
	ctx := context.WithValue(
		context.Background(), CtxKeyConfig, pc.Config())
	pl := s.Plugins[pc.Type]
	err := pl.Init(ctx)
	if err != nil {
		return err
	}
	s.logger.Info("Inited plugin %s", pc.Type)
	return nil
}

func (s *Service) LoadPlugin(pc PluginConfig) (PluginLoaderIntf, error) {
	pluginPath := findLatestSO(pc.Type, pc.PluginPath())
	if pluginPath == "" {
		return nil, fmt.Errorf("failed to find plugin %s in %s", pc.Type, pc.PluginPath())
	}
	s.logger.Info("Loading plugin %s", pluginPath)
	var pl PluginLoaderIntf
	switch s.config.PluginMode {
	case PluginModeGO:
		// if plugin loaded
		if _, ok := s.LoadedPlugins[pluginPath]; ok {
			pl = s.LoadedPlugins[pluginPath]
		} else {
			// plugin not loaded yet
			pl = &pluginLoader{}
			err := pl.Load(pc)
			if err != nil {
				return nil, err
			}
			s.LoadedPlugins[pluginPath] = pl
			// make sure ModuleName is equal with type parsed in
			if pl.Name() != pc.Type {
				return nil, fmt.Errorf("ModuleName %s != plugin type %s", pl.Name(), pc.Type)
			}
		}
	case PluginModeHC:
		pl = &pluginRunner{}
		err := pl.Load(pc)
		if err != nil {
			return nil, err
		}
		// make sure ModuleName is equal with type parsed in
		if pl.Name() != pc.Type {
			return nil, fmt.Errorf("ModuleName %s != plugin type %s", pl.Name(), pc.Type)
		}
	}
	s.Plugins[pl.Name()] = pl
	s.Chans[pl.Name()] = s.GetChan(pl.Name(), defaultChanLength)
	s.logger.Info("Loaded plugin %s", pl.Name())
	return pl, nil
}

func (s *Service) LoadPlugins() error {
	s.logger.Info("loading plugins...")
	for _, pc := range s.config.Plugins {
		_, err := s.LoadPlugin(pc)
		if err != nil {
			return errors.Wrapf(err, "failed load plugin %s", pc.Type)
		}
		err = s.InitPlugin(pc)
		if err != nil {
			return errors.Wrapf(err, "failed init plugin %s", pc.Type)
		}
	}
	return nil
}

func (s *Service) Init(configPath string) error {
	var err error
	s.logger = NewModLogger("service")

	s.LoadedPlugins = make(map[string]PluginLoaderIntf)
	s.Plugins = make(map[string]PluginLoaderIntf)
	s.cancelFuncs = make(map[string]context.CancelFunc)
	// init default chan
	s.Chans = make(map[string]chan interface{})
	s.GetChan(ChanKeyService, defaultChanLength)

	// load config
	err = s.LoadConfig(configPath)
	if err != nil {
		return err
	}

	s.logger.Info("pluginMode: %s", s.config.PluginMode)
	s.logger.Info("logLevel: %s", s.config.LogLevel)

	// load plugins
	err = s.LoadPlugins()
	if err != nil {
		return err
	}
	return nil
}

func (s *Service) StartPlugin(pluginName string) error {
	s.logger.Info("Starting plugin %s", pluginName)
	pl := s.Plugins[pluginName]
	ctx := context.WithValue(context.Background(), CtxKeyInchan, s.Chans[pluginName])
	ctx = context.WithValue(ctx, CtxKeyOutchan, s.Chans[ChanKeyService])
	ctx, cancel := context.WithCancel(ctx)
	s.cancelFuncs[pluginName] = cancel
	err := pl.Start(ctx)
	if err != nil {
		return err
	}
	s.logger.Info("Started plugin %s", pluginName)
	return nil
}

func (s *Service) StartPlugins() error {
	s.logger.Info("Starting Plugins")
	for ptype := range s.Plugins {
		err := s.StartPlugin(ptype)
		if err != nil {
			return errors.Wrapf(err, "failed to start plugin %s", ptype)
		}
		// if in job mode, wait for start return msg
		if s.config.RunMode == RunModeJob {
			select {
			case v := <-s.Chans[ChanKeyService]:
				msg, ok := v.(MsgBase)
				if !ok {
					return fmt.Errorf("failed to convert msg %+v", v)
				}
				switch msg.Type() {
				case MsgStartError:
					err := msg.GetResponse()["error"].(error)
					if err != nil {
						return err
					}
				default:
					return fmt.Errorf("waiting MsgStartError but got %+v", err)
				}
			}
		}

	}
	return nil
}

func (s *Service) signalHandler() {
	c := make(chan os.Signal)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		for sig := range c {
			s.logger.Debug("Receive signal %+v", sig)
			if sig.String() != os.Interrupt.String() {
				continue
			}
			s.UnloadPlugins()
			os.Exit(1)
		}
	}()
}

func (s *Service) startJobMode() error {
	return nil
}

func (s *Service) Start() error {
	// run pluginsjob
	// start worker for kill signal
	go s.signalHandler()
	// run in job mode
	// if s.config.RunMode == RunModeJob {
	// 	s.logger.Info("Start service at job mode")
	// 	return s.startJobMode()
	// }
	// run in service mode
	s.logger.Info("Start service at service mode")
	err := s.StartPlugins()
	if err != nil {
		return err
	}
	for {
		select {
		case v := <-s.Chans[ChanKeyService]:
			// check if received a message
			msg, ok := v.(MsgBase)
			if !ok {
				s.logger.Error("dropping invalid msg %+v", v)
				continue
			}
			// message sent to service for routing
			if msg.To() != ChanKeyService {
				// route message to corresponding chan
				if _, ok := s.Chans[msg.To()]; ok {
					s.logger.Debug("routing msg: %+v", msg)
					s.Chans[msg.To()] <- msg
					continue
				}
				if msg.To() == "" {
					s.logger.Error("dropping invalid msg %+v", v)
					continue
				}
				// channel not ready yet for this message
				// put msg back to queue
				// msg.DeTTL()
				// if msg.Expired() {
				// 	// reach ttl EOL
				// 	// drop msg
				// 	s.logger.Info("drop eol message %v", msg)
				// 	continue
				// }
				//TODO should I directly drop message?
				s.logger.Debug("channel not ready for msg: %+v", msg)
				s.Chans[ChanKeyService] <- msg
			}

			// message sent to controller itself
			switch msg.Type() {
			case MsgTypeStop:
				s.logger.Info("Received stop msg, stopping service")
				if v, ok := msg.GetRequest()["error"]; ok {
					s.logger.Debug("Stop msg with an error: %v", v.(error))
				}
				// if force stop
				if v, ok := msg.GetRequest()["force"]; ok && v.(bool) {
					s.logger.Info("Force quit, Exit...")
					if v, ok := msg.GetRequest()["error"]; ok && v.(error) != nil {
						os.Exit(1)
					}
					os.Exit(0)
				}
				// soft stop
				err := s.Stop()
				msg.SetResponse(map[string]interface{}{"error": err})
				waitGoroutines(minGoroutineNum)
				return msg.GetError()
			case MsgUnloadPlugin:
				pluginName := msg.GetRequest()["name"].(string)
				err := s.UnloadPlugin(pluginName)
				msg.SetResponse(map[string]interface{}{"error": err})
			case MsgListPlugins:
				resp := make(map[string]interface{})
				for pluginName := range s.Plugins {
					resp[pluginName] = true
				}
				msg.SetResponse(resp)
			case MsgLoadPlugin:
				pc := PluginConfig{}
				data, _ := json.Marshal(msg.GetRequest())
				err := json.Unmarshal(data, &pc)
				if err != nil {
					msg.SetResponse(map[string]interface{}{"error": err})
				}
				// LoadConfig(msg.GetRequest()["PluginConfig"], &pc)
				// pc := msg.GetRequest()["PluginConfig"].(PluginConfig)
				_, err = s.LoadPlugin(pc)
				if err != nil {
					msg.SetResponse(map[string]interface{}{"error": err})
				}
				err = s.InitPlugin(pc)
				if err != nil {
					msg.SetResponse(map[string]interface{}{"error": err})
				}
				err = s.StartPlugin(pc.Type)
				if err != nil {
					msg.SetResponse(map[string]interface{}{"error": err})
				}
				msg.SetResponse(map[string]interface{}{"error": nil})
			}
			// default:
			// 	logrus.Errorf("received invalid msg %+v", v)
			// }
		} // end select
	} // end for
}

func (s *Service) UnloadPlugin(pluginType string) error {
	s.logger.Info("Unloading plugin %s", pluginType)
	if _, ok := s.Plugins[pluginType]; !ok {
		return fmt.Errorf("failed to unload plugin %s: %s not found ", pluginType, pluginType)
	}
	pl := s.Plugins[pluginType]
	// send cancel to start
	s.logger.Info("Send cancel message to plugin %s", pluginType)
	s.cancelFuncs[pluginType]()
	// run plugin stop
	s.logger.Info("Stopping plugin %s", pluginType)
	err := pl.Stop(context.Background())
	if err != nil {
		return errors.Wrapf(err, "failed to stop plugin %s", pluginType)
	}
	s.logger.Info("Stopped plugin %s", pluginType)
	// delete pluginMap
	delete(s.Plugins, pluginType)
	// delete loadedpluginMap if in hashicorp mode
	if s.config.PluginMode == PluginModeHC {
		for k := range s.LoadedPlugins {
			if strings.Contains(k, fmt.Sprintf("%s.so", pluginType)) {
				delete(s.LoadedPlugins, k)
			}
		}
	}
	// delete chan
	close(s.Chans[pluginType])
	delete(s.Chans, pluginType)
	// delete cancel
	delete(s.cancelFuncs, pluginType)
	s.logger.Info("Unloaded plugin %s", pluginType)
	return nil
}

func (s *Service) UnloadPlugins() error {
	for ptype := range s.Plugins {
		err := s.UnloadPlugin(ptype)
		if err != nil {
			return errors.Wrapf(err, "failed unload plugin %s", ptype)
		}
	}
	return nil
}

func (s *Service) Stop() error {
	// stop all plugins
	err := s.UnloadPlugins()
	if err != nil {
		return err
	}
	return nil
}
