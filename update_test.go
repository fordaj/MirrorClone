package main

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// buildArchive returns a zip archive with the given files.
func buildArchive(t *testing.T, files map[string]string) []byte {
	t.Helper()

	var buf bytes.Buffer
	writer := zip.NewWriter(&buf)
	for name, content := range files {
		file, err := writer.Create(name)
		if err != nil {
			t.Fatalf("creating %s in the archive: %v", name, err)
		}
		if _, err := file.Write([]byte(content)); err != nil {
			t.Fatalf("writing %s in the archive: %v", name, err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("closing the archive: %v", err)
	}
	return buf.Bytes()
}

func TestCompareVersions(t *testing.T) {
	cases := []struct {
		a    string
		b    string
		want int
	}{
		{a: "1.0.0", b: "1.0.0", want: 0},
		{a: "v1.0.0", b: "1.0.0", want: 0},
		{a: "1.0.1", b: "1.0.0", want: 1},
		{a: "1.0.0", b: "1.0.1", want: -1},
		{a: "0.0.1.0.10", b: "0.0.1.0.9", want: 1},
		{a: "v0.0.1.0.9", b: "0.0.1.0.10", want: -1},
		{a: "1.0", b: "1.0.0", want: 0},
		{a: "2.0.0", b: "1.9.9", want: 1},
		{a: "dev", b: "0.0.1", want: -1},
	}

	for _, tc := range cases {
		got := compareVersions(tc.a, tc.b)
		if got != tc.want {
			t.Errorf("compareVersions(%q, %q) = %d, want %d", tc.a, tc.b, got, tc.want)
		}
	}
}

func TestReleaseVersion(t *testing.T) {
	if got := releaseVersion("v0.0.1.0.10"); got != "0.0.1.0.10" {
		t.Errorf("releaseVersion(%q) = %q, want %q", "v0.0.1.0.10", got, "0.0.1.0.10")
	}
	if got := releaseVersion("0.0.1.0.10"); got != "0.0.1.0.10" {
		t.Errorf("releaseVersion(%q) = %q, want %q", "0.0.1.0.10", got, "0.0.1.0.10")
	}
}

func TestPlatformName(t *testing.T) {
	cases := []struct {
		goos   string
		goarch string
		want   string
	}{
		{goos: "linux", goarch: "amd64", want: "linux-x86_64"},
		{goos: "linux", goarch: "arm64", want: "linux-arm64"},
		{goos: "darwin", goarch: "amd64", want: "macos-x86_64"},
		{goos: "darwin", goarch: "arm64", want: "macos-arm64"},
		{goos: "windows", goarch: "amd64", want: "windows-x86_64"},
		{goos: "windows", goarch: "arm64", want: "windows-arm64"},
		{goos: "linux", goarch: "386", want: ""},
		{goos: "plan9", goarch: "amd64", want: ""},
	}

	for _, tc := range cases {
		got := platformName(tc.goos, tc.goarch)
		if got != tc.want {
			t.Errorf("platformName(%q, %q) = %q, want %q", tc.goos, tc.goarch, got, tc.want)
		}
	}
}

func TestSelectLatestRelease(t *testing.T) {
	releases := []release{
		{TagName: "v0.0.1.0.9"},
		{TagName: "v0.0.1.0.11", Prerelease: true},
		{TagName: "v0.0.1.0.12", Draft: true},
		{TagName: "v0.0.1.0.10"},
	}

	latest, ok := selectLatestRelease(releases)
	if !ok {
		t.Fatal("selectLatestRelease did not find a release")
	}
	if latest.TagName != "v0.0.1.0.10" {
		t.Errorf("selectLatestRelease returned %q, want %q", latest.TagName, "v0.0.1.0.10")
	}
}

func TestSelectLatestReleaseWithoutRelease(t *testing.T) {
	releases := []release{
		{TagName: "v0.0.1.0.11", Prerelease: true},
		{TagName: "v0.0.1.0.12", Draft: true},
	}

	if _, ok := selectLatestRelease(releases); ok {
		t.Error("selectLatestRelease found a release that is not stable")
	}
}

func TestSelectArchive(t *testing.T) {
	assets := []asset{
		{Name: "mirror-clone_1.0.0_linux-x86_64.zip"},
		{Name: "mirror-clone_1.0.0_windows-x86_64.zip"},
		{Name: "checksums.txt"},
	}

	got, ok := selectArchive(assets, "1.0.0", "windows-x86_64")
	if !ok {
		t.Fatal("selectArchive did not find the archive")
	}
	if got.Name != "mirror-clone_1.0.0_windows-x86_64.zip" {
		t.Errorf("selectArchive returned %q, want %q", got.Name, "mirror-clone_1.0.0_windows-x86_64.zip")
	}

	if _, ok := selectArchive(assets, "1.0.0", "macos-arm64"); ok {
		t.Error("selectArchive found an archive for a platform that is not in the release")
	}
}

func TestSelectArchiveOfAnotherVersion(t *testing.T) {
	assets := []asset{
		{Name: "mirror-clone_1.2.3_linux-x86_64.zip"},
	}

	got, ok := selectArchive(assets, "1.0.0", "linux-x86_64")
	if !ok {
		t.Fatal("selectArchive did not find the archive")
	}
	if got.Name != "mirror-clone_1.2.3_linux-x86_64.zip" {
		t.Errorf("selectArchive returned %q, want %q", got.Name, "mirror-clone_1.2.3_linux-x86_64.zip")
	}
}

func TestSelectChecksums(t *testing.T) {
	assets := []asset{
		{Name: "mirror-clone_1.0.0_linux-x86_64.zip"},
		{Name: "checksums.txt"},
	}

	got, ok := selectChecksums(assets)
	if !ok {
		t.Fatal("selectChecksums did not find the checksums")
	}
	if got.Name != "checksums.txt" {
		t.Errorf("selectChecksums returned %q, want %q", got.Name, "checksums.txt")
	}

	if _, ok := selectChecksums([]asset{{Name: "mirror-clone_1.0.0_linux-x86_64.zip"}}); ok {
		t.Error("selectChecksums found checksums that are not in the release")
	}
}

func TestParseChecksums(t *testing.T) {
	data := []byte("aabbcc  mirror-clone_1.0.0_linux-x86_64.zip\n" +
		"ddeeff *mirror-clone_1.0.0_windows-x86_64.zip\n" +
		"\n" +
		"not a checksum line\n")

	sums := parseChecksums(data)
	if len(sums) != 2 {
		t.Fatalf("parseChecksums returned %d entries, want 2", len(sums))
	}
	if sums["mirror-clone_1.0.0_linux-x86_64.zip"] != "aabbcc" {
		t.Errorf("the checksum of the archive is %q, want %q", sums["mirror-clone_1.0.0_linux-x86_64.zip"], "aabbcc")
	}
	if sums["mirror-clone_1.0.0_windows-x86_64.zip"] != "ddeeff" {
		t.Errorf("the checksum of the archive is %q, want %q", sums["mirror-clone_1.0.0_windows-x86_64.zip"], "ddeeff")
	}
}

func TestVerifyChecksum(t *testing.T) {
	dir := t.TempDir()
	archive := writeTestExecutable(t, filepath.Join(dir, "archive.zip"), "the archive")
	sum := sha256.Sum256([]byte("the archive"))

	sums := writeTestExecutable(t, filepath.Join(dir, "checksums.txt"),
		fmt.Sprintf("%x  archive.zip\n", sum))

	if err := verifyChecksum(archive, sums, "archive.zip"); err != nil {
		t.Fatalf("verifyChecksum returned error: %v", err)
	}
}

func TestVerifyChecksumWithAWrongChecksum(t *testing.T) {
	dir := t.TempDir()
	archive := writeTestExecutable(t, filepath.Join(dir, "archive.zip"), "the archive")
	sums := writeTestExecutable(t, filepath.Join(dir, "checksums.txt"), "0000  archive.zip\n")

	if err := verifyChecksum(archive, sums, "archive.zip"); err == nil {
		t.Error("verifyChecksum did not return an error")
	}
}

func TestVerifyChecksumWithoutTheFileInTheChecksums(t *testing.T) {
	dir := t.TempDir()
	archive := writeTestExecutable(t, filepath.Join(dir, "archive.zip"), "the archive")
	sums := writeTestExecutable(t, filepath.Join(dir, "checksums.txt"), "0000  another.zip\n")

	if err := verifyChecksum(archive, sums, "archive.zip"); err == nil {
		t.Error("verifyChecksum did not return an error")
	}
}

func TestExtractExecutable(t *testing.T) {
	dir := t.TempDir()
	content := buildArchive(t, map[string]string{
		"mirror-clone": "the new binary",
		"LICENSE":      "the license",
	})
	archive := filepath.Join(dir, "archive.zip")
	if err := os.WriteFile(archive, content, 0o644); err != nil {
		t.Fatalf("writing the archive: %v", err)
	}

	dest := filepath.Join(dir, "mirror-clone")
	if err := extractExecutable(archive, dest, "mirror-clone"); err != nil {
		t.Fatalf("extractExecutable returned error: %v", err)
	}

	data, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("reading %s: %v", dest, err)
	}
	if string(data) != "the new binary" {
		t.Errorf("the extracted file contains %q, want %q", data, "the new binary")
	}
}

func TestExtractExecutableWithoutTheFileInTheArchive(t *testing.T) {
	dir := t.TempDir()
	content := buildArchive(t, map[string]string{"LICENSE": "the license"})
	archive := filepath.Join(dir, "archive.zip")
	if err := os.WriteFile(archive, content, 0o644); err != nil {
		t.Fatalf("writing the archive: %v", err)
	}

	if err := extractExecutable(archive, filepath.Join(dir, "mirror-clone"), "mirror-clone"); err == nil {
		t.Error("extractExecutable did not return an error")
	}
}

// releaseServer serves a release with one archive and the checksums of it.
type releaseServer struct {
	*httptest.Server
	archiveName string
	archive     []byte
	checksums   string
}

// newReleaseServer starts a server with a single stable release.
func newReleaseServer(t *testing.T, tag string, binary string) *releaseServer {
	t.Helper()

	platform := platformName(runtime.GOOS, runtime.GOARCH)
	archiveName := releaseAssetName(releaseVersion(tag), platform)
	archive := buildArchive(t, map[string]string{
		"mirror-clone": binary,
		"LICENSE":      "the license",
	})
	sum := sha256.Sum256(archive)

	server := &releaseServer{
		archiveName: archiveName,
		archive:     archive,
		checksums:   fmt.Sprintf("%x  %s\n", sum, archiveName),
	}

	var httpServer *httptest.Server
	httpServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/releases":
			fmt.Fprintf(w, `[
				{
					"tag_name": %q,
					"published_at": "2024-01-01T00:00:00Z",
					"assets": [
						{"name": %q, "browser_download_url": "%s/download/archive"},
						{"name": "checksums.txt", "browser_download_url": "%s/download/checksums"}
					]
				}
			]`, tag, archiveName, httpServer.URL, httpServer.URL)
		case "/download/archive":
			_, _ = w.Write(server.archive)
		case "/download/checksums":
			_, _ = w.Write([]byte(server.checksums))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	server.Server = httpServer
	t.Cleanup(server.Close)

	return server
}

// newTestUpdater returns an updater that reads from the server.
func newTestUpdater(server *releaseServer, dir, name, installed string) *updater {
	log := NewLogger(io.Discard, io.Discard, LevelError)
	return &updater{
		log: log,
		releases: &releaseClient{
			baseURL: server.URL + "/releases",
			client:  server.Client(),
			log:     log,
		},
		client:    server.Client(),
		dir:       dir,
		name:      name,
		installed: installed,
	}
}

func TestUpdaterInstallsTheLatestRelease(t *testing.T) {
	server := newReleaseServer(t, "v9.9.9", "the new binary")
	dir := t.TempDir()
	target := writeTestExecutable(t, filepath.Join(dir, "mirror-clone"), "the old binary")

	up := newTestUpdater(server, dir, "mirror-clone", "1.0.0")
	if err := up.Run(context.Background()); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("reading %s: %v", target, err)
	}
	if string(data) != "the new binary" {
		t.Errorf("%s contains %q, want %q", target, data, "the new binary")
	}

	info, err := os.Stat(target)
	if err != nil {
		t.Fatalf("reading the mode of %s: %v", target, err)
	}
	if info.Mode().Perm() != 0o755 {
		t.Errorf("the mode of %s is %v, want %v", target, info.Mode().Perm(), os.FileMode(0o755))
	}
}

