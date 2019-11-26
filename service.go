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
	"github.com/sirupsen/logrus"
)

const (
	defaultChanLength = 1000
	ChanKeyService    = "common"
	minGoroutineNum   = 3
)

const (
	PluginModeGO = "goplugin"
	PluginModeHC = "hcplugin"
)

const (
	MsgConfigReload = "reload_config"
	MsgUnloadPlugin = "unload_plugin"
	MsgLoadPlugin   = "load_plugin"
)

type PluginConfig struct {
	Type       string                 `json:"type"`
	PluginDir  string                 `json:"plugin_path"`
	ChanLength int                    `json:"chan_len"`
	ConfMap    map[string]interface{} `json:"config"`
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

func (s PluginConfig) ChanLen() int {
	// TODO should I make it default to 1?
	return s.ChanLength
}

type ServiceConfig struct {
	LogLevel   string         `json:"log_level"`
	PluginMode string         `json:"plugin_mode"`
	Plugins    []PluginConfig `json:"plugins"`
}

type Service struct {
	LoadedPlugins map[string]PluginLoaderIntf // since plugin couldn't unload, reload plugin will lookup here
	Plugins       map[string]PluginLoaderIntf
	Chans         map[string]chan interface{}
	cancelFuncs   map[string]context.CancelFunc
	config        *ServiceConfig
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
	if confObj.PluginMode == "" {
		confObj.PluginMode = "goplugin"
	}
	if confObj.PluginMode != PluginModeGO && confObj.PluginMode != PluginModeHC {
		return fmt.Errorf("Plugin mode %s is neither %s nor %s", confObj.PluginMode, PluginModeGO, PluginModeHC)
	}
	s.config = confObj
	return nil
}

func (s *Service) InitPlugin(pc PluginConfig) error {
	// init plugin
	logrus.Infof("Initing plugin %s", pc.Type)
	ctx := context.WithValue(
		context.Background(), CtxKeyConfig, pc.Config())
	pl := s.Plugins[pc.Type]
	err := pl.Init(ctx)
	if err != nil {
		return err
	}
	logrus.Infof("Inited plugin %s", pc.Type)
	return nil
}

func (s *Service) LoadPlugin(pc PluginConfig) (PluginLoaderIntf, error) {
	pluginPath := FindLatestSO(pc.Type, pc.PluginPath())
	if pluginPath == "" {
		return nil, fmt.Errorf("failed to find plugin %s in %s", pc.Type, pc.PluginPath())
	}
	logrus.Infof("Loading plugin %s", pluginPath)
	var pl PluginLoaderIntf
	switch s.config.PluginMode {
	case PluginModeGO:
		// if plugin loaded
		if _, ok := s.LoadedPlugins[pluginPath]; ok {
			pl = s.LoadedPlugins[pluginPath]
		} else {
			// plugin not loaded yet
			pl = &PluginLoader{}
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
		pl = &PluginRunner{}
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
	s.Chans[pl.Name()] = s.GetChan(pl.Name(), pc.ChanLen())
	logrus.Infof("Loaded plugin %s", pl.Name())
	return pl, nil
}

func (s *Service) LoadPlugins() error {
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

	// load plugins
	err = s.LoadPlugins()
	if err != nil {
		return err
	}
	return nil
}

func (s *Service) StartPlugin(pluginName string) error {
	logrus.Infof("Starting plugin %s", pluginName)
	pl := s.Plugins[pluginName]
	ctx, cancel := context.WithCancel(context.WithValue(
		context.Background(), CtxKeyChans, s.Chans))
	s.cancelFuncs[pluginName] = cancel
	err := pl.Start(ctx)
	if err != nil {
		return err
	}
	logrus.Infof("Started plugin %s", pluginName)
	return nil
}

func (s *Service) StartPlugins() error {
	logrus.Infof("Starting Plugins")
	for ptype := range s.Plugins {
		err := s.StartPlugin(ptype)
		if err != nil {
			return errors.Wrapf(err, "failed to start plugin %s", ptype)
		}

	}
	return nil
}

func (s *Service) signalHandler() {
	c := make(chan os.Signal)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		for sig := range c {
			logrus.Debugf("Receive signal %+v", sig)
			if sig.String() != os.Interrupt.String() {
				continue
			}
			s.UnloadPlugins()
			os.Exit(1)
		}
	}()
}

func (s *Service) Start() error {
	// run plugins
	go s.signalHandler()
	err := s.StartPlugins()
	if err != nil {
		return err
	}
	for {
		select {
		case v := <-s.Chans[ChanKeyService]:
			// switch v.(type) {
			// only support MsgBase
			msg, ok := v.(MsgBase)
			if !ok {
				logrus.Errorf("received invalid msg %+v", v)
				continue
			}
			// case MsgBase:
			// msg := v.(MsgBase)

			// message sent to service for routing
			if msg.To() != ChanKeyService {
				// route message to corresponding chan
				if _, ok := s.Chans[msg.To()]; ok {
					s.Chans[msg.To()] <- msg
					continue
				}
				// channel not ready yet for this message
				// put msg back to queue
				msg.DeTTL()
				if msg.Expired() {
					// reach ttl EOL
					// drop msg
					logrus.Infof("drop eol message %v", msg)
					continue
				}
				s.Chans[ChanKeyService] <- msg
			}

			// msg.To() == ChanKeyService
			// message for service
			switch msg.Type() {
			case MsgTypeStop:
				logrus.Infof("Received stop msg, stopping service")
				err := s.Stop()
				msg.SetResponse(map[string]interface{}{"error": err})
				WaitGoroutines(minGoroutineNum)
				return nil
			case MsgUnloadPlugin:
				pluginName := msg.GetRequest()["name"].(string)
				err := s.UnloadPlugin(pluginName)
				msg.SetResponse(map[string]interface{}{"error": err})
			case MsgLoadPlugin:
				pc := PluginConfig{}
				data, _ := json.Marshal(msg.GetRequest())
				logrus.Debugf("#elynn req %+v", msg)
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
	logrus.Infof("Unloading plugin %s", pluginType)
	if _, ok := s.Plugins[pluginType]; !ok {
		return fmt.Errorf("failed to unload plugin %s: %s not found ", pluginType, pluginType)
	}
	pl := s.Plugins[pluginType]
	// send cancel to start
	logrus.Infof("Send cancel message to plugin %s", pluginType)
	s.cancelFuncs[pluginType]()
	// run plugin stop
	logrus.Infof("Stopping plugin %s", pluginType)
	err := pl.Stop(context.Background())
	if err != nil {
		return errors.Wrapf(err, "failed to stop plugin %s", pluginType)
	}
	logrus.Infof("Stopped plugin %s", pluginType)
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
	logrus.Infof("Unloaded plugin %s", pluginType)
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
