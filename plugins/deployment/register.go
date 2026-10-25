// Package deployment registers contract deployment independently of storage.
package deployment

import (
	"github.com/lengzhao/pluginkit"
	"github.com/lengzhao/vm/core"
	impl "github.com/lengzhao/vm/internal/deployment"
)

type Config struct{}
type Deps struct {
	Store     core.ContractCreator   `json:"store"`
	Validator core.ArtifactValidator `json:"validator"`
}

func New(_ Config, d Deps) (core.ContractDeployer, error) { return impl.New(d.Store, d.Validator) }
func init()                                               { pluginkit.Register("deployment/default", New) }