func TestUpdaterKeepsTheInstalledBinaryWhenTheChecksumIsWrong(t *testing.T) {
	server := newReleaseServer(t, "v9.9.9", "the new binary")
	server.checksums = "0000  " + server.archiveName + "\n"
	dir := t.TempDir()
	target := writeTestExecutable(t, filepath.Join(dir, "mirror-clone"), "the old binary")

	up := newTestUpdater(server, dir, "mirror-clone", "1.0.0")
	if err := up.Run(context.Background()); err == nil {
		t.Fatal("Run did not return an error")
	}

	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("reading %s: %v", target, err)
	}
	if string(data) != "the old binary" {
		t.Errorf("%s contains %q, want %q", target, data, "the old binary")
	}
}

func TestUpdaterKeepsTheInstalledBinaryWhenTheDownloadFails(t *testing.T) {
	server := newReleaseServer(t, "v9.9.9", "the new binary")
	dir := t.TempDir()
	target := writeTestExecutable(t, filepath.Join(dir, "mirror-clone"), "the old binary")

	up := newTestUpdater(server, dir, "mirror-clone", "1.0.0")
	up.client = &http.Client{Transport: failingTransport{}}

	if err := up.Run(context.Background()); err == nil {
		t.Fatal("Run did not return an error")
	}

	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("reading %s: %v", target, err)
	}
	if string(data) != "the old binary" {
		t.Errorf("%s contains %q, want %q", target, data, "the old binary")
	}
}

