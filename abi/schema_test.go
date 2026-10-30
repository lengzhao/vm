package abi

import "testing"

func TestTypeSyntaxIsPreserved(t *testing.T) {
	a, err := ExtractABI([]byte(`package main;func F(a [3]int,b <-chan int,c chan<- string,d ...bool){}`))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"[3]int", "<-chan int", "chan<- string", "...bool"}
	for i, p := range a.Functions[0].Inputs {
		if p.Type != want[i] {
			t.Fatalf("want %s, got %s", want[i], p.Type)
		}
	}
	if a.Version != SchemaVersion {
		t.Fatal("missing schema version")
	}
}

func TestEventsUseRealAPIBindingAndValueTypes(t *testing.T) {
	code := []byte("package main\nimport api \"github.com/lengzhao/vm/contractapi\"\nconst Name = `Transfer`\nfunc F(amount int64){delta:=int64(7);api.Log(Name,`amount`,amount,`delta`,delta);api:=struct{Log func(string)}{};api.Log(\"Fake\")}")
	a, err := ExtractABI(code)
	if err != nil {
		t.Fatal(err)
	}
	if len(a.Events) != 1 || a.Events[0].Name != "Transfer" {
		t.Fatalf("wrong events: %+v", a.Events)
	}
	for _, p := range a.Events[0].Parameters {
		if p.Type != "int64" {
			t.Fatalf("wrong event type: %+v", p)
		}
	}
}
