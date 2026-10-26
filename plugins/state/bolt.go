package state

import (
	"fmt"
	"time"

	"github.com/lengzhao/pluginkit"
	impl "github.com/lengzhao/vm/internal/state"
)

type BoltConfig struct {
	MaxBatchReceiptMB int    `json:"maxBatchReceiptMB"`
	Path              string `json:"path"`
	LockTimeout       string `json:"lockTimeout"`
}

func NewBolt(c BoltConfig) (*impl.Bolt, error) {
	var timeout time.Duration
	if c.LockTimeout != "" {
		var err error
		timeout, err = time.ParseDuration(c.LockTimeout)
		if err != nil {
			return nil, err
		}
		if timeout <= 0 {
			return nil, fmt.Errorf("lockTimeout must be positive")
		}
	}
	return impl.NewBolt(impl.BoltOptions{Path: c.Path, LockTimeout: timeout, MaxBatchReceiptMB: c.MaxBatchReceiptMB})
}
func init() { pluginkit.Register("state/bbolt", NewBolt) }
