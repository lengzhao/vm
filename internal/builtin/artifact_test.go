package vm

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"github.com/lengzhao/vm/core"
	"github.com/lengzhao/vm/internal/artifact"
	"github.com/lengzhao/vm/internal/storage"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCompileRejectsTamperedCache(t *testing.T) {
	c := NewContractCompilerWithOptions(t.TempDir(), true)
	source := "package main;func F()int{return 7}"
	compiled, err := c.Compile(source)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(compiled.ExecutablePath, []byte("tampered"), 0755); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Compile(source); err == nil {
		t.Fatal("tampered cache accepted")
	}
}

func TestArtifactTamperingRejected(t *testing.T) {
	for _, target := range []string{"binary", "abi", "profile", "metadata", "manifest", "missing manifest", "invalid profile", "missing profile", "renamed binary"} {
		t.Run(target, func(t *testing.T) {
			dir := t.TempDir()
			e := NewVMEngine(VMConfig{ContractStorageDir: dir})
			c, err := e.Compile("package main;func F()int{return 7}")
			if err != nil {
				t.Fatal(err)
			}
			address, err := e.Deploy(c)
			if err != nil {
				t.Fatal(err)
			}
			record := filepath.Join(dir, address)
			switch target {
			case "binary":
				err = os.WriteFile(c.ExecutablePath, []byte("tampered"), 0755)
			case "abi":
				err = os.WriteFile(filepath.Join(record, "abi.json"), []byte(`{"version":1,"package_name":"changed","functions":[]}`), 0644)
			case "profile":
				err = os.WriteFile(filepath.Join(record, "gas_profile.json"), []byte(`{"version":1,"entry_gas":999}`), 0644)
			case "metadata":
				data, readErr := os.ReadFile(filepath.Join(record, "metadata.json"))
				if readErr != nil {
					t.Fatal(readErr)
				}
				var m map[string]any
				if err := json.Unmarshal(data, &m); err != nil {
					t.Fatal(err)
				}
				m["source_hash"] = "tampered"
				data, err = json.Marshal(m)
				if err == nil {
					err = os.WriteFile(filepath.Join(record, "metadata.json"), data, 0644)
				}
			case "manifest":
				err = os.WriteFile(filepath.Join(record, "manifest.json"), []byte(`{"version":999}`), 0644)
			case "invalid profile":
				err = os.WriteFile(filepath.Join(record, "gas_profile.json"), []byte("["), 0644)
			case "missing profile":
				err = os.Remove(filepath.Join(record, "gas_profile.json"))
			case "renamed binary":
				err = os.Rename(c.ExecutablePath, filepath.Join(record, "other-executable"))
			case "missing manifest":
				err = os.Remove(filepath.Join(record, "manifest.json"))
			}
			if err != nil {
				t.Fatal(err)
			}
			if _, err := e.GetContract(address); err == nil {
				t.Fatal("tampered record loaded")
			}
			if _, err := e.Execute(address, "F"); err == nil {
				t.Fatal("tampered record executed")
			}
		})
	}
}

func TestDeployRejectsChangedBinaryAndMetadata(t *testing.T) {
	e := NewVMEngine(VMConfig{ContractStorageDir: t.TempDir()})
	for _, target := range []string{"binary", "policy", "gas", "abi"} {
		t.Run(target, func(t *testing.T) {
			c := compileTestContract(t, t.TempDir(), "package main;func F()int{return 7}")
			switch target {
			case "binary":
				if err := os.WriteFile(c.ExecutablePath, []byte("tampered"), 0755); err != nil {
					t.Fatal(err)
				}
			case "policy":
				c.SourcePolicy = "changed"
			case "gas":
				c.GasProfile.EntryGas++
			case "abi":
				c.ABI.PackageName = "changed"
			}
			if _, err := e.Deploy(c); err == nil {
				t.Fatal("changed artifact deployed")
			}
		})
	}
}

