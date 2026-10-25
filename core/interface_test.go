package core_test

import (
	"github.com/lengzhao/vm/core"
	"reflect"
	"testing"
)

func TestCompilerHasOnlyCompilationResponsibility(t *testing.T) {
	typ := reflect.TypeFor[core.ContractCompiler]()
	if typ.NumMethod() != 1 || typ.Method(0).Name != "Compile" {
		t.Fatalf("compiler exposes unrelated methods: %v", typ)
	}
}
func TestReviewerDoesNotRequireAllowlistImplementation(t *testing.T) {
	typ := reflect.TypeFor[core.SecurityReviewer]()
	if typ.NumMethod() != 1 || typ.Method(0).Name != "Review" {
		t.Fatalf("reviewer exposes allowlist details: %v", typ)
	}
}
func TestEngineExcludesCompatibilityAndLifecycle(t *testing.T) {
	typ := reflect.TypeFor[core.Engine]()
	for _, name := range []string{"GetLastEvents", "GetGasConsumed", "GetConfig", "GetVersion", "String", "Stop"} {
		if _, ok := typ.MethodByName(name); ok {
			t.Errorf("engine still requires %s", name)
		}
	}
}
