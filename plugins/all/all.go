// Package all registers all built-in VM plugins.
package all

import (
	_ "github.com/lengzhao/vm/plugins/abi"
	_ "github.com/lengzhao/vm/plugins/batch"
	_ "github.com/lengzhao/vm/plugins/compiler"
	_ "github.com/lengzhao/vm/plugins/deployment"
	_ "github.com/lengzhao/vm/plugins/engine"
	_ "github.com/lengzhao/vm/plugins/gas"
	_ "github.com/lengzhao/vm/plugins/objects"
	_ "github.com/lengzhao/vm/plugins/policy"
	_ "github.com/lengzhao/vm/plugins/runner"
	_ "github.com/lengzhao/vm/plugins/security"
	_ "github.com/lengzhao/vm/plugins/state"
	_ "github.com/lengzhao/vm/plugins/storage"
)
