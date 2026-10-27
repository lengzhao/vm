package execution

import (
	"context"
	"errors"
	"fmt"
	"io"
	"testing"

	"github.com/lengzhao/vm/core"
	"github.com/lengzhao/vm/internal/objects"
	"github.com/lengzhao/vm/internal/state"
)

func TestOwnedHostBudgetsAndFailureClassification(t *testing.T) {
	for _, tc := range []struct {
		name, op, id, extra string
		gas                 uint64
		code                core.ErrorCode
	}{
		{"undercharge", "create", "x", "", 29, core.CodeInvalidResponse},
		{"no-id", "create", "", "", 30, core.CodeInvalidResponse},
		{"no-service", "create", "x", "", 30, core.CodeStateFailure},
		{"owner-spoof", "create", "x", `,"owner":"mallory"`, 30, core.CodeObjectFailure},
		{"denied", "set", "x", "", 20, core.CodeObjectFailure},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			tx, _ := state.NewMVCC().Begin(ctx)
			defer tx.Rollback()
			svc := objects.New(objects.OwnerPolicy{})
			s := svc.Bind(tx, core.ObjectScope{Contract: "a", Sender: "alice"})
			s.Apply(ctx, core.ObjectCommand{ID: "x", Op: "create"})
			if tc.name != "no-service" {
				ctx = core.WithObjects(ctx, svc.Bind(tx, core.ObjectScope{Contract: "a", Sender: "bob"}))
			}
			o := &objectOutput{boundedOutput: boundedOutput{limit: 1 << 20, cancel: func() {}}, ctx: ctx, reply: io.Discard, gasFloor: 10}
			line := fmt.Sprintf(`{"version":3,"type":"object","op":%q,"id":%q,"gas":%d%s}`+"\n", tc.op, tc.id, tc.gas, tc.extra)
			if e := o.handle([]byte(line)); e == nil {
				t.Fatal("request accepted")
			}
			var e *core.ExecutionError
			if !errors.As(o.err, &e) || e.Code != tc.code {
				t.Fatal(o.err)
			}
		})
	}
}
