// Package storage registers artifact persistence without deployment semantics.
package storage

import (
	"github.com/lengzhao/pluginkit"
	"github.com/lengzhao/vm/core"
	impl "github.com/lengzhao/vm/internal/storage"
)

type Config struct {
	Directory string `json:"directory"`
}

func New(c Config) (core.ContractStore, error) { return impl.New(c.Directory) }
func init()                                    { pluginkit.Register("storage/filesystem", New) }
