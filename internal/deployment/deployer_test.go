package deployment

import (
	"context"
	"errors"
	"github.com/lengzhao/vm/core"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type writer struct {
	address  string
	artifact *core.CompiledContract
}

func (w *writer) Create(ctx context.Context, a string, c *core.CompiledContract) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	w.address = a
	w.artifact = c
	return nil
}

type validator struct{ err error }

func (v validator) ValidateArtifact(ctx context.Context, c *core.CompiledContract) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return v.err
}
func TestDeploymentOnlyNeedsCreator(t *testing.T) {
	p := filepath.Join(t.TempDir(), "artifact")
	if err := os.WriteFile(p, []byte("x"), 0755); err != nil {
		t.Fatal(err)
	}
	w := &writer{}
	d, err := New(w, validator{})
	if err != nil {
		t.Fatal(err)
	}
	c := &core.CompiledContract{ExecutablePath: p, SourceHash: "hash", ManifestSHA256: strings.Repeat("a", 64)}
	address, err := d.Deploy(context.Background(), core.DeploymentRequest{Contract: c, Input: core.DeploymentInput{ChainID: "chain", Sender: "alice"}})
	if err != nil {
		t.Fatal(err)
	}
	if address == "" || w.address != address || w.artifact == nil {
		t.Fatal("artifact not persisted")
	}
	if c.Address != "" || c.ExecutablePath != p {
		t.Fatal("deployment mutated compiler output")
	}
}
func TestRejectBeforeWriting(t *testing.T) {
	denied := errors.New("denied")
	w := &writer{}
	d, err := New(w, validator{err: denied})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.Deploy(context.Background(), core.DeploymentRequest{Contract: &core.CompiledContract{}, Input: core.DeploymentInput{ChainID: "chain", Sender: "alice"}}); !errors.Is(err, denied) {
		t.Fatal(err)
	}
	if w.artifact != nil {
		t.Fatal("rejected artifact was persisted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := d.Deploy(ctx, core.DeploymentRequest{Contract: &core.CompiledContract{}}); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}
