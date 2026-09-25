package selfupdate

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type Channel string

const (
	Stable Channel = "stable"
	Beta   Channel = "beta"
)

func (c Channel) Valid() bool { return c == Stable || c == Beta }
func (c Channel) Label() string {
	if c == Beta {
		return "Beta"
	}
	return "Stable"
}

type version struct {
	numbers [3]uint64
	beta    uint64
}

func parseVersion(value string) (version, error) {
	base, suffix, prerelease := strings.Cut(value, "-")
	numbers, err := versionNumbers(base)
	if err != nil {
		return version{}, err
	}
	v := version{numbers: numbers}
	if prerelease {
		if !strings.HasPrefix(suffix, "beta.") {
			return version{}, fmt.Errorf("unsupported release version %q", value)
		}
		n := strings.TrimPrefix(suffix, "beta.")
		if n == "" || n[0] == '0' {
			return version{}, fmt.Errorf("invalid beta version %q", value)
		}
		for _, digit := range n {
			if digit < '0' || digit > '9' {
				return version{}, fmt.Errorf("invalid beta version %q", value)
			}
		}
		v.beta, err = strconv.ParseUint(n, 10, 32)
		if err != nil {
			return version{}, err
		}
	}
	return v, nil
}

func (v version) channel() Channel {
	if v.beta != 0 {
		return Beta
	}
	return Stable
}

func CurrentChannel(value string) Channel {
	v, _ := parseVersion(value)
	return v.channel()
}

// Cross-channel replacements are tied to the version that requested the switch.
// Normal updates never downgrade or change channels, including when another
// editor instance has replaced the installation since the download began.
func CanInstall(release Release, current string) bool {
	next, err := parseVersion(release.Version)
	if err != nil {
		return false
	}
	previous, err := parseVersion(current)
	if err != nil {
		return false
	}
	if next.channel() != previous.channel() {
		from, err := parseVersion(release.SwitchFrom)
		return err == nil && from == previous
	}
	return release.SwitchFrom == "" && Newer(release.Version, current)
}

func CheckChannel(ctx context.Context, current string, channel Channel) (Release, error) {
	if !Supported() {
		return Release{}, fmt.Errorf("automatic updates require Windows x64")
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	return checkChannel(ctx, httpClient(), current, channel, strings.TrimSuffix(releaseAPI, "/latest"))
}

func readReleaseData(ctx context.Context, client *http.Client, url string) ([]byte, error) {
	resp, err := get(ctx, client, url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	const limit = 8 << 20
	data, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if len(data) > limit {
		return nil, fmt.Errorf("release details are too large")
	}
	return data, nil
}

func checkChannel(ctx context.Context, client *http.Client, current string, channel Channel, api string) (Release, error) {
	if !channel.Valid() {
		return Release{}, fmt.Errorf("unknown release channel")
	}
	if _, err := parseVersion(current); err != nil {
		return Release{}, fmt.Errorf("development builds do not receive automatic updates")
	}
	latestStable := func() (githubRelease, error) {
		data, err := readReleaseData(ctx, client, api+"/latest")
		if err != nil {
			return githubRelease{}, err
		}
		var source githubRelease
		if err := json.Unmarshal(data, &source); err != nil {
			return githubRelease{}, fmt.Errorf("read release details: %w", err)
		}
		return source, nil
	}
	if channel == Stable {
		source, err := latestStable()
		if err != nil {
			return Release{}, err
		}
		return parseChannelRelease(source, current, channel)
	}
	var latest githubRelease
	for page := 1; ; page++ {
		data, err := readReleaseData(ctx, client, fmt.Sprintf("%s?per_page=100&page=%d", api, page))
		if err != nil {
			return Release{}, err
		}
		var releases []githubRelease
		if err := json.Unmarshal(data, &releases); err != nil {
			return Release{}, fmt.Errorf("read beta releases: %w", err)
		}
		for _, source := range releases {
			v, err := parseVersion(source.Tag)
			if err != nil || source.Draft || !source.Prerelease || v.channel() != Beta {
				continue
			}
			if latest.Tag == "" || Newer(source.Tag, latest.Tag) {
				latest = source
			}
		}
		if len(releases) < 100 {
			break
		}
	}
	// Stable releases often promote a beta and move past it. Beta users must
	// not sit on an older build, and Stable users must not "switch" backwards.
	if stable, err := latestStable(); err == nil && !stable.Draft && !stable.Prerelease &&
		(latest.Tag == "" || Newer(stable.Tag, latest.Tag)) {
		stableVersion := strings.TrimPrefix(stable.Tag, "v")
		behind := "Stable " + stableVersion + " is newer than any beta"
		if latest.Tag != "" {
			behind = "The newest beta, " + strings.TrimPrefix(latest.Tag, "v") + ", is older than Stable " + stableVersion
		}
		if CurrentChannel(current) == Stable {
			return Release{Notice: behind + ", so switching now would downgrade. Stay on Stable until a newer beta is published."}, nil
		}
		if Newer(stable.Tag, current) {
			release, err := parseChannelRelease(stable, current, Stable)
			if err == nil && release.Version != "" {
				release.Notice = "Beta is behind Stable. " + behind + ", which has fixes this beta lacks. Switch to Stable."
				return release, nil
			}
		}
	}
	if latest.Tag == "" {
		return Release{}, fmt.Errorf("no Windows beta release is published yet")
	}
	return parseChannelRelease(latest, current, channel)
}
