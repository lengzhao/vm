package sandbox

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func integrationDocker(t *testing.T, c Config) *Docker {
	t.Helper()
	c.Image = os.Getenv("VM_DOCKER_IMAGE")
	if c.Image == "" {
		t.Skip("set VM_DOCKER_IMAGE for real Docker isolation tests")
	}
	d, err := New(c)
	if err != nil {
		t.Fatal(err)
	}
	return d
}
func TestDockerRealIsolationAndResourceLimits(t *testing.T) {
	d := integrationDocker(t, Config{MemoryMB: 64, PIDs: 24, TmpMB: 1})
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	sentinel := t.TempDir() + "/secret"
	os.WriteFile(sentinel, []byte("secret"), 0600)
	script := `test "$(id -u)" != 0 && test ! -e "$1" && ! touch /forbidden && ! test -e /var/run/docker.sock && test "$(cat /sys/fs/cgroup/pids.max)" = 24 && test "$(cat /sys/fs/cgroup/memory.max)" = 67108864 && test "$(cat /sys/fs/cgroup/cpu.max)" = '100000 100000' && ! dd if=/dev/zero of=/tmp/full bs=1M count=2 && echo isolated`
	out, err := d.Run(ctx, "/bin/sh", nil, nil, "-c", script, "sh", sentinel)
	if err != nil || !strings.Contains(out, "isolated") {
		t.Fatalf("isolation failed: %v %s", err, out)
	}
}
func TestDockerRealOOM(t *testing.T) {
	d := integrationDocker(t, Config{MemoryMB: 32, TmpMB: 128})
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	// tmpfs consumes cgroup memory; this must be killed before filling 128 MiB.
	out, err := d.Run(ctx, "/bin/sh", nil, nil, "-c", "dd if=/dev/zero of=/tmp/oom bs=1M count=120")
	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.ExitCode() != 137 || ctx.Err() != nil {
		t.Fatalf("expected resource failure, got %v %s", err, out)
	}
}
func TestDockerRealCancellationRemovesDescendants(t *testing.T) {
	d := integrationDocker(t, Config{})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cmd, cleanup, err := d.Prepare(ctx, "/bin/sh", nil, nil, "-c", "sleep 60 & echo ready; wait")
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	// Observe actual container startup independently from the cancellation budget.
	pipe, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	ready := make(chan error, 1)
	go func() {
		buf := make([]byte, 6)
		_, err := pipe.Read(buf)
		if string(buf) != "ready\n" && err == nil {
			err = context.Canceled
		}
		ready <- err
	}()
	select {
	case err := <-ready:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("container did not start")
	}
	cancel()
	_ = cmd.Wait()
	if err := cleanup(); err != nil {
		t.Fatal(err)
	}
	name := cmd.Args[len(cmd.Args)-1]
	out, err := exec.Command("docker", "inspect", name).CombinedOutput()
	if err == nil || (!strings.Contains(string(out), "No such object") && !strings.Contains(string(out), "No such container")) {
		t.Fatalf("container removal not confirmed: %v %s", err, out)
	}
}

func TestDockerRealPIDExhaustion(t *testing.T) {
	d := integrationDocker(t, Config{PIDs: 24})
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	out, err := d.Run(ctx, "/bin/sh", nil, nil, "-c", `i=0; while [ "$i" -lt 64 ]; do /bin/sleep 30 & i=$((i+1)); done; exit 0`)
	if err == nil || ctx.Err() != nil || (!strings.Contains(strings.ToLower(out), "cannot fork") && !strings.Contains(strings.ToLower(out), "resource temporarily unavailable")) {
		t.Fatalf("expected PID exhaustion: %v %s", err, out)
	}
}

func TestDockerRealBuildMountBoundary(t *testing.T) {
	d := integrationDocker(t, Config{})
	source, outDir := t.TempDir(), t.TempDir()
	if err := os.Chmod(source, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(outDir, 0777); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(source+"/input", []byte("original"), 0644); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	out, err := d.Run(ctx, "/bin/sh", nil, []Mount{{Source: source, Target: "/src", ReadOnly: true}, {Source: outDir, Target: "/out"}}, "-c", `test "$(cat /src/input)" = original && ! sh -c 'echo changed > /src/input' && echo artifact > /out/artifact`)
	if err != nil {
		t.Fatalf("build mount boundary: %v %s", err, out)
	}
	original, _ := os.ReadFile(source + "/input")
	built, _ := os.ReadFile(outDir + "/artifact")
	if string(original) != "original" || string(built) != "artifact\n" {
		t.Fatalf("source=%q output=%q", original, built)
	}
}
