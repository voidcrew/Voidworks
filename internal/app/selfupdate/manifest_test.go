package selfupdate

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestStableVersionOrdering(t *testing.T) {
	for _, test := range []struct {
		next, current string
		newer         bool
	}{
		{"0.5.2", "0.5.1", true}, {"v0.5.2", "0.5.1", true}, {"0.10.0", "0.9.9", true},
		{"1.0.0", "0.99.99", true}, {"0.5.1", "0.5.1", false}, {"0.5.0", "0.5.1", false},
		{"0.5.2-beta", "0.5.1", false}, {"0.5.2", "undefined", false}, {"0.5.2", "0.5.1-preview", false},
		{"0.05.2", "0.5.1", false}, {"0.+5.2", "0.5.1", false}, {"99999999999.0.0", "0.5.1", false},
	} {
		if got := Newer(test.next, test.current); got != test.newer {
			t.Errorf("Newer(%q,%q) = %v", test.next, test.current, got)
		}
	}
}

func TestReleaseValidation(t *testing.T) {
	fixture := `{"tag_name":"0.5.2","body":"Release notes","assets":[{"name":"Voidworks-0.5.2-windows-x64.zip","browser_download_url":"https://github.com/voidcrew/Voidworks/releases/download/0.5.2/Voidworks-0.5.2-windows-x64.zip","digest":"sha256:` + strings.Repeat("a", 64) + `","size":100}]}`
	for _, test := range []struct {
		name   string
		change func(*githubRelease)
	}{
		{"draft", func(r *githubRelease) { r.Draft = true }},
		{"prerelease", func(r *githubRelease) { r.Prerelease = true }},
		{"bad tag", func(r *githubRelease) { r.Tag = "../bad" }},
		{"no package", func(r *githubRelease) { r.Assets = nil }},
		{"no digest", func(r *githubRelease) { r.Assets[0].Digest = "" }},
		{"bad digest", func(r *githubRelease) { r.Assets[0].Digest = "sha256:" + strings.Repeat("z", 64) }},
		{"wrong repo", func(r *githubRelease) { r.Assets[0].URL = strings.ReplaceAll(r.Assets[0].URL, "voidcrew/", "SpaiR/") }},
		{"http", func(r *githubRelease) { r.Assets[0].URL = strings.ReplaceAll(r.Assets[0].URL, "https:", "http:") }},
		{"too big", func(r *githubRelease) { r.Assets[0].Size = maxDownload + 1 }},
		{"duplicate", func(r *githubRelease) { r.Assets = append(r.Assets, r.Assets[0]) }},
	} {
		t.Run(test.name, func(t *testing.T) {
			var source githubRelease
			if err := json.Unmarshal([]byte(fixture), &source); err != nil {
				t.Fatal(err)
			}
			test.change(&source)
			data, _ := json.Marshal(source)
			if _, err := parseRelease(data, "0.5.1"); err == nil {
				t.Fatal("accepted invalid release")
			}
		})
	}
	release, err := parseRelease([]byte(fixture), "0.5.1")
	if err != nil || release.Version != "0.5.2" {
		t.Fatalf("valid release: %+v %v", release, err)
	}
	var withInstaller githubRelease
	if err := json.Unmarshal([]byte(fixture), &withInstaller); err != nil {
		t.Fatal(err)
	}
	installer := withInstaller.Assets[0]
	installer.Name = "Voidworks-0.5.2-windows-x64-setup.exe"
	installer.URL = releaseDownloads + "0.5.2/" + installer.Name
	withInstaller.Assets = append(withInstaller.Assets, installer)
	data, _ := json.Marshal(withInstaller)
	if release, err := parseRelease(data, "0.5.1"); err != nil || !strings.HasSuffix(release.URL, "-windows-x64.zip") {
		t.Fatalf("setup program was mistaken for the update package: %+v %v", release, err)
	}
	for _, current := range []string{"0.5.2", "0.6.0"} {
		if release, err := parseRelease([]byte(fixture), current); err != nil || release.Version != "" {
			t.Fatalf("offered downgrade or reinstall: %+v %v", release, err)
		}
	}
}

func TestDownloadRedirectPolicy(t *testing.T) {
	client := httpClient()
	for _, test := range []struct {
		url     string
		allowed bool
	}{
		{"https://release-assets.githubusercontent.com/path", true},
		{"https://objects.githubusercontent.com/path", true},
		{"https://api.github.com/repositories/1360753474/releases?per_page=100&page=1", true},
		{"http://api.github.com/repositories/1360753474/releases", false},
		{"http://github.com/path", false}, {"https://example.com/payload", false},
		{"https://github.com.evil.example/payload", false}, {"https://user@github.com/path", false},
	} {
		req, _ := http.NewRequest(http.MethodGet, test.url, nil)
		if err := client.CheckRedirect(req, nil); (err == nil) != test.allowed {
			t.Errorf("redirect %s: %v", test.url, err)
		}
	}
}
