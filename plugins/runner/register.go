// Package runner registers the process execution backend.
package runner

import (
	"fmt"
	"github.com/lengzhao/pluginkit"
	"github.com/lengzhao/vm/core"
	"github.com/lengzhao/vm/internal/process"
	"time"
)

type Config struct {
	Timeout string `json:"timeout"`
}

func New(c Config) (core.Runner, error) {
	var timeout time.Duration
	var err error
	if c.Timeout != "" {
		timeout, err = time.ParseDuration(c.Timeout)
		if err != nil {
			return nil, err
		}
		if timeout <= 0 {
			return nil, fmt.Errorf("timeout must be positive")
		}
	}
	return process.New(timeout), nil
}
func init() { pluginkit.Register("runner/process", New) }
