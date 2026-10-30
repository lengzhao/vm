// Package runner registers the process execution backend.
package runner

import (
	"fmt"
	"github.com/lengzhao/pluginkit"
	"github.com/lengzhao/vm/core"
	"github.com/lengzhao/vm/internal/process"
	"github.com/lengzhao/vm/internal/sandbox"
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

// DockerConfig selects an isolated backend, with no fallback to host execution.
type DockerConfig struct {
	Timeout string         `json:"timeout"`
	Sandbox sandbox.Config `json:"sandbox"`
}

func NewDocker(c DockerConfig) (core.Runner, error) {
	d, err := sandbox.New(c.Sandbox)
	if err != nil {
		return nil, err
	}
	timeout := 30 * time.Second
	if c.Timeout != "" {
		timeout, err = time.ParseDuration(c.Timeout)
		if err != nil {
			return nil, err
		}
		if timeout <= 0 {
			return nil, fmt.Errorf("timeout must be positive")
		}
	}
	return process.NewDocker(timeout, d), nil
}
func init() { pluginkit.Register("runner/docker", NewDocker) }
