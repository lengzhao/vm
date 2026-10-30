package execution

import (
	"bytes"
	"context"
	"github.com/lengzhao/vm/core"
	"github.com/lengzhao/vm/internal/state"
	"io"
	"testing"
)

func TestSynchronousHostReplyAndLargeNumbers(t *testing.T) {
	tx, _ := state.New().Begin(context.Background())
	defer tx.Rollback()
	ctx := core.WithObjectStore(context.Background(), state.Bind(tx, "contract"))
	var input bytes.Buffer
	session := NewSession(ctx, nil, &input, func() {})
	_, err := io.WriteString(session.Stdout(), `{"version":3,"type":"object","op":"set","key":"x","value":"eQ==","gas":20}`+"\n")
	if err != nil || input.Len() == 0 {
		t.Fatalf("host reply missing: %v", err)
	}
	io.WriteString(session.Stdout(), `{"version":3,"ok":true,"gas":20,"result":18446744073709551615}`)
	result, err := session.Result(nil)
	if err != nil || string(result.Data) != "18446744073709551615" {
		t.Fatalf("result=%v err=%v", result, err)
	}
}
func TestSessionRejectsMultipleResponses(t *testing.T) {
	s := NewSession(context.Background(), nil, io.Discard, func() {})
	io.WriteString(s.Stdout(), `{"version":3,"ok":true,"gas":10}{}`)
	if _, err := s.Result(nil); err == nil {
		t.Fatal("multiple responses accepted")
	}
}
