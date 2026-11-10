package main

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// updater replaces the installed executable with the latest stable release
// of the repository.
type updater struct {
	// log logs the progress of the update.
	log *Logger
	// releases reads the releases of the repository.
	releases *releaseClient
	// client downloads the assets of a release.
	client *http.Client
	// dir is the directory the executable is installed in.
	dir string
	// name is the file name of the executable.
	name string
	// installed is the version of the running executable.
	installed string
}

// newUpdater returns an updater for the installation in dir.
func newUpdater(log *Logger, dir, name string) *updater {
	return &updater{
		log:       log,
		releases:  newReleaseClient(log),
		client:    &http.Client{Timeout: 30 * time.Minute},
		dir:       dir,
		name:      name,
		installed: version,
	}
}

// Run downloads the latest stable release and replaces the installed
// executable with it.
//
// The installed executable is kept when the download, the verification or
// the replacement fails, so a failed update never breaks the installation.
func (u *updater) Run(ctx context.Context) error {
	latest, err := u.releases.latestRelease(ctx)
	if err != nil {
		return err
	}

	available := releaseVersion(latest.TagName)
	switch compareVersions(available, u.installed) {
	case 0:
		u.log.Infof("%s %s is up to date (latest release: %s)", appName, u.installed, available)
		return nil
	case -1:
		u.log.Warnf("%s %s is newer than the latest release %s, nothing to do", appName, u.installed, available)
		return nil
	}
	u.log.Infof("Found the release %s", latest.TagName)

	platform := platformName(runtime.GOOS, runtime.GOARCH)
	if platform == "" {
		return fmt.Errorf("unsupported platform: %s/%s", runtime.GOOS, runtime.GOARCH)
	}

	archive, ok := selectArchive(latest.Assets, available, platform)
	if !ok {
		return fmt.Errorf("the release %s has no archive for %s", latest.TagName, platform)
	}
	checksums, ok := selectChecksums(latest.Assets)
	if !ok {
		return fmt.Errorf("the release %s has no %s", latest.TagName, checksumsName)
	}

	// The work directory is inside the installation directory, so the new
	// executable is moved into place instead of being copied across the
	// devices.
	work, err := os.MkdirTemp(u.dir, "."+appName+"-update-")
	if err != nil {
		return fmt.Errorf("creating the temporary directory: %w", err)
	}
	defer os.RemoveAll(work)

	archivePath := filepath.Join(work, archive.Name)
	if err := u.download(ctx, archive.DownloadURL, archivePath); err != nil {
		return fmt.Errorf("downloading %s: %w", archive.Name, err)
	}
	sumsPath := filepath.Join(work, checksums.Name)
	if err := u.download(ctx, checksums.DownloadURL, sumsPath); err != nil {
		return fmt.Errorf("downloading %s: %w", checksums.Name, err)
	}

	if err := verifyChecksum(archivePath, sumsPath, archive.Name); err != nil {
		return fmt.Errorf("verifying %s: %w", archive.Name, err)
	}
	u.log.Infof("Verified the checksum of %s", archive.Name)

	binary := filepath.Join(work, u.name)
	if err := extractExecutable(archivePath, binary, u.name); err != nil {
		return fmt.Errorf("extracting %s: %w", u.name, err)
	}
	if err := os.Chmod(binary, 0o755); err != nil {
		return fmt.Errorf("setting the permissions of %s: %w", binary, err)
	}

	target := filepath.Join(u.dir, u.name)
	if err := replaceFile(target, binary); err != nil {
		return fmt.Errorf("replacing %s: %w", target, err)
	}

	u.log.Infof("Updated %s from %s to %s", target, u.installed, available)
	return nil
}

// download downloads the file at url into dest.
func (u *updater) download(ctx context.Context, url, dest string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("building the request: %w", err)
	}
	req.Header.Set("User-Agent", appName)

	u.log.Debugf("GET %s", url)

	resp, err := u.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status %s", resp.Status)
	}

	out, err := os.Create(dest)
	if err != nil {
		return fmt.Errorf("creating %s: %w", dest, err)
	}
	if _, err := io.Copy(out, resp.Body); err != nil {
		out.Close()
		return fmt.Errorf("writing %s: %w", dest, err)
	}
	return out.Close()
}

// verifyChecksum checks the file at path against the checksum that the
// checksums file holds for the file with the given name.
func verifyChecksum(path, sumsPath, name string) error {
	data, err := os.ReadFile(sumsPath)
	if err != nil {
		return fmt.Errorf("reading %s: %w", sumsPath, err)
	}

	sums := parseChecksums(data)
	want, ok := sums[name]
	if !ok {
		return fmt.Errorf("the checksums do not contain %s", name)
	}

	got, err := sha256File(path)
	if err != nil {
		return err
	}
	if !strings.EqualFold(got, want) {
		return fmt.Errorf("the checksum of %s does not match: expected %s, got %s", name, want, got)
	}
	return nil
}

// parseChecksums parses the content of a checksums file into a map of file
// names and their checksums. The format is the one of sha256sum.
func parseChecksums(data []byte) map[string]string {
	sums := map[string]string{}

	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 {
			continue
		}
		// The name of the file is prefixed with a star when the file is
		// opened in binary mode.
		name := strings.TrimPrefix(fields[1], "*")
		sums[name] = strings.ToLower(fields[0])
	}
	return sums
}

// sha256File returns the SHA256 checksum of the file at path.
func sha256File(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("opening %s: %w", path, err)
	}
	defer file.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", fmt.Errorf("reading %s: %w", path, err)
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

// extractExecutable extracts the executable with the given name from the zip
// archive at archive into dest.
func extractExecutable(archive, dest, name string) error {
	reader, err := zip.OpenReader(archive)
	if err != nil {
		return fmt.Errorf("opening %s: %w", archive, err)
	}
	defer reader.Close()

	for _, file := range reader.File {
		if file.FileInfo().IsDir() {
			continue
		}
		if filepath.Base(file.Name) != name {
			continue
		}
		return writeArchiveEntry(file, dest)
	}
	return fmt.Errorf("the archive %s does not contain %s", archive, name)
}

// writeArchiveEntry writes the content of a file of an archive to dest.
func writeArchiveEntry(file *zip.File, dest string) error {
	src, err := file.Open()
	if err != nil {
		return fmt.Errorf("opening %s: %w", file.Name, err)
	}
	defer src.Close()

	out, err := os.OpenFile(dest, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o755)
	if err != nil {
		return fmt.Errorf("creating %s: %w", dest, err)
	}
	if _, err := io.Copy(out, src); err != nil {
		out.Close()
		return fmt.Errorf("writing %s: %w", dest, err)
	}
	return out.Close()
}

// runUpdate updates the installed executable to the latest stable release.
func runUpdate(ctx context.Context, log *Logger) int {
	dir, err := defaultInstallDir()
	if err != nil {
		log.Errorf("%v", err)
		return 1
	}

	target := installPath(runtime.GOOS, dir)
	if _, err := os.Stat(target); err != nil {
		log.Errorf("%s is not installed in %s, run '%s install' first", appName, dir, appName)
		return 1
	}

	if err := newUpdater(log, dir, executableName(runtime.GOOS)).Run(ctx); err != nil {
		log.Errorf("%v", err)
		return 1
	}
	return 0
}
