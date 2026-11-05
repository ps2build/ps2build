package update

import (
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"
)

const manifestURL = "https://raw.githubusercontent.com/ps2build/manifest/main/latest.json"

type manifestFile struct {
	Version    int                      `json:"version"`
	Components map[string]manifestEntry `json:"components"`
}

type manifestEntry struct {
	Tag    string `json:"tag"`
	Assets []struct {
		Name string `json:"name"`
		URL  string `json:"url"`
	} `json:"assets"`
}

var (
	manifestOnce sync.Once
	manifestData manifestFile
)

func loadManifest() manifestFile {
	manifestOnce.Do(func() {
		client := http.Client{Timeout: 10 * time.Second}
		resp, err := client.Get(manifestURL)
		if err != nil {
			return
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return
		}
		var m manifestFile
		if json.NewDecoder(resp.Body).Decode(&m) == nil && m.Version == 1 {
			manifestData = m
		}
	})
	return manifestData
}

func latestFromManifest(repo string) (releaseInfo, bool) {
	name := repo[strings.LastIndex(repo, "/")+1:]
	entry, ok := loadManifest().Components[name]
	if !ok || entry.Tag == "" {
		return releaseInfo{}, false
	}
	rel := releaseInfo{TagName: entry.Tag}
	for _, a := range entry.Assets {
		rel.Assets = append(rel.Assets, releaseAsset{Name: a.Name, BrowserDownloadURL: a.URL})
	}
	return rel, true
}
