package elsvc

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"plugin"
	"strconv"
	"strings"

	"github.com/pkg/errors"
)

const (
	FuncInit  = "Init"
	FuncStart = "Start"
	FuncStop  = "Stop"
)

type PluginLoaderIntf interface {
	Name() string
	Load(PluginConfig) error
	Init(ctx context.Context) error
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
}

type pluginLoader struct {
	goplugin   *plugin.Plugin
	elplugin   PluginIntf
	pluginPath string
	logger     *Logger
}

// func (s PluginLoader) PluginPath() string {
// 	return s.pluginPath
// }

func (s pluginLoader) Name() string {
	return s.elplugin.ModuleName()
}

func (s *pluginLoader) Load(pc PluginConfig) error {
	s.logger = NewModLogger("pluginLoader")

	pluginPath := findLatestSO(pc.Type, pc.PluginPath())
	if pluginPath == "" {
		return fmt.Errorf("failed to find plugin for %s", pc.Type)
	}
	p, err := plugin.Open(pluginPath)
	if err != nil {
		return err
	}
	symbol, err := p.Lookup("PluginObj")
	if err != nil {
		return errors.Wrapf(err, "failed to find PluginObj in %s", pluginPath)
	}
	elp, ok := symbol.(PluginIntf)
	if !ok {
		return fmt.Errorf("failed to convert PluginObj in %s", pluginPath)
	}
	// Set env
	for k, v := range pc.EnvMap {
		os.Setenv(k, v)
	}
	s.elplugin = elp
	s.goplugin = p
	s.pluginPath = pluginPath
	return nil
}

func (s *pluginLoader) Init(ctx context.Context) error {
	return s.elplugin.Init(ctx)
}

func (s *pluginLoader) Start(ctx context.Context) error {
	go func() {
		err := s.elplugin.Start(ctx)
		if err != nil {
			s.logger.Error("plugin %s error from start: %s", s.pluginPath, err.Error())
		}
		s.logger.Info("plugin %s return from Start()", s.pluginPath)
		// return start() error to Service
		msg := NewMsg(ChanKeyService, MsgStartError)
		msg.SetResponse(map[string]interface{}{
			"plugin": s.Name(),
			"error":  err,
		})
		SendMsg(ctx, msg)
	}()
	return nil
}

func (s *pluginLoader) Stop(ctx context.Context) error {
	return s.elplugin.Stop(ctx)
}

//FindLatestSO find the latest so under pluginPath
// so file must format as: <plugin_name>.so.<int version>
// returns: fullPluginPath
func findLatestSO(pluginName string, pluginPath string) string {
	// if plugin_path is a so file e.g. "./<plugin>.so"
	if isFile(pluginPath) {
		return pluginPath
	}
	// if plugin_path is a so dir e.g. "./"
	files, err := ioutil.ReadDir(pluginPath)
	if err != nil {
		return ""
	}
	filePrefix := fmt.Sprintf("%s.so", pluginName)
	pluginFile := ""
	version := -1
	for _, f := range files {
		name := f.Name()
		if !strings.HasPrefix(name, filePrefix) {
			continue
		}
		splitN := strings.Split(name, ".")
		switch len(splitN) {
		case 2:
			// <plugin>.so
			if pluginFile == "" {
				pluginFile = name
			}
		case 3:
			// <plugin>.so.<version>
			versionStr := splitN[2]
			ver, err := strconv.Atoi(versionStr)
			if err != nil {
				// invalid version
				continue
			}
			if ver > version {
				version = ver
				pluginFile = name
			}
		}
	}
	if pluginFile == "" {
		return ""
	}
	return fmt.Sprintf("%s/%s", strings.TrimRight(pluginPath, "/"), pluginFile)
}
