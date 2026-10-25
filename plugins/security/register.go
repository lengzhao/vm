// Package security registers the AST security reviewer.
package security

import (
	"github.com/lengzhao/pluginkit"
	"github.com/lengzhao/vm/core"
	impl "github.com/lengzhao/vm/internal/security"
)

type Config struct{}

func New(Config) (core.SecurityReviewer, error) { return impl.New(), nil }
func init()                                     { pluginkit.Register("security/go", New) }
