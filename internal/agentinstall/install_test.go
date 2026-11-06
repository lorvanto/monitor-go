package agentinstall

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const fakeAgent = `#!/bin/sh
set -eu
printf 'agent %s\n' "$*" >> "$STUB_LOG"
config=
enrollment=
while [ "$#" -gt 0 ]; do
    case "$1" in
        -once) shift ;;
        -server) shift 2 ;;
        -config) config=$2; shift 2 ;;
        -enrollment-file) enrollment=$2; shift 2 ;;
        *) exit 2 ;;
    esac
done
if [ ! -f "$config" ]; then
    token=$(cat "$enrollment")
    printf '{"credential":"persisted-once","enrollmentToken":"%s"}\n' "$token" > "$config"
fi
: > "$config.lock"
[ "${STUB_BOOTSTRAP_FAIL:-0}" = 0 ]
`

type installerFixture struct {
	root, script, log, url string
	corrupt                atomic.Bool
	requests               atomic.Int32
}

func installerTestFixture(t *testing.T) *installerFixture {
	t.Helper()
	for _, tool := range []string{"sh", "curl", "sha256sum"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("%s is required for installer integration tests", tool)
		}
	}
	f := &installerFixture{root: t.TempDir()}
	f.log = filepath.Join(f.root, "commands.log")
	for _, dir := range []string{"stubs", "state-parent", "bin", "etc", "run", "tmp"} {
		if err := os.Mkdir(filepath.Join(f.root, dir), 0700); err != nil {
			t.Fatal(err)
		}
	}
	stubs := map[string]string{
		"id":    `case "$*" in -u) printf '%s\n' "${STUB_UID:-0}" ;; '-u monitor-go-agent') echo 123 ;; '-gn monitor-go-agent') echo monitor-go-agent ;; *) exit 2 ;; esac`,
		"uname": `case "$1" in -s) echo Linux ;; -m) printf '%s\n' "${STUB_ARCH:-x86_64}" ;; *) exit 2 ;; esac`,
		"systemctl": `printf 'systemctl %s\n' "$*" >> "$STUB_LOG"
case "$1" in
    is-system-running) printf '%s\n' "${STUB_SYSTEM_STATE-running}"; exit "${STUB_SYSTEM_STATE_EXIT:-0}" ;;
    show) [ "$*" = 'show --property=SystemState --value' ]; [ "${STUB_MANAGER_REACHABLE:-1}" = 1 ]; printf '%s\n' "${STUB_SYSTEM_STATE-running}" ;;
    is-active) [ "${STUB_INACTIVE:-0}" = 0 ] ;;
    *) exit 0 ;;
esac`,
		"getent":  `exit 0`,
		"useradd": `echo 'unexpected useradd' >> "$STUB_LOG"; exit 2`,
		"chown":   `printf 'chown %s\n' "$*" >> "$STUB_LOG"`,
		"flock":   `exit 0`,
		"install": `mode=0755
directory=false
while [ "$#" -gt 0 ]; do
    case "$1" in -d) directory=true; shift ;; -m) mode=$2; shift 2 ;; -o|-g) shift 2 ;; *) break ;; esac
done
if [ "$directory" = true ]; then mkdir -p "$1"; chmod "$mode" "$1"; else cp "$1" "$2"; chmod "$mode" "$2"; fi`,
	}
	for name, body := range stubs {
		if err := os.WriteFile(filepath.Join(f.root, "stubs", name), []byte("#!/bin/sh\nset -eu\n"+body+"\n"), 0755); err != nil {
			t.Fatal(err)
		}
	}
	script := string(Script())
	for old, value := range map[string]string{
		"state_dir=/var/lib/monitor-go-agent":               "state_dir=" + testShellQuote(filepath.Join(f.root, "state-parent", "agent")),
		"binary=/usr/local/bin/monitor-go-agent":            "binary=" + testShellQuote(filepath.Join(f.root, "bin", "agent")),
		"unit=/etc/systemd/system/monitor-go-agent.service": "unit=" + testShellQuote(filepath.Join(f.root, "etc", "agent.service")),
		"lock=/run/monitor-go-agent-install.lock":           "lock=" + testShellQuote(filepath.Join(f.root, "run", "install.lock")),
	} {
		if strings.Count(script, old) != 1 {
			t.Fatalf("unsafe test path substitution: %s", old)
		}
		script = strings.Replace(script, old, value, 1)
	}
	f.script = filepath.Join(f.root, "install.sh")
	if err := os.WriteFile(f.script, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.requests.Add(1)
		if r.URL.RawQuery != "" || r.Header.Get("Authorization") != "" {
			t.Error("download included query or authorization")
		}
		switch r.URL.Path {
		case "/api/v1/agent/download/linux/amd64", "/api/v1/agent/download/linux/arm64":
			io.WriteString(w, fakeAgent)
		case "/api/v1/agent/download/linux/amd64/sha256", "/api/v1/agent/download/linux/arm64/sha256":
			if f.corrupt.Load() {
				fmt.Fprintln(w, strings.Repeat("0", 64))
			} else {
				fmt.Fprintf(w, "%x\n", sha256.Sum256([]byte(fakeAgent)))
			}
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	f.url = server.URL
	return f
}

func testShellQuote(value string) string { return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'" }

func (f *installerFixture) run(t *testing.T, token string, options []string, environment ...string) (string, error) {
	t.Helper()
	command := f.command(context.Background(), options, environment...)
	command.Stdin = strings.NewReader(token + "\n")
	output, err := command.CombinedOutput()
	if strings.Contains(string(output), token) {
		t.Fatal("installer output exposed the enrollment token")
	}
	return string(output), err
}

func (f *installerFixture) command(ctx context.Context, options []string, environment ...string) *exec.Cmd {
	command := exec.CommandContext(ctx, "sh", append([]string{f.script, "--server", f.url}, options...)...)
	command.Env = append(os.Environ(), "PATH="+filepath.Join(f.root, "stubs")+":"+os.Getenv("PATH"), "TMPDIR="+filepath.Join(f.root, "tmp"), "STUB_LOG="+f.log)
	command.Env = append(command.Env, environment...)
	return command
}

func (f *installerFixture) read(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(f.root, name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestInstallerInstallationAndIdenticalRetry(t *testing.T) {
	f := installerTestFixture(t)
	token := strings.Repeat("a", 64)
	if output, err := f.run(t, token, nil); err != nil {
		t.Fatalf("install: %v: %s", err, output)
	}
	before := f.read(t, "state-parent/agent/config.json")
	marker := f.read(t, "state-parent/agent/install-identity")
	if output, err := f.run(t, token, nil, "STUB_ARCH=aarch64"); err != nil {
		t.Fatalf("retry: %v: %s", err, output)
	}
	if string(before) != string(f.read(t, "state-parent/agent/config.json")) || string(marker) != string(f.read(t, "state-parent/agent/install-identity")) {
		t.Fatal("identical retry changed pending credentials")
	}
	commands := string(f.read(t, "commands.log"))
	if strings.Contains(commands, token) || !strings.Contains(commands, "-once -server "+f.url) || !strings.Contains(commands, "systemctl enable --now monitor-go-agent.service") || !strings.Contains(commands, "systemctl is-active --quiet monitor-go-agent.service") {
		t.Fatal("installer did not bootstrap and start without exposing token arguments")
	}
	for _, path := range []string{"state-parent/agent/config.json", "state-parent/agent/config.json.lock", "state-parent/agent/install-identity"} {
		info, err := os.Stat(filepath.Join(f.root, path))
		if err != nil || info.Mode().Perm() != 0600 {
			t.Fatalf("private file mode: %s", path)
		}
	}
}

func TestInstallerAllowsStartingAndDegradedReachableManager(t *testing.T) {
	for _, state := range []string{"starting", "degraded"} {
		t.Run(state, func(t *testing.T) {
			f := installerTestFixture(t)
			// is-system-running reports these useful states with a nonzero exit.
			output, err := f.run(t, strings.Repeat("a", 64), nil, "STUB_SYSTEM_STATE="+state, "STUB_SYSTEM_STATE_EXIT=1")
			if err != nil {
				t.Fatalf("reachable %s manager rejected: %v: %s", state, err, output)
			}
			commands := string(f.read(t, "commands.log"))
			if f.requests.Load() != 2 || !strings.Contains(commands, "systemctl show --property=SystemState --value\n") || !strings.Contains(commands, "systemctl enable --now monitor-go-agent.service\n") {
				t.Fatalf("manager was not checked before successful installation: %s", commands)
			}
		})
	}
}

func TestInstallerRejectsUnreachableManagerAndUnsafeStatesBeforeToken(t *testing.T) {
	for _, state := range []string{"starting", "offline", "unknown", "maintenance", "stopping", "initializing", ""} {
		t.Run("state="+state, func(t *testing.T) {
			f := installerTestFixture(t)
			oldFiles := map[string]string{"bin/agent": "existing-binary", "etc/agent.service": "existing-unit"}
			for path, content := range oldFiles {
				if err := os.WriteFile(filepath.Join(f.root, path), []byte(content), 0600); err != nil {
					t.Fatal(err)
				}
			}
			// An open pipe with no bytes proves host validation finishes before
			// trying to read an enrollment token, rather than only before download.
			reader, writer, err := os.Pipe()
			if err != nil {
				t.Fatal(err)
			}
			defer reader.Close()
			defer writer.Close()
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			command := f.command(ctx, nil, "STUB_SYSTEM_STATE="+state, "STUB_SYSTEM_STATE_EXIT=1", "STUB_MANAGER_REACHABLE=0")
			command.Stdin = reader
			output, err := command.CombinedOutput()
			if err == nil || ctx.Err() != nil {
				t.Fatalf("host check accepted unsafe state or tried to read token: %v: %s", err, output)
			}
			want := "Unsupported systemd state: "
			if state == "starting" {
				want = "Cannot contact the systemd system manager (state: starting)"
			}
			if !strings.Contains(string(output), want) {
				t.Fatalf("missing actionable host-state diagnostic: %s", output)
			}
			commands := string(f.read(t, "commands.log"))
			wantCommands := "systemctl is-system-running\n"
			if state == "starting" {
				wantCommands += "systemctl show --property=SystemState --value\n"
			}
			if f.requests.Load() != 0 || commands != wantCommands {
				t.Fatalf("host rejection downloaded or modified service: requests=%d commands=%s", f.requests.Load(), commands)
			}
			for path, content := range oldFiles {
				if string(f.read(t, path)) != content {
					t.Fatal("existing installation changed", path)
				}
			}
			for _, path := range []string{"state-parent/agent", "run/install.lock"} {
				if _, err := os.Stat(filepath.Join(f.root, path)); !os.IsNotExist(err) {
					t.Fatal("host rejection created installation state", path, err)
				}
			}
		})
	}
}

func TestInstallerRejectsChecksumAndDifferentInstallationBeforeStop(t *testing.T) {
	f := installerTestFixture(t)
	token := strings.Repeat("a", 64)
	if output, err := f.run(t, token, nil); err != nil {
		t.Fatalf("install: %v: %s", err, output)
	}
	before := f.read(t, "state-parent/agent/config.json")
	for _, test := range []struct {
		name, token string
		corrupt     bool
	}{
		{"checksum", token, true}, {"different token", strings.Repeat("b", 64), false},
	} {
		t.Run(test.name, func(t *testing.T) {
			f.corrupt.Store(test.corrupt)
			os.WriteFile(f.log, nil, 0600)
			if _, err := f.run(t, test.token, nil); err == nil {
				t.Fatal("unsafe installation accepted")
			}
			if strings.Contains(string(f.read(t, "commands.log")), "systemctl stop") || string(before) != string(f.read(t, "state-parent/agent/config.json")) {
				t.Fatal("rejected download/enrollment stopped service or changed credentials")
			}
		})
	}
	f.corrupt.Store(false)
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/sha256") {
			fmt.Fprintf(w, "%x\n", sha256.Sum256([]byte(fakeAgent)))
		} else {
			io.WriteString(w, fakeAgent)
		}
	}))
	defer other.Close()
	original := f.url
	f.url = other.URL
	os.WriteFile(f.log, nil, 0600)
	if _, err := f.run(t, token, nil); err == nil {
		t.Fatal("same token for a different server was accepted")
	}
	if strings.Contains(string(f.read(t, "commands.log")), "systemctl stop") {
		t.Fatal("different server stopped existing service")
	}
	f.url = original + "/"
	if output, err := f.run(t, token, nil); err != nil {
		t.Fatalf("equivalent trailing-slash origin should retry: %v: %s", err, output)
	}
}

