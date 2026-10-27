package compiler

import (
	"context"
	"encoding/hex"
	"fmt"
	"github.com/lengzhao/vm/internal/artifact"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// toolchain is resolved per compilation so a later PATH change cannot silently
// change the binary between version validation and the build command.
type toolchain struct {
	goarch       string
	goos         string
	image        string
	binarySHA256 string
	binary       string
	identity     string
	env          []string
}

func buildEnvironment() []string {
	// Ignore ambient toolchain, target, workspace and persisted Go configuration.
	overridden := map[string]bool{
		"GOROOT": true, "GOTOOLDIR": true, "GOENV": true,
		"GOOS": true, "GOARCH": true, "CGO_ENABLED": true,
		"GOTOOLCHAIN": true, "GOFLAGS": true, "GOEXPERIMENT": true,
		"GOWORK": true, "GO111MODULE": true, "GOFIPS140": true,
		"GOAMD64": true, "GOARM64": true, "GOARM": true,
		"GOMIPS": true, "GOMIPS64": true, "GOPPC64": true,
		"GORISCV64": true, "GOWASM": true,
	}
	var env []string
	for _, value := range os.Environ() {
		key, _, _ := strings.Cut(value, "=")
		if !overridden[key] {
			env = append(env, value)
		}
	}
	return append(env, "GOENV=off", "GOOS="+runtime.GOOS, "GOARCH="+runtime.GOARCH, "CGO_ENABLED=0", "GOTOOLCHAIN=local", "GOFLAGS=", "GOEXPERIMENT=", "GOWORK=off", "GO111MODULE=on")
}

func (c *Compiler) resolveToolchain(ctx context.Context) (toolchain, error) {
	targetOS, targetArch := runtime.GOOS, runtime.GOARCH
	if c.sandbox != nil {
		targetOS = "linux"
	}
	if c.wasi {
		targetOS, targetArch = "wasip1", "wasm"
	}
	if c.sandbox != nil {
		env := []string{"PATH=/usr/local/go/bin:/usr/bin:/bin", "HOME=/tmp", "GOCACHE=/tmp/gocache", "GOPATH=/tmp/gopath", "GOTMPDIR=/tmp", "GOENV=off", "GOTOOLCHAIN=local", "GOOS=" + targetOS, "GOARCH=" + targetArch, "CGO_ENABLED=0", "GOWORK=off", "GOFLAGS=", "GOEXPERIMENT=", "GO111MODULE=on", "GOPROXY=off", "GOSUMDB=off"}
		out, err := c.sandbox.Run(ctx, "/bin/sh", env, nil, "-c", "/usr/local/go/bin/go version && sha256sum /usr/local/go/bin/go")
		if err != nil {
			return toolchain{}, fmt.Errorf("probe sandbox toolchain: %w: %s", err, out)
		}
		lines := strings.Split(strings.TrimSpace(out), "\n")
		want := "go version " + c.goVersion + " linux/" + runtime.GOARCH
		if len(lines) != 2 || strings.TrimSpace(lines[0]) != want {
			return toolchain{}, fmt.Errorf("sandbox Go toolchain mismatch: want %q, got %q", want, out)
		}
		fields := strings.Fields(lines[1])
		if len(fields) != 2 {
			return toolchain{}, fmt.Errorf("invalid sandbox toolchain digest")
		}
		digest, err := hex.DecodeString(fields[0])
		if err != nil || len(digest) != 32 {
			return toolchain{}, fmt.Errorf("invalid sandbox toolchain digest")
		}
		return toolchain{binary: "/usr/local/go/bin/go", binarySHA256: fields[0], identity: want, env: env, goos: targetOS, goarch: targetArch, image: c.sandbox.Image()}, nil
	}

	binary, err := exec.LookPath(c.goBinary)
	if err != nil {
		return toolchain{}, fmt.Errorf("resolve Go toolchain: %w", err)
	}
	binary, err = filepath.Abs(binary)
	if err != nil {
		return toolchain{}, err
	}
	binary, err = filepath.EvalSymlinks(binary)
	if err != nil {
		return toolchain{}, err
	}
	digest, err := artifact.FileDigest(ctx, binary)
	if err != nil {
		return toolchain{}, err
	}
	tc := toolchain{goos: targetOS, goarch: targetArch, binary: binary, binarySHA256: digest, env: buildEnvironment()}
	if c.wasi {
		for i, v := range tc.env {
			if strings.HasPrefix(v, "GOOS=") {
				tc.env[i] = "GOOS=wasip1"
			}
			if strings.HasPrefix(v, "GOARCH=") {
				tc.env[i] = "GOARCH=wasm"
			}
		}
	}
	cmd := exec.CommandContext(ctx, binary, "version")
	cmd.Env = tc.env
	cmd.WaitDelay = 100 * time.Millisecond
	var output diagnosticBuffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return toolchain{}, fmt.Errorf("probe Go toolchain: %w", ctx.Err())
		}
		return toolchain{}, fmt.Errorf("probe Go toolchain: %w: %s", err, strings.TrimSpace(output.String()))
	}
	want := "go version " + c.goVersion + " " + runtime.GOOS + "/" + runtime.GOARCH
	tc.identity = strings.TrimSpace(output.String())
	if tc.identity != want {
		return toolchain{}, fmt.Errorf("Go toolchain mismatch: want %q, got %q", want, tc.identity)
	}
	if err := ctx.Err(); err != nil {
		return toolchain{}, err
	}
	return tc, nil
}
