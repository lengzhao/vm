package process

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lengzhao/vm/core"
	"github.com/lengzhao/vm/internal/artifact"
	"github.com/lengzhao/vm/internal/artifact/artifacttest"
	"github.com/lengzhao/vm/internal/sandbox"
)

func TestDockerCleanupFailureRejectsSuccessfulResponse(t *testing.T) {
	cli := filepath.Join(t.TempDir(), "docker")
	script := `#!/bin/sh
case "$1" in
 create) exit 0;;
 start) read request; echo '{"version":3,"ok":true,"gas":1,"result":7}';;
 rm) echo 'daemon cleanup failure' >&2; exit 1;;
esac
`
	if err := os.WriteFile(cli, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	c := artifacttest.Seal(t, &core.CompiledContract{ProtocolVersion: core.ProtocolVersion})
	b := c.Manifest.Build
	b.Toolchain.GOOS = "linux"
	if err := artifact.Seal(context.Background(), c, b); err != nil {
		t.Fatal(err)
	}
	d, err := sandbox.New(sandbox.Config{Binary: cli, Image: "sha256:" + strings.Repeat("a", 64)})
	if err != nil {
		t.Fatal(err)
	}
	_, err = NewDocker(time.Second, d).Run(context.Background(), c, core.CallRequest{Function: "Test"})
	var e *core.ExecutionError
	if !errors.As(err, &e) || e.Code != core.CodeProcessFailure || !strings.Contains(err.Error(), "cleanup") {
		t.Fatalf("successful response escaped cleanup failure: %v", err)
	}
}

func TestDockerCancellationDuringCreateKeepsErrorCode(t *testing.T) {
	dir := t.TempDir()
	cli := filepath.Join(dir, "docker")
	marker := filepath.Join(dir, "created")
	script := "#!/bin/sh\nif [ \"$1\" = create ]; then touch '" + marker + "'; sleep 0.2; fi\n"
	if err := os.WriteFile(cli, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	c := artifacttest.Seal(t, &core.CompiledContract{ProtocolVersion: core.ProtocolVersion})
	b := c.Manifest.Build
	b.Toolchain.GOOS = "linux"
	if err := artifact.Seal(context.Background(), c, b); err != nil {
		t.Fatal(err)
	}
	d, _ := sandbox.New(sandbox.Config{Binary: cli, Image: "sha256:" + strings.Repeat("a", 64)})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := NewDocker(5*time.Second, d).Run(ctx, c, core.CallRequest{Function: "Test"})
		done <- err
	}()
	until := time.After(3 * time.Second)
	for {
		if _, err := os.Stat(marker); err == nil {
			break
		}
		select {
		case <-until:
			t.Fatal("create not observed")
		case <-time.After(10 * time.Millisecond):
		}
	}
	cancel()
	select {
	case err := <-done:
		var e *core.ExecutionError
		if !errors.As(err, &e) || e.Code != core.CodeCanceled {
			t.Fatalf("cancellation misclassified: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("cancel did not finish")
	}
}
