package elsvc

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"

	"github.com/ghodss/yaml"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

const (
	defaultChanLength = 1000
	ChanKeyService    = "common"
)

const (
	MsgConfigReload = "reload_config"
	MsgUnloadPlugin = "unload_plugin"
	MsgLoadPlugin   = "load_plugin"
)

type PluginConfig struct {
	// Name       string                 `json:"name"`
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
	LogLevel string         `json:"log_level"`
	Plugins  []PluginConfig `json:"plugins"`
}

type Service struct {
	LoadedPlugins map[string]*PluginLoader // since plugin couldn't unload, reload plugin will lookup here
	Plugins       map[string]*PluginLoader
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

func (s *Service) LoadConfig() error {
	confPath := os.Getenv("CONFIGPATH")
	if confPath == "" {
		return fmt.Errorf("env CONFIGPATH is empty")
	}
	data, err := ioutil.ReadFile(confPath)
	if err != nil {
		return err
	}
	confObj := &ServiceConfig{}
	err = yaml.Unmarshal(data, confObj)
	if err != nil {
		return err
	}
	s.config = confObj
	return nil
}

func (s *Service) LoadPlugin(pc PluginConfig) (*PluginLoader, error) {
	pluginPath := FindLatestSO(pc.Type, pc.PluginPath())
	if pluginPath == "" {
		return nil, fmt.Errorf("failed to find %s in %s", pc.Type, pc.PluginPath())
	}
	// if plugin not yet loaded
	if _, ok := s.LoadedPlugins[pluginPath]; !ok {
		logrus.Debugf("Loading plugin %s", pluginPath)
		pl := &PluginLoader{}
		err := pl.Load(pluginPath)
		if err != nil {
			return nil, err
		}
		pl.Name = pc.Type
		s.LoadedPlugins[pluginPath] = pl
	}
	pl := s.LoadedPlugins[pluginPath]
	s.Plugins[pc.Type] = pl
	s.Chans[pc.Type] = s.GetChan(pc.Type, pc.ChanLen())
	logrus.Debugf("Loaded plugin %s", pl.Name)
	return pl, nil
}

func (s *Service) InitPlugin(pc PluginConfig) error {
	// init plugin
	ctx := context.WithValue(
		context.Background(), CtxKeyConfig, pc.Config())
	pl := s.Plugins[pc.Type]
	err := pl.Init(ctx)
	if err != nil {
		return err
	}
	return nil
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

func (s *Service) UnloadPlugin(pluginType string) error {
	if _, ok := s.Plugins[pluginType]; !ok {
		return fmt.Errorf("failed to unload plugin %s: %s not found ", pluginType, pluginType)
	}
	pl := s.Plugins[pluginType]
	// send cancel to start
	s.cancelFuncs[pluginType]()
	// run plugin stop
	logrus.Debugf("Stopping plugin %s", pl.Name)
	err := pl.Stop(context.Background())
	if err != nil {
		return errors.Wrapf(err, "failed to stop plugin %s", pluginType)
	}
	// delete chan
	close(s.Chans[pluginType])
	delete(s.Chans, pluginType)
	// delete cancel
	delete(s.cancelFuncs, pluginType)
	return nil
}

func (s *Service) Init() error {
	var err error

	s.LoadedPlugins = make(map[string]*PluginLoader)
	s.Plugins = make(map[string]*PluginLoader)
	s.cancelFuncs = make(map[string]context.CancelFunc)
	// init default chan
	s.Chans = make(map[string]chan interface{})
	s.GetChan(ChanKeyService, defaultChanLength)

	// load config
	err = s.LoadConfig()
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
	pl := s.Plugins[pluginName]
	ctx, cancel := context.WithCancel(context.WithValue(
		context.Background(), CtxKeyChans, s.Chans))
	s.cancelFuncs[pluginName] = cancel
	err := pl.Start(ctx)
	if err != nil {
		return err
	}
	return nil
}

func (s *Service) StartPlugins() error {
	for ptype := range s.Plugins {
		err := s.StartPlugin(ptype)
		if err != nil {
			return errors.Wrapf(err, "failed to start plugin %s", ptype)
		}
	}
	return nil
}

func (s *Service) Start() error {
	// run plugins
	err := s.StartPlugins()
	if err != nil {
		return err
	}
	for {
		select {
		case v := <-s.Chans[ChanKeyService]:
			switch v.(type) {
			// only support MsgBase
			case MsgBase:
				msg := v.(MsgBase)
				switch msg.Type() {
				case MsgTypeStop:
					logrus.Infof("Received stop msg, stopping service")
					err := s.Stop()
					msg.SetResponse(err)
					WaitGoroutines()
					return nil
				case MsgUnloadPlugin:
					pluginName := msg.GetRequest().(string)
					err := s.UnloadPlugin(pluginName)
					msg.SetResponse(err)
				case MsgLoadPlugin:
					pc := msg.GetRequest().(PluginConfig)
					_, err := s.LoadPlugin(pc)
					if err != nil {
						msg.SetResponse(err)
					}
					err = s.InitPlugin(pc)
					if err != nil {
						msg.SetResponse(err)
					}
					err = s.StartPlugin(pc.Type)
					if err != nil {
						msg.SetResponse(err)
					}
					msg.SetResponse(nil)
				}
			default:
				logrus.Errorf("received invalid msg %+v", v)
			}
		}
	}
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
