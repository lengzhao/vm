package sandbox

import (
	"context"
	"strings"
	"testing"
)

func TestDockerRejectsUnsafeConfiguration(t *testing.T) {
	for _, c := range []Config{{}, {Image: "golang:latest"}, {Image: "sha256:" + strings.Repeat("a", 64), MemoryMB: -1}, {Image: "sha256:" + strings.Repeat("a", 64), PIDs: -1}, {Image: "sha256:" + strings.Repeat("a", 64), CPUs: -1}} {
		if _, err := New(c); err == nil {
			t.Fatalf("accepted unsafe config: %+v", c)
		}
	}
}
func TestDockerIsolationArguments(t *testing.T) {
	d, err := New(Config{Image: "sha256:" + strings.Repeat("a", 64)})
	if err != nil {
		t.Fatal(err)
	}
	args, err := d.createArgs("vm-test", "/artifact", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(args, " ")
	for _, s := range []string{"--network none", "--read-only", "--user 65532:65532", "--cap-drop ALL", "--security-opt no-new-privileges", "--pids-limit 128", "--memory 256m", "--memory-swap 256m", "--cpus 1", "--log-driver none", "--pull never", "--entrypoint /artifact"} {
		if !strings.Contains(joined, s) {
			t.Errorf("missing %s in %s", s, joined)
		}
	}
}
func TestDockerPreCanceledDoesNotInvokeCLI(t *testing.T) {
	d, err := New(Config{Binary: "/does/not/exist", Image: "sha256:" + strings.Repeat("a", 64)})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err = d.Prepare(ctx, "/artifact", nil, nil); err != context.Canceled {
		t.Fatalf("got %v", err)
	}
}
