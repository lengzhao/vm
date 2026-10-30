package sandbox

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDockerCleanupAfterStartAndFailure(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(map[bool]string{false: "success", true: "create failure"}[fail], func(t *testing.T) {
			dir := t.TempDir()
			log := filepath.Join(dir, "calls")
			cli := filepath.Join(dir, "docker")
			script := "#!/bin/sh\necho \"$1\" >> '" + log + "'\n"
			if fail {
				script += "if [ \"$1\" = create ]; then exit 1; fi\n"
			}
			script += "exit 0\n"
			if err := os.WriteFile(cli, []byte(script), 0700); err != nil {
				t.Fatal(err)
			}
			d, _ := New(Config{Binary: cli, Image: "sha256:" + strings.Repeat("a", 64)})
			_, err := d.Run(context.Background(), "/artifact", nil, nil)
			if (err != nil) != fail {
				t.Fatalf("got %v", err)
			}
			data, _ := os.ReadFile(log)
			want := "create\nstart\nrm\n"
			if fail {
				want = "create\nrm\n"
			}
			if string(data) != want {
				t.Fatalf("calls: %q", data)
			}
		})
	}
}
func TestDockerCleanupFailureCannotSucceed(t *testing.T) {
	cli := filepath.Join(t.TempDir(), "docker")
	os.WriteFile(cli, []byte("#!/bin/sh\nif [ \"$1\" = rm ]; then exit 1; fi\n"), 0700)
	d, _ := New(Config{Binary: cli, Image: "sha256:" + strings.Repeat("a", 64)})
	if _, err := d.Run(context.Background(), "/artifact", nil, nil); err == nil {
		t.Fatal("cleanup failure lost")
	}
}
func TestDockerCancellationCleansContainer(t *testing.T) {
	dir := t.TempDir()
	log := filepath.Join(dir, "calls")
	started := filepath.Join(dir, "started")
	cli := filepath.Join(dir, "docker")
	os.WriteFile(cli, []byte("#!/bin/sh\necho \"$1\" >> '"+log+"'\nif [ \"$1\" = start ]; then touch '"+started+"'; exec sleep 30; fi\n"), 0700)
	d, _ := New(Config{Binary: cli, Image: "sha256:" + strings.Repeat("a", 64)})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { _, err := d.Run(ctx, "/artifact", nil, nil); done <- err }()
	deadline := time.After(5 * time.Second)
	for {
		if _, err := os.Stat(started); err == nil {
			break
		}
		select {
		case <-deadline:
			t.Fatal("start not observed")
		case <-time.After(10 * time.Millisecond):
		}
	}
	cancel()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("canceled run succeeded")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("cancellation blocked")
	}
	data, _ := os.ReadFile(log)
	if !strings.HasSuffix(string(data), "rm\n") {
		t.Fatalf("not cleaned: %s", data)
	}
}

func TestDockerCreateFailureIncludesContainerIdentity(t *testing.T) {
	cli := filepath.Join(t.TempDir(), "docker")
	os.WriteFile(cli, []byte("#!/bin/sh\nif [ \"$1\" = create ]; then exit 1; fi\necho 'No such container' >&2; exit 1\n"), 0700)
	d, _ := New(Config{Binary: cli, Image: "sha256:" + strings.Repeat("a", 64)})
	_, err := d.Run(context.Background(), "/artifact", nil, nil)
	if err == nil || !strings.Contains(err.Error(), "vm-d1-") {
		t.Fatalf("cannot reconcile failed create: %v", err)
	}
}

func TestDockerCreateTimeoutNeverStartsAndReportsUncertainty(t *testing.T) {
	dir := t.TempDir()
	log := filepath.Join(dir, "calls")
	cli := filepath.Join(dir, "docker")
	os.WriteFile(cli, []byte("#!/bin/sh\necho \"$1\" >> '"+log+"'\nif [ \"$1\" = create ]; then exec sleep 1; fi\necho 'No such container' >&2; exit 1\n"), 0700)
	d, _ := New(Config{Binary: cli, Image: "sha256:" + strings.Repeat("a", 64)})
	d.createTimeout = 100 * time.Millisecond
	_, err := d.Run(context.Background(), "/artifact", nil, nil)
	if err == nil || !strings.Contains(err.Error(), "outcome uncertain") || !strings.Contains(err.Error(), "vm-d1-") {
		t.Fatalf("uncertainty lost: %v", err)
	}
	calls, _ := os.ReadFile(log)
	if strings.Contains(string(calls), "start") {
		t.Fatalf("started uncertain container: %s", calls)
	}
}