// failingTransport fails every request.
type failingTransport struct{}

func (failingTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, fmt.Errorf("the connection failed")
}

func TestUpdaterDoesNotDowngrade(t *testing.T) {
	server := newReleaseServer(t, "v1.0.0", "the new binary")
	dir := t.TempDir()
	target := writeTestExecutable(t, filepath.Join(dir, "mirror-clone"), "the old binary")

	up := newTestUpdater(server, dir, "mirror-clone", "2.0.0")
	if err := up.Run(context.Background()); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("reading %s: %v", target, err)
	}
	if string(data) != "the old binary" {
		t.Errorf("%s contains %q, want %q", target, data, "the old binary")
	}
}

func TestUpdaterDoesNothingWhenTheVersionIsTheSame(t *testing.T) {
	server := newReleaseServer(t, "v2.0.0", "the new binary")
	dir := t.TempDir()
	target := writeTestExecutable(t, filepath.Join(dir, "mirror-clone"), "the old binary")

	up := newTestUpdater(server, dir, "mirror-clone", "2.0.0")
	if err := up.Run(context.Background()); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("reading %s: %v", target, err)
	}
	if string(data) != "the old binary" {
		t.Errorf("%s contains %q, want %q", target, data, "the old binary")
	}
}

func TestUpdaterWithoutARelease(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("[]"))
	}))
	defer server.Close()

	dir := t.TempDir()
	log := NewLogger(io.Discard, io.Discard, LevelError)
	up := &updater{
		log: log,
		releases: &releaseClient{
			baseURL: server.URL,
			client:  server.Client(),
			log:     log,
		},
		client: server.Client(),
		dir:    dir,
		name:   "mirror-clone",
	}

	if err := up.Run(context.Background()); err == nil {
		t.Error("Run did not return an error")
	}
}

func TestUpdaterWithoutAnArchiveForThePlatform(t *testing.T) {
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[{"tag_name":"v9.9.9","assets":[{"name":"checksums.txt","browser_download_url":"` + server.URL + `/download/checksums"}]}]`))
	}))
	defer server.Close()

	dir := t.TempDir()
	log := NewLogger(io.Discard, io.Discard, LevelError)
	up := &updater{
		log: log,
		releases: &releaseClient{
			baseURL: server.URL,
			client:  server.Client(),
			log:     log,
		},
		client: server.Client(),
		dir:    dir,
		name:   "mirror-clone",
	}

	if err := up.Run(context.Background()); err == nil {
		t.Error("Run did not return an error")
	}
}
