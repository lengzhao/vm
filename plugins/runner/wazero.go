package runner

import (
	"fmt"
	"github.com/lengzhao/pluginkit"
	"github.com/lengzhao/vm/core"
	"github.com/lengzhao/vm/internal/wasm"
	"time"
)

type WazeroConfig struct {
	Timeout     string `json:"timeout"`
	MemoryMB    int    `json:"memoryMB"`
	MaxModuleMB int    `json:"maxModuleMB"`
	Engine      string `json:"engine"`
}

func NewWazero(c WazeroConfig) (core.Runner, error) {
	var timeout time.Duration
	if c.Timeout != "" {
		var err error
		timeout, err = time.ParseDuration(c.Timeout)
		if err != nil {
			return nil, err
		}
		if timeout <= 0 {
			return nil, fmt.Errorf("timeout must be positive")
		}
	}
	return wasm.New(wasm.Options{Timeout: timeout, MemoryMB: c.MemoryMB, MaxModuleMB: c.MaxModuleMB, Engine: c.Engine})
}
func init() { pluginkit.Register("runner/wazero", NewWazero) }
