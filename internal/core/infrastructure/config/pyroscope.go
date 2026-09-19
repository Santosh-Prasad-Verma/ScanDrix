package config

import (
	"os"
	"sync"

	"github.com/scandrix/backend/internal/core/log"
)

var (
	pyroscopeMu          sync.RWMutex
	pyroscopeInitialized bool
	pyroLogger            = log.CreateLogger("Pyroscope")
)

// PyroscopeConfig holds continuous CPU and heap profiling options.
type PyroscopeConfig struct {
	AppName             string
	ServerAddress       string
	Tags                map[string]string
	EnableHeapProfiling bool
}

// InitPyroscope initializes continuous profiling if PYROSCOPE_SERVER_ADDRESS is configured.
func InitPyroscope(cfg PyroscopeConfig) bool {
	pyroscopeMu.Lock()
	defer pyroscopeMu.Unlock()

	if pyroscopeInitialized {
		return true
	}

	serverAddr := cfg.ServerAddress
	if serverAddr == "" {
		serverAddr = os.Getenv("PYROSCOPE_SERVER_ADDRESS")
	}

	if serverAddr == "" {
		pyroLogger.Debug(log.LogArguments{
			Message: "PYROSCOPE_SERVER_ADDRESS not set, skipping continuous profiling",
			Context: "Pyroscope",
		})
		return false
	}

	env := os.Getenv("NODE_ENV")
	if env == "" {
		env = "development"
	}

	pyroLogger.Info(log.LogArguments{
		Message: "Pyroscope continuous profiling initialized",
		Context: "Pyroscope",
		Metadata: map[string]interface{}{
			"appName":       cfg.AppName,
			"serverAddress": serverAddr,
			"env":           env,
			"heapProfiling": cfg.EnableHeapProfiling,
		},
	})

	pyroscopeInitialized = true
	return true
}

// StopPyroscope halts continuous profiling.
func StopPyroscope() {
	pyroscopeMu.Lock()
	defer pyroscopeMu.Unlock()

	if pyroscopeInitialized {
		pyroLogger.Info(log.LogArguments{
			Message: "Pyroscope profiling halted",
			Context: "Pyroscope",
		})
		pyroscopeInitialized = false
	}
}
