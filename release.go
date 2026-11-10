package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// releasesURL is the endpoint of the releases of the repository.
const releasesURL = "https://api.github.com/repos/fordaj/MirrorClone/releases"

// checksumsName is the name of the asset with the checksums of the release
// archives.
const checksumsName = "checksums.txt"

// release is a release of the GitHub repository.
type release struct {
	// TagName is the tag of the release, for example "v0.0.1.0.10".
	TagName string `json:"tag_name"`
	// Draft is set for a release that was not published yet.
	Draft bool `json:"draft"`
	// Prerelease is set for a release that is not stable.
	Prerelease bool `json:"prerelease"`
	// PublishedAt is the time the release was published.
	PublishedAt string `json:"published_at"`
	// Assets are the files that are attached to the release.
	Assets []asset `json:"assets"`
}

// asset is a file that is attached to a release.
type asset struct {
	// Name is the file name of the asset.
	Name string `json:"name"`
	// DownloadURL is the location the asset is downloaded from.
	DownloadURL string `json:"browser_download_url"`
	// Size is the size of the asset in bytes.
	Size int64 `json:"size"`
}

// releaseClient reads the releases of the repository from the GitHub API.
type releaseClient struct {
	// baseURL is the endpoint of the releases.
	baseURL string
	// client is the HTTP client used for the requests.
	client *http.Client
	// log logs the progress of the requests.
	log *Logger
}

// newReleaseClient returns a client for the releases of the repository.
func newReleaseClient(log *Logger) *releaseClient {
	return &releaseClient{
		baseURL: releasesURL,
		client:  &http.Client{Timeout: 30 * time.Second},
		log:     log,
	}
}

// releases returns every release of the repository.
func (c *releaseClient) releases(ctx context.Context) ([]release, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL, nil)
	if err != nil {
		return nil, fmt.Errorf("building the request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", appName)

	c.log.Debugf("GET %s", c.baseURL)

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("querying the releases: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("querying the releases: unexpected status %s", resp.Status)
	}

	var releases []release
	if err := json.NewDecoder(resp.Body).Decode(&releases); err != nil {
		return nil, fmt.Errorf("decoding the releases: %w", err)
	}
	return releases, nil
}

// latestRelease returns the latest published release that is neither a draft
// nor a pre release.
func (c *releaseClient) latestRelease(ctx context.Context) (release, error) {
	releases, err := c.releases(ctx)
	if err != nil {
		return release{}, err
	}
	latest, ok := selectLatestRelease(releases)
	if !ok {
		return release{}, fmt.Errorf("the repository has no published release yet")
	}
	return latest, nil
}

// selectLatestRelease returns the newest release of the list. Drafts and pre
// releases are ignored, so the latest stable release is used.
func selectLatestRelease(releases []release) (release, bool) {
	var latest release
	found := false

	for _, candidate := range releases {
		if candidate.Draft || candidate.Prerelease || candidate.TagName == "" {
			continue
		}
		if !found {
			latest, found = candidate, true
			continue
		}
		if compareVersions(candidate.TagName, latest.TagName) > 0 {
			latest = candidate
		}
	}
	return latest, found
}

// platformName returns the name of the platform used in the release
// archives, for example "linux-x86_64". It is empty for a platform without
// a release archive.
//
// The names of the archives follow the platforms of the build scripts, so
// macOS is called "macos" and not "darwin".
func platformName(goos, goarch string) string {
	var arch string
	switch goarch {
	case "amd64":
		arch = "x86_64"
	case "arm64":
		arch = "arm64"
	default:
		return ""
	}

	switch goos {
	case "linux":
		return "linux-" + arch
	case "darwin":
		return "macos-" + arch
	case "windows":
		return "windows-" + arch
	default:
		return ""
	}
}

// releaseAssetName returns the name of the release archive of a version for
// the given platform.
func releaseAssetName(version, platform string) string {
	if platform == "" {
		return ""
	}
	return fmt.Sprintf("%s_%s_%s.zip", appName, version, platform)
}

// selectArchive returns the release archive of the platform. The archive of
// the given version is preferred, but an archive of another version of the
// same platform is used as well, so a release that names its assets
// differently can still be installed.
func selectArchive(assets []asset, version, platform string) (asset, bool) {
	want := releaseAssetName(version, platform)
	for _, candidate := range assets {
		if candidate.Name == want {
			return candidate, true
		}
	}

	suffix := "_" + platform + ".zip"
	for _, candidate := range assets {
		if strings.HasPrefix(candidate.Name, appName+"_") && strings.HasSuffix(candidate.Name, suffix) {
			return candidate, true
		}
	}
	return asset{}, false
}

// selectChecksums returns the asset with the checksums of the release
// archives.
func selectChecksums(assets []asset) (asset, bool) {
	for _, candidate := range assets {
		if candidate.Name == checksumsName {
			return candidate, true
		}
	}
	for _, candidate := range assets {
		if strings.HasSuffix(candidate.Name, checksumsName) {
			return candidate, true
		}
	}
	return asset{}, false
}