func TestManifestRulesAndSourceDigest(t *testing.T) {
	source := "package main;func F()int{return 7}"
	c := compileTestContract(t, t.TempDir(), source)
	expectedSource := sha256.Sum256([]byte(source))
	if c.Manifest.Build.SourceSHA256 != fmt.Sprintf("%x", expectedSource) {
		t.Fatal("source digest does not describe original source")
	}
	bytes, readErr := os.ReadFile(c.ExecutablePath)
	if readErr != nil {
		t.Fatal(readErr)
	}
	expectedBinary := sha256.Sum256(bytes)
	if c.Manifest.BinarySHA256 != fmt.Sprintf("%x", expectedBinary) {
		t.Fatal("binary digest mismatch")
	}
	data, err := os.ReadFile(filepath.Join(filepath.Dir(c.ExecutablePath), "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"source_sha256", "binary_sha256", "abi_sha256", "gas_profile_sha256", "toolchain", "runtime_version", "gas_version"} {
		if !strings.Contains(string(data), field) {
			t.Errorf("manifest missing %s", field)
		}
	}
	_, err = NewProcessRunner(0).Run(context.Background(), c, CallRequest{Function: "F"})
	if err != nil {
		t.Fatal(err)
	}
}

func TestNativeRulesRejectWellFormedUnsupportedArtifacts(t *testing.T) {
	original := compileTestContract(t, t.TempDir(), "package main;func F()int{return 7}")
	cases := map[string]func(*core.ArtifactBuild){
		"runtime version": func(b *core.ArtifactBuild) { b.RuntimeVersion = "future-runtime" },
		"gas version":     func(b *core.ArtifactBuild) { b.GasVersion = "future-gas" },
		"runtime digest":  func(b *core.ArtifactBuild) { b.RuntimeSHA256 = artifact.Digest([]byte("foreign runtime")) },
		"os":              func(b *core.ArtifactBuild) { b.Toolchain.GOOS = "unsupported" },
		"arch":            func(b *core.ArtifactBuild) { b.Toolchain.GOARCH = "unsupported" },
		"cgo":             func(b *core.ArtifactBuild) { b.Toolchain.CGOEnabled = true },
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			c := *original
			build := c.Manifest.Build
			change(&build)
			if err := artifact.Seal(context.Background(), &c, build); err != nil {
				t.Fatal(err)
			}
			e := NewVMEngine(VMConfig{ContractStorageDir: t.TempDir()})
			_, err := e.Deploy(&c)
			requireCode(t, err, CodeUnsupportedVersion)
			_, err = NewProcessRunner(time.Second).Run(context.Background(), &c, CallRequest{Function: "F"})
			requireCode(t, err, CodeUnsupportedVersion)
		})
	}
}

func TestGetRejectsIncompatibleNativeRules(t *testing.T) {
	dir := t.TempDir()
	e := NewVMEngine(VMConfig{ContractStorageDir: dir})
	c := compileTestContract(t, t.TempDir(), "package main;func F()int{return 7}")
	build := c.Manifest.Build
	build.RuntimeVersion = "future-runtime"
	if err := artifact.Seal(context.Background(), c, build); err != nil {
		t.Fatal(err)
	}
	store, err := storage.New(dir)
	if err != nil {
		t.Fatal(err)
	}
	// A generic store persists complete artifacts independently of runtime policy.
	if err := store.Put(context.Background(), "future", c); err != nil {
		t.Fatal(err)
	}
	_, err = e.GetContract("future")
	requireCode(t, err, CodeUnsupportedVersion)
}

func TestCompileRejectsIncompleteCache(t *testing.T) {
	for _, target := range []string{"manifest.json", "manifest.sha256", "invalid manifest"} {
		t.Run(target, func(t *testing.T) {
			compiler := NewContractCompilerWithOptions(t.TempDir(), true)
			source := "package main;func F()int{return 7}"
			c, err := compiler.Compile(source)
			if err != nil {
				t.Fatal(err)
			}
			dir := filepath.Dir(c.ExecutablePath)
			if target == "invalid manifest" {
				err = os.WriteFile(filepath.Join(dir, "manifest.json"), []byte("["), 0644)
			} else {
				err = os.Remove(filepath.Join(dir, target))
			}
			if err != nil {
				t.Fatal(err)
			}
			_, err = compiler.Compile(source)
			requireCode(t, err, core.CodeArtifactIntegrity)
		})
	}
}
