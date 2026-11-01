//go:build upgrade

package upgradetest

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// instance is one running econumo binary. Its environment is built from
// scratch, so nothing from the developer's shell or a .env (cmd.Dir is a
// temp dir) reaches it.
type instance struct {
	label   string
	cmd     *exec.Cmd
	logPath string
	base    string
	exited  chan struct{}
	waitErr error
}

func environment(dir, databaseURL string, port int) []string {
	return []string{
		"HOME=" + dir,
		"PATH=/usr/bin:/bin",
		"DATABASE_URL=" + databaseURL,
		"PORT=" + strconv.Itoa(port),
		"ECONUMO_ALLOW_REGISTRATION=true",
		"ECONUMO_CHECK_UPDATES=false",
		"ECONUMO_LOG_LEVEL=info",
	}
}

func startServer(t *testing.T, label, bin, databaseURL string) *instance {
	t.Helper()
	port := freePort(t)
	dir := t.TempDir()
	logPath := filepath.Join(dir, "server.log")
	logFile, err := os.Create(logPath)
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(bin, "serve")
	cmd.Dir = dir
	cmd.Env = environment(dir, databaseURL, port)
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	if err := cmd.Start(); err != nil {
		logFile.Close()
		t.Fatalf("%s: start %s: %v", label, bin, err)
	}
	in := &instance{label: label, cmd: cmd, logPath: logPath, base: "http://127.0.0.1:" + strconv.Itoa(port), exited: make(chan struct{})}
	go func() {
		in.waitErr = cmd.Wait()
		logFile.Close()
		close(in.exited)
	}()
	t.Cleanup(func() { in.kill() })
	if err := in.waitHealthy(60 * time.Second); err != nil {
		in.kill()
		t.Fatalf("%s did not become healthy: %v\n--- server log ---\n%s", label, err, in.logs())
	}
	return in
}

func (in *instance) waitHealthy(timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	client := &http.Client{Timeout: 2 * time.Second}
	for time.Now().Before(deadline) {
		select {
		case <-in.exited:
			return fmt.Errorf("process exited: %v", in.waitErr)
		default:
		}
		resp, err := client.Get(in.base + "/health")
		if err == nil {
			var body struct {
				Data struct {
					Database bool `json:"database"`
				} `json:"data"`
			}
			decodeErr := json.NewDecoder(resp.Body).Decode(&body)
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK && decodeErr == nil && body.Data.Database {
				return nil
			}
		}
		time.Sleep(200 * time.Millisecond)
	}
	return fmt.Errorf("timed out after %s", timeout)
}

// stop ends the server gracefully, so SQLite has released the file before the
// next binary opens it.
func (in *instance) stop(t *testing.T) {
	t.Helper()
	_ = in.cmd.Process.Signal(syscall.SIGTERM)
	select {
	case <-in.exited:
		if in.waitErr != nil {
			t.Fatalf("%s exited with %v on SIGTERM\n--- server log ---\n%s", in.label, in.waitErr, in.logs())
		}
	case <-time.After(20 * time.Second):
		in.kill()
		t.Fatalf("%s did not stop within 20s\n--- server log ---\n%s", in.label, in.logs())
	}
}

func (in *instance) kill() {
	select {
	case <-in.exited:
	default:
		_ = in.cmd.Process.Kill()
		<-in.exited
	}
}

func (in *instance) logs() string {
	raw, err := os.ReadFile(in.logPath)
	if err != nil {
		return "(no log: " + err.Error() + ")"
	}
	return string(raw)
}

// errorLines returns the log lines at ERROR level.
func (in *instance) errorLines() []string {
	var out []string
	for _, line := range strings.Split(in.logs(), "\n") {
		if strings.Contains(line, "level=ERROR") {
			out = append(out, line)
		}
	}
	return out
}

// runCLI runs a management command against a database no server holds open.
func runCLI(t *testing.T, bin, databaseURL string, args ...string) {
	t.Helper()
	dir := t.TempDir()
	cmd := exec.Command(bin, args...)
	cmd.Dir = dir
	cmd.Env = environment(dir, databaseURL, 0)
	if raw, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%s %s: %v\n%s", bin, strings.Join(args, " "), err, raw)
	}
}

func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

// buildCurrent compiles this checkout's binary once per test run.
func buildCurrent(ctx context.Context, repoRoot, out string) error {
	cmd := exec.CommandContext(ctx, "go", "build", "-o", out, "./cmd/econumo")
	cmd.Dir = repoRoot
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	if raw, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("go build: %v\n%s", err, raw)
	}
	return nil
}