func TestInstallerReplacementBacksUpAndRetainsFailedBootstrap(t *testing.T) {
	f := installerTestFixture(t)
	if output, err := f.run(t, strings.Repeat("a", 64), nil); err != nil {
		t.Fatalf("install: %v: %s", err, output)
	}
	old := f.read(t, "state-parent/agent/config.json")
	if err := os.WriteFile(f.log, nil, 0600); err != nil {
		t.Fatal(err)
	}
	token := strings.Repeat("b", 64)
	if _, err := f.run(t, token, []string{"--replace"}, "STUB_BOOTSTRAP_FAIL=1"); err == nil {
		t.Fatal("failed first report claimed success")
	}
	pending := f.read(t, "state-parent/agent/config.json")
	if string(pending) == string(old) || !strings.Contains(string(pending), token) {
		t.Fatal("replacement did not preserve new pending credentials")
	}
	backups, err := filepath.Glob(filepath.Join(f.root, "state-parent", "agent", "backups", "credentials.*", "config.json"))
	if err != nil || len(backups) != 1 {
		t.Fatalf("private backup: %v %v", backups, err)
	}
	backup, err := os.ReadFile(backups[0])
	if err != nil || string(backup) != string(old) {
		t.Fatal("old credentials were not backed up")
	}
	info, err := os.Stat(backups[0])
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("credential backup is not private")
	}
	if strings.Contains(string(f.read(t, "commands.log")), "systemctl enable") {
		t.Fatal("service enabled before a successful first report")
	}
	if output, err := f.run(t, token, nil); err != nil {
		t.Fatalf("retry pending replacement: %v: %s", err, output)
	}
	if string(pending) != string(f.read(t, "state-parent/agent/config.json")) {
		t.Fatal("retry rotated pending credentials")
	}
}

