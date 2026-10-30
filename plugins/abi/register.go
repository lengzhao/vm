// Package abi registers callable ABI extraction.
package abi

import (
	"github.com/lengzhao/pluginkit"
	"github.com/lengzhao/vm/core"
	impl "github.com/lengzhao/vm/internal/abigen"
)

type Config struct{}

func New(Config) (core.ABIGenerator, error) { return impl.New(), nil }
func init()                                 { pluginkit.Register("abi/go", New) }
