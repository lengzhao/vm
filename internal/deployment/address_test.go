package deployment

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/lengzhao/vm/core"
)

func TestAddressV1FixedVector(t *testing.T) {
	input := core.DeploymentInput{ChainID: "chain-A", Sender: "alice", Nonce: 42}
	for i := range input.Salt {
		input.Salt[i] = byte(i)
	}
	got, err := DeriveAddress(input, strings.Repeat("ab", 32))
	if err != nil {
		t.Fatal(err)
	}
	// Independently computed with Python hashlib/struct (big-endian lengths/nonce).
	const want = "contract_fcef3323949e2c2397b5761bc412e1282e402308fcb0f75b2cb89d38e832aee8"
	if got != want {
		t.Fatalf("address protocol changed: %s", got)
	}
}
func TestAddressBindsEveryInputAndSeparatesFields(t *testing.T) {
	original := core.DeploymentInput{ChainID: "chain", Sender: "alice"}
	digest := strings.Repeat("ab", 32)
	expected, err := DeriveAddress(original, digest)
	if err != nil {
		t.Fatal(err)
	}
	changes := []func(*core.DeploymentInput){
		func(v *core.DeploymentInput) { v.ChainID = "other" }, func(v *core.DeploymentInput) { v.Sender = "bob" },
		func(v *core.DeploymentInput) { v.Nonce = 1 }, func(v *core.DeploymentInput) { v.Salt[0] = 1 },
	}
	for _, change := range changes {
		input := original
		change(&input)
		got, err := DeriveAddress(input, digest)
		if err != nil {
			t.Fatal(err)
		}
		if got == expected {
			t.Fatal("deployment input not bound")
		}
	}
	changed, err := DeriveAddress(original, strings.Repeat("cd", 32))
	if err != nil {
		t.Fatal(err)
	}
	if changed == expected {
		t.Fatal("artifact not bound")
	}
	a, _ := DeriveAddress(core.DeploymentInput{ChainID: "ab", Sender: "c"}, digest)
	b, _ := DeriveAddress(core.DeploymentInput{ChainID: "a", Sender: "bc"}, digest)
	if a == b {
		t.Fatal("ambiguous field encoding")
	}
}
func TestInvalidInputsNeverCreateRecords(t *testing.T) {
	creator := &writer{}
	deployer, err := New(creator, validator{})
	if err != nil {
		t.Fatal(err)
	}
	for _, input := range []core.DeploymentInput{{}, {ChainID: "chain"}, {Sender: "alice"}, {ChainID: strings.Repeat("x", 257), Sender: "alice"}, {ChainID: "chain", Sender: core.Address(string([]byte{255}))}} {
		_, err := deployer.Deploy(context.Background(), core.DeploymentRequest{Contract: &core.CompiledContract{ManifestSHA256: strings.Repeat("a", 64)}, Input: input})
		var ee *core.ExecutionError
		if !errors.As(err, &ee) || ee.Code != core.CodeInvalidRequest {
			t.Fatalf("invalid input accepted: %v", err)
		}
	}
	if creator.artifact != nil {
		t.Fatal("invalid request wrote a record")
	}
	for _, digest := range []string{"", strings.Repeat("a", 16), strings.Repeat("AB", 32), strings.Repeat("z", 64)} {
		if _, err := DeriveAddress(core.DeploymentInput{ChainID: "chain", Sender: "alice"}, digest); err == nil {
			t.Fatal("invalid manifest digest accepted")
		}
	}
}
