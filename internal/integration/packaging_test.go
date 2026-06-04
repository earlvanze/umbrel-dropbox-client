package integration

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestPackagingSmokeBuildsBinariesAndServiceReferencesDaemon(t *testing.T) {
	repo := repoRoot(t)
	out := t.TempDir()
	for _, target := range []struct{ name, pkg string }{
		{"umbrel-dropbox-client", "./cmd/umbrel-dropbox-client"},
		{"umbrel-dropbox-clientd", "./cmd/umbrel-dropbox-clientd"},
	} {
		cmd := exec.Command("go", "build", "-o", filepath.Join(out, target.name), target.pkg)
		cmd.Dir = repo
		if b, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("go build %s: %v\n%s", target.pkg, err, b)
		}
	}
	service := readFile(t, filepath.Join(repo, "packaging/systemd/umbrel-dropbox-client.service"))
	if !strings.Contains(service, "ExecStart=/usr/bin/umbrel-dropbox-clientd --config %h/.config/umbrel-dropbox-client/config.json") {
		t.Fatalf("service ExecStart missing daemon config path:\n%s", service)
	}
	goreleaser := readFile(t, filepath.Join(repo, ".goreleaser.yaml"))
	for _, want := range []string{"binary: umbrel-dropbox-client", "binary: umbrel-dropbox-clientd", "dst: /usr/lib/systemd/user/umbrel-dropbox-client.service"} {
		if !strings.Contains(goreleaser, want) {
			t.Fatalf("goreleaser missing %q", want)
		}
	}
}

func TestInstallScriptInitializesDaemonConfig(t *testing.T) {
	repo := repoRoot(t)
	script := readFile(t, filepath.Join(repo, "scripts/install-linux.sh"))
	for _, want := range []string{
		"/usr/local/bin/umbrel-dropbox-client",
		"/usr/local/bin/umbrel-dropbox-clientd",
		"mkdir -p ~/.config/umbrel-dropbox-client",
		"umbrel-dropbox-client init --root \"$HOME/Dropbox\" --config \"$HOME/.config/umbrel-dropbox-client/config.json\"",
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("install script missing %q:\n%s", want, script)
		}
	}
}

func TestUmbrelAppPackagingStartsDryRunDaemon(t *testing.T) {
	repo := repoRoot(t)
	dockerfile := readFile(t, filepath.Join(repo, "Dockerfile"))
	for _, want := range []string{
		"umbrel-dropbox-client-umbrel-entrypoint",
		"ENTRYPOINT [\"/usr/bin/umbrel-dropbox-client-umbrel-entrypoint\"]",
		"EXPOSE 8477",
	} {
		if !strings.Contains(dockerfile, want) {
			t.Fatalf("Dockerfile missing %q:\n%s", want, dockerfile)
		}
	}
	entrypoint := readFile(t, filepath.Join(repo, "packaging/docker/umbrel-entrypoint.sh"))
	for _, want := range []string{
		"UDC_ROOT:-/dropbox",
		"UDC_REMOTE_PATH:-/",
		"UDC_LIVE_SCOPE:-/",
		"live mode requires UDC_ALLOW_LIVE=true",
		"umbrel-dropbox-client init --root \"$ROOT\" --db \"$DB\"",
		"exec umbrel-dropbox-clientd --config \"$CONFIG\"",
		"\"dry_run\": $DRY_RUN",
		"\"allow_live\": $ALLOW_LIVE",
		"\"health_addr\": \"$HEALTH_ADDR\"",
	} {
		if !strings.Contains(entrypoint, want) {
			t.Fatalf("entrypoint missing %q:\n%s", want, entrypoint)
		}
	}
	compose := readFile(t, filepath.Join(repo, "umbrel-app/docker-compose.yml"))
	for _, want := range []string{
		"app_proxy:",
		"APP_HOST: umbrel-dropbox-client_server_1",
		"APP_PORT: 8477",
		"/home/umbrel/Dropbox:/dropbox",
		"UDC_DRY_RUN=true",
		"UDC_ROOT=/dropbox",
		"UDC_REMOTE_DELTA=auto",
	} {
		if !strings.Contains(compose, want) {
			t.Fatalf("docker-compose missing %q:\n%s", want, compose)
		}
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Clean(filepath.Join(wd, "../.."))
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
