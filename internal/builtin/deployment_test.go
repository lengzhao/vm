package vm

import (
	"context"
	"encoding/json"
	"errors"
	compatvm "github.com/lengzhao/vm"
	"github.com/lengzhao/vm/core"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLocalDeploymentIsDeterministicAndIdempotent(t *testing.T) {
	e := NewVMEngine(VMConfig{ContractStorageDir: t.TempDir()})
	c, err := e.Compile("package main;func F()int{return 7}")
	if err != nil {
		t.Fatal(err)
	}
	first, err := e.Deploy(c)
	if err != nil {
		t.Fatal(err)
	}
	second, err := e.Deploy(c)
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatalf("same deployment changed address: %s != %s", first, second)
	}
}

func TestExplicitDeploymentPredictsAddressAcrossStores(t *testing.T) {
	source := compileTestContract(t, t.TempDir(), "package main;func F()int{return 7}")
	originalPath := source.ExecutablePath
	input := core.DeploymentInput{ChainID: "chain-A", Sender: "alice", Nonce: 42, Salt: [32]byte{7}}
	predicted, err := compatvm.DeriveDeploymentAddress(input, source.ManifestSHA256)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		engine := NewVMEngine(VMConfig{ContractStorageDir: t.TempDir()})
		address, err := engine.Engine.Deploy(context.Background(), core.DeploymentRequest{Contract: source, Input: input})
		if err != nil {
			t.Fatal(err)
		}
		if address != predicted {
			t.Fatalf("node address differs: %s", address)
		}
		if source.Address != "" || source.Deployment != nil || source.ExecutablePath != originalPath {
			t.Fatal("native deployment mutated producer artifact")
		}
		loaded, err := engine.GetContract(address)
		if err != nil {
			t.Fatal(err)
		}
		if loaded.Deployment == nil || loaded.Deployment.Input != input || loaded.Deployment.ArtifactSHA256 != source.ManifestSHA256 {
			t.Fatal("deployment input lost")
		}
		result, err := engine.Execute(address, "F")
		if err != nil || string(result) != "7" {
			t.Fatalf("deployed artifact did not run: %s %v", result, err)
		}
	}
}
func TestDeploymentRecordTamperingRejected(t *testing.T) {
	for _, target := range []string{"nonce", "sender", "artifact", "missing", "version"} {
		t.Run(target, func(t *testing.T) {
			dir := t.TempDir()
			engine := NewVMEngine(VMConfig{ContractStorageDir: dir})
			c, err := engine.Compile("package main;func F()int{return 7}")
			if err != nil {
				t.Fatal(err)
			}
			address, err := engine.DeployWithInput(c, core.DeploymentInput{ChainID: "chain", Sender: "alice"})
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(dir, address, "metadata.json")
			bytes, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			var metadata core.ContractMetadata
			if err := json.Unmarshal(bytes, &metadata); err != nil {
				t.Fatal(err)
			}
			switch target {
			case "nonce":
				metadata.Deployment.Input.Nonce++
			case "sender":
				metadata.Deployment.Input.Sender = "mallory"
			case "artifact":
				metadata.Deployment.ArtifactSHA256 = strings.Repeat("a", 64)
			case "missing":
				metadata.Deployment = nil
			case "version":
				metadata.Deployment.Version = 99
			}
			bytes, err = json.Marshal(metadata)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, bytes, 0644); err != nil {
				t.Fatal(err)
			}
			if _, err := engine.GetContract(address); err == nil {
				t.Fatal("tampered deployment loaded")
			}
			if _, err := engine.Execute(address, "F"); err == nil {
				t.Fatal("tampered deployment executed")
			}
		})
	}
}
func TestCanceledExplicitDeploymentHasNoRecord(t *testing.T) {
	dir := t.TempDir()
	engine := NewVMEngine(VMConfig{ContractStorageDir: dir})
	c := compileTestContract(t, t.TempDir(), "package main;func F()int{return 7}")
	input := core.DeploymentInput{ChainID: "chain", Sender: "alice"}
	expected, err := compatvm.DeriveDeploymentAddress(input, c.ManifestSHA256)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := engine.DeployContext(ctx, c, input); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, expected)); !os.IsNotExist(err) {
		t.Fatal("canceled deployment created a record")
	}
}
