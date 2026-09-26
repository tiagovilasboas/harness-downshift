// Copyright (c) 2026 Tiago de Carvalho Vilas Boas.
// SPDX-License-Identifier: BUSL-1.1
// Commercial use requires a licence — see LICENSE for terms.

package installer_test

import (
	"archive/tar"
	"compress/gzip"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// fakeCurl stands in for curl so install.sh can be exercised offline.
// FAKE_LATEST / FAKE_LIST hold the tag served by /releases/latest and
// /releases?per_page=1; an empty value simulates an HTTP error (curl -f → 22).
// Downloads copy FAKE_TARBALL to the -o path. Every URL is appended to FAKE_LOG.
const fakeCurl = `#!/bin/sh
url=""; out=""
while [ $# -gt 0 ]; do
  case "$1" in
    -o) out="$2"; shift ;;
    http*) url="$1" ;;
  esac
  shift
done
echo "$url" >> "$FAKE_LOG"
case "$url" in
  */releases/latest)
    [ -n "$FAKE_LATEST" ] || exit 22
    printf '{\n  "tag_name": "%s",\n  "prerelease": false\n}\n' "$FAKE_LATEST" ;;
  *"/releases?per_page=1")
    [ -n "$FAKE_LIST" ] || exit 22
    printf '[\n  {\n    "tag_name": "%s",\n    "prerelease": true\n  }\n]\n' "$FAKE_LIST" ;;
  */releases/download/*)
    cp "$FAKE_TARBALL" "$out" ;;
  *) exit 22 ;;
esac
`

type installResult struct {
	code   int
	output string
	urls   []string
	binDir string
}

func runInstall(t *testing.T, latest, list string, args ...string) installResult {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("install.sh targets macOS/Linux")
	}
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh not available")
	}
	tmp := t.TempDir()
	fakeBin := filepath.Join(tmp, "fakebin")
	if err := os.MkdirAll(fakeBin, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(fakeBin, "curl"), []byte(fakeCurl), 0o755); err != nil {
		t.Fatal(err)
	}
	tarball := filepath.Join(tmp, "release.tar.gz")
	writeTarball(t, tarball)
	logFile := filepath.Join(tmp, "curl.log")
	binDir := filepath.Join(tmp, "bin")

	cmd := exec.Command("sh", append([]string{"install.sh"}, args...)...)
	cmd.Env = []string{
		"PATH=" + fakeBin + string(os.PathListSeparator) + os.Getenv("PATH"),
		"HOME=" + tmp,
		"DOWNSHIFT_INSTALL_DIR=" + binDir,
		"FAKE_LATEST=" + latest,
		"FAKE_LIST=" + list,
		"FAKE_TARBALL=" + tarball,
		"FAKE_LOG=" + logFile,
	}
	out, err := cmd.CombinedOutput()
	code := 0
	if err != nil {
		exitErr, ok := err.(*exec.ExitError)
		if !ok {
			t.Fatalf("running install.sh: %v", err)
		}
		code = exitErr.ExitCode()
	}
	logged, _ := os.ReadFile(logFile)
	return installResult{
		code:   code,
		output: string(out),
		urls:   strings.Fields(string(logged)),
		binDir: binDir,
	}
}

func writeTarball(t *testing.T, path string) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	body := []byte("#!/bin/sh\necho fake-downshift\n")
	if err := tw.WriteHeader(&tar.Header{Name: "downshift", Mode: 0o755, Size: int64(len(body))}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(body); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
}

func downloadURL(t *testing.T, r installResult) string {
	t.Helper()
	for _, u := range r.urls {
		if strings.Contains(u, "/releases/download/") {
			return u
		}
	}
	t.Fatalf("no download attempted; curl calls: %v\noutput:\n%s", r.urls, r.output)
	return ""
}

func assertInstalled(t *testing.T, r installResult) {
	t.Helper()
	if r.code != 0 {
		t.Fatalf("install.sh exit %d, want 0\noutput:\n%s", r.code, r.output)
	}
	if _, err := os.Stat(filepath.Join(r.binDir, "downshift")); err != nil {
		t.Fatalf("binary not installed in %s: %v", r.binDir, err)
	}
}

// Regression: while only prereleases exist, /releases/latest returns 404 and
// the one-line installer exited 1 for every user.
func TestInstallFallsBackToPrereleaseWhenNoStableRelease(t *testing.T) {
	r := runInstall(t, "", "v0.1.0-beta.1")
	assertInstalled(t, r)
	u := downloadURL(t, r)
	if !strings.Contains(u, "/releases/download/v0.1.0-beta.1/downshift_0.1.0-beta.1_") {
		t.Errorf("download URL = %s, want the v0.1.0-beta.1 archive", u)
	}
}

func TestInstallPrefersLatestStableRelease(t *testing.T) {
	r := runInstall(t, "v1.0.0", "v1.1.0-beta.1")
	assertInstalled(t, r)
	if u := downloadURL(t, r); !strings.Contains(u, "/releases/download/v1.0.0/") {
		t.Errorf("download URL = %s, want v1.0.0", u)
	}
	for _, u := range r.urls {
		if strings.Contains(u, "per_page") {
			t.Errorf("release list queried although /releases/latest answered: %v", r.urls)
		}
	}
}

func TestInstallExplicitVersionSkipsAPI(t *testing.T) {
	r := runInstall(t, "", "", "v0.1.0-beta.1")
	assertInstalled(t, r)
	for _, u := range r.urls {
		if strings.Contains(u, "api.github.com") {
			t.Errorf("explicit version should not query the API: %v", r.urls)
		}
	}
}

func TestInstallFailsClearlyWhenAPIUnavailable(t *testing.T) {
	r := runInstall(t, "", "")
	if r.code != 1 {
		t.Fatalf("install.sh exit %d, want 1\noutput:\n%s", r.code, r.output)
	}
	if !strings.Contains(r.output, "could not determine latest version") {
		t.Errorf("missing actionable error, got:\n%s", r.output)
	}
}

func TestInstallScriptShellcheck(t *testing.T) {
	path, err := exec.LookPath("shellcheck")
	if err != nil {
		t.Skip("shellcheck not installed")
	}
	if out, err := exec.Command(path, "install.sh").CombinedOutput(); err != nil {
		t.Fatalf("shellcheck install.sh: %v\n%s", err, out)
	}
}
