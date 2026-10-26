// Package sandbox contains the Docker isolation boundary shared by build and run.
// The Docker daemon and the configured immutable image are trusted infrastructure.
package sandbox

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Binary   string `json:"binary"`
	Image    string `json:"image"`
	MemoryMB int    `json:"memoryMB"`
	CPUs     int    `json:"cpus"`
	PIDs     int    `json:"pids"`
	TmpMB    int    `json:"tmpMB"`
}
type Docker struct {
	config        Config
	createTimeout time.Duration
}
type Mount struct {
	Source, Target string
	ReadOnly       bool
}

var imageID = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)

func New(c Config) (*Docker, error) {
	if !imageID.MatchString(c.Image) {
		return nil, fmt.Errorf("sandbox image must be a local immutable sha256 image ID")
	}
	if c.MemoryMB < 0 || c.CPUs < 0 || c.PIDs < 0 || c.TmpMB < 0 {
		return nil, fmt.Errorf("sandbox resource limits must be positive")
	}
	if c.Binary == "" {
		c.Binary = "docker"
	}
	if c.MemoryMB == 0 {
		c.MemoryMB = 256
	}
	if c.CPUs == 0 {
		c.CPUs = 1
	}
	if c.PIDs == 0 {
		c.PIDs = 128
	}
	if c.TmpMB == 0 {
		c.TmpMB = 64
	}
	return &Docker{config: c, createTimeout: 15 * time.Second}, nil
}
func (d *Docker) Image() string { return d.config.Image }
func (d *Docker) createArgs(name, entry string, env []string, mounts []Mount) ([]string, error) {
	c := d.config
	args := []string{"create", "--name", name, "--pull", "never", "--network", "none", "--read-only", "--user", "65532:65532", "--cap-drop", "ALL", "--security-opt", "no-new-privileges", "--pids-limit", strconv.Itoa(c.PIDs), "--memory", fmt.Sprintf("%dm", c.MemoryMB), "--memory-swap", fmt.Sprintf("%dm", c.MemoryMB), "--cpus", strconv.Itoa(c.CPUs), "--ulimit", "nofile=256:256", "--ipc", "private", "--cgroupns", "private", "--log-driver", "none", "--no-healthcheck", "--tmpfs", fmt.Sprintf("/tmp:rw,nosuid,nodev,noexec,size=%dm,mode=1777", c.TmpMB), "--workdir", "/tmp", "-i", "--entrypoint", entry}
	for _, e := range env {
		if !strings.Contains(e, "=") {
			return nil, fmt.Errorf("sandbox env requires explicit value")
		}
		args = append(args, "--env", e)
	}
	for _, m := range mounts {
		src, err := filepath.Abs(m.Source)
		if err != nil {
			return nil, err
		}
		if strings.ContainsAny(src, ",\n\r") || !strings.HasPrefix(m.Target, "/") || strings.ContainsAny(m.Target, ",\n\r") {
			return nil, fmt.Errorf("invalid sandbox mount")
		}
		spec := "type=bind,src=" + src + ",dst=" + m.Target
		if m.ReadOnly {
			spec += ",readonly"
		}
		args = append(args, "--mount", spec)
	}
	return append(args, c.Image), nil
}

// Prepare creates a stopped container before returning its attached start command.
// Callers must invoke cleanup after Wait, including on startup failure. Cleanup
// failures must propagate so successful VM results cannot commit with leaked work.
func (d *Docker) Prepare(ctx context.Context, entry string, env []string, mounts []Mount, command ...string) (*exec.Cmd, func() error, error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		return nil, nil, err
	}
	name := "vm-d1-" + hex.EncodeToString(id[:])
	args, err := d.createArgs(name, entry, env, mounts)
	if err != nil {
		return nil, nil, err
	}
	args = append(args, command...)
	cleanup := func() error {
		cleanCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		out, err := d.control(cleanCtx, "rm", "-f", name)
		if err != nil && !strings.Contains(out, "No such container") {
			return fmt.Errorf("sandbox cleanup %s: %w: %s", name, err, out)
		}
		return nil
	}
	// Do not kill a create request half way through a daemon mutation on caller
	// cancellation: finish this bounded control request, then remove by known name.
	createCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), d.createTimeout)
	defer cancel()
	out, err := d.control(createCtx, args...)
	if err != nil {
		cleanErr := cleanup()
		if createCtx.Err() != nil {
			cleanErr = errors.Join(cleanErr, fmt.Errorf("sandbox create outcome uncertain for %s; reconcile this container on the daemon", name))
		}
		return nil, nil, errors.Join(fmt.Errorf("sandbox create %s: %w: %s", name, err, out), cleanErr)
	}
	if err := ctx.Err(); err != nil {
		return nil, nil, errors.Join(err, cleanup())
	}
	cmd := exec.CommandContext(ctx, d.config.Binary, "start", "--attach", "--interactive", name)
	cmd.WaitDelay = 100 * time.Millisecond
	return cmd, cleanup, nil
}

// Run is for bounded control/build operations, never for the interactive protocol.
func (d *Docker) Run(ctx context.Context, entry string, env []string, mounts []Mount, command ...string) (string, error) {
	cmd, cleanup, err := d.Prepare(ctx, entry, env, mounts, command...)
	if err != nil {
		return "", err
	}
	var out limitedBuffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err = cmd.Run()
	if ctx.Err() != nil {
		err = ctx.Err()
	}
	return out.String(), errors.Join(err, cleanup())
}
func (d *Docker) control(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, d.config.Binary, args...)
	cmd.WaitDelay = 100 * time.Millisecond
	var out limitedBuffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	return out.String(), err
}

type limitedBuffer struct{ bytes.Buffer }

func (b *limitedBuffer) Write(p []byte) (int, error) {
	n := len(p)
	if left := 64*1024 - b.Len(); left > 0 {
		if len(p) > left {
			p = p[:left]
		}
		_, _ = b.Buffer.Write(p)
	}
	return n, nil
}
