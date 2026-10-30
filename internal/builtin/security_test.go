package vm

import (
	"testing"
)

func TestNewSecurityReviewer(t *testing.T) {
	reviewer := NewSecurityReviewer()
	if reviewer == nil {
		t.Error("Expected SecurityReviewer to be created, got nil")
	}
}

func TestIsKeywordAllowed(t *testing.T) {
	reviewer := NewSecurityReviewer()

	allowedKeywords := []string{"int", "string", "if", "for", "func", "package"}
	for _, keyword := range allowedKeywords {
		if !reviewer.IsKeywordAllowed(keyword) {
			t.Errorf("Expected keyword '%s' to be allowed", keyword)
		}
	}

	disallowedKeywords := []string{"unsafe", "go", "select", "chan", "goto", "map", "cap"}
	for _, keyword := range disallowedKeywords {
		if reviewer.IsKeywordAllowed(keyword) {
			t.Errorf("Expected keyword '%s' to be disallowed", keyword)
		}
	}
}

func TestIsImportAllowed(t *testing.T) {
	reviewer := NewSecurityReviewer()

	allowedImports := []string{"fmt", "strconv", "math", "time", "errors", "github.com/lengzhao/vm/contractapi"}
	for _, imp := range allowedImports {
		if !reviewer.IsImportAllowed(imp) {
			t.Errorf("Expected import '%s' to be allowed", imp)
		}
	}

	// 宿主根包不得被合约导入
	if reviewer.IsImportAllowed("github.com/lengzhao/vm") {
		t.Error("Expected host root package import to be disallowed")
	}

	disallowedImports := []string{"os", "net", "syscall", "unsafe"}
	for _, imp := range disallowedImports {
		if reviewer.IsImportAllowed(imp) {
			t.Errorf("Expected import '%s' to be disallowed", imp)
		}
	}
}

func TestReview_ValidCode(t *testing.T) {
	reviewer := NewSecurityReviewer()

	validCode := `
package main

import (
	"fmt"
	"strconv"
)

func Hello() {
	fmt.Println("Hello, World!")
}

func Add(a, b int) int {
	return a + b
}
`

	if err := reviewer.Review(validCode); err != nil {
		t.Errorf("Expected no error for valid code, got %v", err)
	}
}

func TestReview_InvalidImport(t *testing.T) {
	reviewer := NewSecurityReviewer()

	invalidCode := `
package main

import (
	"os"
)

func Hello() {}
`

	err := reviewer.Review(invalidCode)
	if err == nil {
		t.Fatal("Expected error for invalid import, got nil")
	}

	secErr, ok := err.(*SecurityError)
	if !ok {
		t.Fatalf("Expected SecurityError, got %T", err)
	}
	if secErr.ErrorType != ImportNotAllowed {
		t.Errorf("Expected ImportNotAllowed error type, got %v", secErr.ErrorType)
	}
}

func TestReview_ForbiddenKeywords(t *testing.T) {
	reviewer := NewSecurityReviewer()

	cases := []struct {
		name string
		code string
		key  string
	}{
		{
			name: "goroutine",
			code: "package main\nfunc Hello() { go Hello() }\n",
			key:  "go",
		},
		{
			name: "channel",
			code: "package main\nfunc Hello() { var c chan int; _ = c }\n",
			key:  "chan",
		},
		{
			name: "map",
			code: "package main\nfunc Hello() { var m map[string]int; _ = m }\n",
			key:  "map",
		},
		{
			name: "goto",
			code: "package main\nfunc Hello() { goto Label\nLabel:\n}\n",
			key:  "goto",
		},
		{
			name: "cap",
			code: "package main\nfunc Hello() { s := make([]int, 0, 1); _ = cap(s) }\n",
			key:  "cap",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := reviewer.Review(tc.code)
			if err == nil {
				t.Fatalf("Expected error for %s", tc.name)
			}
			secErr, ok := err.(*SecurityError)
			if !ok {
				t.Fatalf("Expected SecurityError, got %T", err)
			}
			if secErr.ErrorType != KeywordNotAllowed {
				t.Errorf("Expected KeywordNotAllowed, got %v", secErr.ErrorType)
			}
			if secErr.Keyword != tc.key {
				t.Errorf("Expected keyword %s, got %s", tc.key, secErr.Keyword)
			}
		})
	}
}

func TestReview_GlobalVarNotAllowed(t *testing.T) {
	reviewer := NewSecurityReviewer()

	code := `
package main

var counter int

func Hello() int {
	return counter
}
`
	err := reviewer.Review(code)
	if err == nil {
		t.Fatal("Expected error for package-level var")
	}
	secErr, ok := err.(*SecurityError)
	if !ok {
		t.Fatalf("Expected SecurityError, got %T", err)
	}
	if secErr.ErrorType != GlobalVarNotAllowed {
		t.Errorf("Expected GlobalVarNotAllowed, got %v", secErr.ErrorType)
	}
}

func TestReview_HostRootImportNotAllowed(t *testing.T) {
	reviewer := NewSecurityReviewer()
	code := `
package main

import "github.com/lengzhao/vm"

func Hello() {}
`
	err := reviewer.Review(code)
	if err == nil {
		t.Fatal("expected host root import to be rejected")
	}
}

func TestReview_ConstAllowed(t *testing.T) {
	reviewer := NewSecurityReviewer()

	code := `
package main

const Max = 100

func Hello() int {
	return Max
}
`
	if err := reviewer.Review(code); err != nil {
		t.Errorf("Expected const to be allowed, got %v", err)
	}
}
