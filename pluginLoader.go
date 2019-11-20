package elsvc

import (
	"context"
	"fmt"
	"io/ioutil"
	"plugin"
	"strconv"
	"strings"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

const (
	FuncInit  = "Init"
	FuncStart = "Start"
	FuncStop  = "Stop"
)

type PluginLoader struct {
	Name     string
	goplugin *plugin.Plugin
	elplugin PluginIntf
	fullPath string
}

func (s PluginLoader) PluginPath() string {
	return s.fullPath
}

func (s *PluginLoader) Load(pluginPath string) error {
	p, err := plugin.Open(pluginPath)
	if err != nil {
		return err
	}
	// s.funcs = make(map[string]plugin.Symbol)
	// // lookup init
	// initFunc, err := p.Lookup(FuncInit)
	// if err != nil {
	// 	return err
	// }
	// s.funcs[FuncInit] = initFunc
	// // lookup start
	// startFunc, err := p.Lookup(FuncStart)
	// if err != nil {
	// 	return err
	// }
	// s.funcs[FuncStart] = startFunc
	// // lookup stop
	// stopFunc, err := p.Lookup(FuncStop)
	// if err != nil {
	// 	return err
	// }
	// s.funcs[FuncStop] = stopFunc
	symbol, err := p.Lookup("PluginObj")
	if err != nil {
		return errors.Wrapf(err, "failed to find PluginObj in %s", pluginPath)
	}
	elp, ok := symbol.(PluginIntf)
	if !ok {
		return fmt.Errorf("failed to convert PluginObj in %s", pluginPath)
	}
	s.elplugin = elp
	s.goplugin = p
	s.fullPath = pluginPath
	return nil
}

func (s *PluginLoader) Init(ctx context.Context) error {
	// initFunc := s.funcs[FuncInit].(func(context.Context) error)
	// return initFunc(ctx)
	return s.elplugin.Init(ctx)
}

func (s *PluginLoader) Start(ctx context.Context) error {
	// startFunc := s.funcs[FuncStart].(func(context.Context) error)
	// return startFunc(ctx)
	go func() {
		err := s.elplugin.Start(ctx)
		if err != nil {
			logrus.Errorf("plugin %s error from start: %s", s.PluginPath(), err.Error())
		}
		logrus.Infof("plugin %s return from Start()", s.PluginPath())
	}()
	return nil
}

func (s *PluginLoader) Stop(ctx context.Context) error {
	// stopFunc := s.funcs[FuncStop].(func(context.Context) error)
	// return stopFunc(ctx)
	return s.elplugin.Stop(ctx)
}

//FindLatestSO find the latest so under pluginPath
// so file must format as: <plugin_name>.so.<int version>
// returns: fullPluginPath
func FindLatestSO(pluginName string, pluginPath string) string {
	// if plugin_path is a so file e.g. "./<plugin>.so"
	if IsFile(pluginPath) {
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