func TestInstallerRequiresRootSystemdAndTrustedTLS(t *testing.T) {
	for _, environment := range []string{"STUB_UID=123", "STUB_SYSTEM_STATE=offline", "STUB_ARCH=riscv64"} {
		t.Run(environment, func(t *testing.T) {
			f := installerTestFixture(t)
			if _, err := f.run(t, strings.Repeat("a", 64), nil, environment); err == nil {
				t.Fatal("unsupported host accepted")
			}
			if f.requests.Load() != 0 {
				t.Fatal("download started before host requirements passed")
			}
			if _, err := os.Stat(filepath.Join(f.root, "state-parent", "agent")); !os.IsNotExist(err) {
				t.Fatal("unsupported host was mutated")
			}
		})
	}
	f := installerTestFixture(t)
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { t.Error("untrusted TLS reached handler") }))
	defer server.Close()
	f.url = server.URL
	curlHome := filepath.Join(f.root, "curl-home")
	if err := os.Mkdir(curlHome, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(curlHome, ".curlrc"), []byte("insecure\nlocation\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := f.run(t, strings.Repeat("a", 64), nil, "CURL_HOME="+curlHome); err == nil {
		t.Fatal("untrusted certificate accepted")
	}
	if strings.Contains(string(f.read(t, "commands.log")), "systemctl stop") {
		t.Fatal("TLS failure stopped service")
	}
}

func TestInstallerRejectsInactiveServiceAndKeepsUnitInSync(t *testing.T) {
	f := installerTestFixture(t)
	if _, err := f.run(t, strings.Repeat("a", 64), nil, "STUB_INACTIVE=1"); err == nil {
		t.Fatal("inactive service claimed success")
	}
	unit, err := os.ReadFile("../../deploy/monitor-go-agent.service")
	if err != nil {
		t.Fatal(err)
	}
	if string(unit) != string(f.read(t, "etc/agent.service")) {
		t.Fatal("installer service differs from deployment template")
	}
	copy := Script()
	copy[0] = 'X'
	if Script()[0] != '#' {
		t.Fatal("caller changed embedded installer")
	}
}
