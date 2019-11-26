package elsvc

import (
	fmt "fmt"
	"os"
	"sync"

	"github.com/hashicorp/go-hclog"
)

const (
	LogInfoLevel  = "info"
	LogDebugLevel = "debug"
)

var mut = &sync.Mutex{}
var logger *Logger

type LoggerIntf interface {
	Error(string, ...interface{}) error
	Info(string, ...interface{})
	Debug(string, ...interface{})
}

func NewModLogger(name string) *Logger {
	mut.Lock()
	defer mut.Unlock()
	logg := &Logger{
		hclogger: logger.hclogger.Named(name),
	}
	return logg
}

func SetupLogger(name string, logLevel string) error {
	mut.Lock()
	defer mut.Unlock()
	err := logger.SetLogLevel(logLevel)
	if err != nil {
		return err
	}
	logger.hclogger = logger.hclogger.ResetNamed(name)
	return nil
}

//SetupLoggerPlugin this func will only be call in StartPlugin
func setupLoggerPlugin() error {
	logOpt := &hclog.LoggerOptions{
		// Name:   name,
		Level:      hclog.Debug,
		Output:     os.Stderr,
		JSONFormat: true,
	}
	mut.Lock()
	defer mut.Unlock()
	logger = &Logger{
		hclogger: hclog.New(logOpt),
	}
	return nil
}

func init() {
	mut.Lock()
	defer mut.Unlock()
	logger = &Logger{
		hclogger: hclog.New(&hclog.LoggerOptions{
			Name:   "elsvc",
			Output: os.Stdout,
			Level:  hclog.Debug,
		}),
	}
}

type Logger struct {
	// Mode     string
	// logLevel string
	hclogger hclog.Logger
	// stdlogger *logrus.Logger
}

func (s *Logger) SetLogLevel(logLevel string) error {
	switch logLevel {
	case LogDebugLevel:
		s.hclogger.SetLevel(hclog.Debug)
	case LogInfoLevel:
		s.hclogger.SetLevel(hclog.Info)
	default:
		return fmt.Errorf("invalid logLevel %s", logLevel)
	}
	return nil
}

func (s Logger) Error(format string, args ...interface{}) error {
	str := fmt.Sprintf(format, args...)
	s.hclogger.Error(str)
	return fmt.Errorf(str)
}

func (s Logger) Info(format string, args ...interface{}) {
	str := fmt.Sprintf(format, args...)
	s.hclogger.Info(str)
}

func (s Logger) Debug(format string, args ...interface{}) {
	str := fmt.Sprintf(format, args...)
	s.hclogger.Debug(str)
}

func Error(format string, args ...interface{}) error {
	mut.Lock()
	defer mut.Unlock()
	return logger.Error(format, args...)
}

func Info(format string, args ...interface{}) {
	mut.Lock()
	defer mut.Unlock()
	logger.Info(format, args...)
}

func Debug(format string, args ...interface{}) {
	mut.Lock()
	defer mut.Unlock()
	logger.Debug(format, args...)
}
