// Package objects registers owned-object storage and independent authorization.
package objects

import (
	"fmt"
	"github.com/lengzhao/pluginkit"
	"github.com/lengzhao/vm/core"
	impl "github.com/lengzhao/vm/internal/objects"
)

func NewOwner(struct{}) (core.ObjectAuthorizer, error) { return impl.OwnerPolicy{}, nil }

type Deps struct {
	Authorizer core.ObjectAuthorizer `json:"authorizer"`
}

func New(_ struct{}, d Deps) (core.ObjectService, error) {
	if d.Authorizer == nil {
		return nil, fmt.Errorf("object authorizer required")
	}
	return impl.New(d.Authorizer), nil
}
func init() {
	pluginkit.Register("objects/owner", NewOwner)
	pluginkit.Register("objects/transactional", New)
}
