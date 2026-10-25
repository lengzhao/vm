package abigen

import (
	"github.com/lengzhao/vm/abi"
	"github.com/lengzhao/vm/core"
)

type Generator struct{}

func New() *Generator { return &Generator{} }
func (*Generator) Generate(source string) (*abi.ABI, error) {
	a, err := abi.ExtractCallableABI([]byte(source))
	if err != nil {
		return nil, &core.ExecutionError{Code: core.CodeUnsupportedABI, Message: err.Error(), Cause: err}
	}
	return a, nil
}
