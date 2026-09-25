package selfupdate

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"runtime"
	"strconv"
	"strings"
	"time"

	"sdmm/internal/env"
)

const releaseAPI = "https://api.github.com/repos/voidcrew/Voidworks/releases/latest"
const releaseDownloads = "https://github.com/voidcrew/Voidworks/releases/download/"
const maxDownload = 256 << 20

type Release struct {
	Version     string
	Description string
	URL         string
	SHA256      string
	Size        int64
	SwitchFrom  string
	// Notice explains a channel problem, such as Beta falling behind Stable.
	Notice string
}

type githubRelease struct {
	Tag        string `json:"tag_name"`
	Body       string `json:"body"`
	Draft      bool   `json:"draft"`
	Prerelease bool   `json:"prerelease"`
	Assets     []struct {
		Name   string `json:"name"`
		URL    string `json:"browser_download_url"`
		Digest string `json:"digest"`
		Size   int64  `json:"size"`
	} `json:"assets"`
}

func Supported() bool { return runtime.GOOS == "windows" && runtime.GOARCH == "amd64" }

// Stable version components are also used by numbered beta releases.
func versionNumbers(version string) ([3]uint64, error) {
	var result [3]uint64
	parts := strings.Split(strings.TrimPrefix(version, "v"), ".")
	if len(parts) != 3 {
		return result, fmt.Errorf("invalid stable version %q", version)
	}
	for i, part := range parts {
		if part == "" || (len(part) > 1 && part[0] == '0') {
			return result, fmt.Errorf("invalid stable version %q", version)
		}
		for _, c := range part {
			if c < '0' || c > '9' {
				return result, fmt.Errorf("invalid stable version %q", version)
			}
		}
		n, err := strconv.ParseUint(part, 10, 32)
		if err != nil {
			return result, err
		}
		result[i] = n
	}
	return result, nil
}

func Newer(candidate, current string) bool {
	next, err := parseVersion(candidate)
	if err != nil {
		return false
	}
	previous, err := parseVersion(current)
	if err != nil {
		return false
	}
	for i := range next.numbers {
		if next.numbers[i] != previous.numbers[i] {
			return next.numbers[i] > previous.numbers[i]
		}
	}
	if next.beta == previous.beta {
		return false
	}
	return next.beta == 0 || (previous.beta != 0 && next.beta > previous.beta)
}

func packageName(version string) string { return "Voidworks-" + version + "-windows-x64" }

func parseRelease(data []byte, current string) (Release, error) {
	var source githubRelease
	if err := json.Unmarshal(data, &source); err != nil {
		return Release{}, fmt.Errorf("read release details: %w", err)
	}
	return parseChannelRelease(source, current, Stable)
}

func parseChannelRelease(source githubRelease, current string, channel Channel) (Release, error) {
	v, err := parseVersion(source.Tag)
	if err != nil {
		return Release{}, err
	}
	if !channel.Valid() || source.Draft || v.channel() != channel || source.Prerelease != (channel == Beta) {
		return Release{}, fmt.Errorf("release is not a published %s build", channel.Label())
	}
	previous, err := parseVersion(current)
	if err != nil {
		return Release{}, err
	}
	switchFrom := ""
	if previous.channel() != channel {
		switchFrom = strings.TrimPrefix(current, "v")
	} else if !Newer(source.Tag, current) {
		return Release{}, nil
	}
	version := strings.TrimPrefix(source.Tag, "v")
	name := packageName(version) + ".zip"
	var result Release
	for _, asset := range source.Assets {
		if asset.Name != name {
			continue
		}
		if result.Version != "" {
			return Release{}, fmt.Errorf("release contains duplicate Windows packages")
		}
		digest := strings.TrimPrefix(asset.Digest, "sha256:")
		hash, err := hex.DecodeString(digest)
		if err != nil || len(hash) != 32 || asset.Digest == digest {
			return Release{}, fmt.Errorf("release package has no valid SHA-256 digest")
		}
		if asset.URL != releaseDownloads+source.Tag+"/"+name {
			return Release{}, fmt.Errorf("release package is outside voidcrew/Voidworks")
		}
		if asset.Size <= 0 || asset.Size > maxDownload {
			return Release{}, fmt.Errorf("release package size is invalid")
		}
		result = Release{Version: version, Description: source.Body, URL: asset.URL, SHA256: strings.ToLower(digest), Size: asset.Size, SwitchFrom: switchFrom}
	}
	if result.Version == "" {
		return Release{}, fmt.Errorf("the latest release has no Windows x64 package")
	}
	return result, nil
}

func httpClient() *http.Client {
	return &http.Client{
		Timeout: 10 * time.Minute,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return fmt.Errorf("too many download redirects")
			}
			// GitHub answers a renamed repository with an api.github.com redirect,
			// so release checks must follow it as well as package downloads.
			switch req.URL.Host {
			case "github.com", "api.github.com", "release-assets.githubusercontent.com", "objects.githubusercontent.com":
				if req.URL.Scheme == "https" && req.URL.User == nil {
					return nil
				}
			}
			return fmt.Errorf("download redirected outside GitHub (%s)", req.URL.Host)
		},
	}
}

func get(ctx context.Context, client *http.Client, url string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Voidworks/"+env.Version)
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("connect to GitHub: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		if resp.StatusCode == 403 || resp.StatusCode == 429 {
			return nil, fmt.Errorf("GitHub temporarily limited update checks; try again later")
		}
		return nil, fmt.Errorf("GitHub returned HTTP %d", resp.StatusCode)
	}
	return resp, nil
}

func Check(ctx context.Context, current string) (Release, error) {
	return CheckChannel(ctx, current, CurrentChannel(current))
}
