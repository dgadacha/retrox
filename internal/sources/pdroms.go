package sources

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"retrox/internal/platforms"
)

// pdRomsFeedURL is the global RSS — there's no per-platform feed, so we
// parse the categories of each item to route them to our catalog IDs.
const pdRomsFeedURL = "https://pdroms.de/feed"

// pdRomsCategoryToPlatform maps PDRoms's `<category>` tags onto our
// internal platform IDs. Many entries carry several tags; we take the
// first one that matches.
var pdRomsCategoryToPlatform = map[string]string{
	"NES":                                       "nes",
	"Nintendo Entertainment System (Famicom)":   "nes",
	"Famicom":                                   "nes",
	"SNES":                                      "snes",
	"Super Nintendo":                            "snes",
	"Super Nintendo Entertainment System":       "snes",
	"Nintendo 64":                               "n64",
	"N64":                                       "n64",
	"Game Boy":                                  "gb",
	"Game Boy Color":                            "gbc",
	"GBC":                                       "gbc",
	"Game Boy Advance":                          "gba",
	"GBA":                                       "gba",
	"Nintendo DS":                               "nds",
	"NDS":                                       "nds",
	"GameCube":                                  "gamecube",
	"Wii":                                       "wii",
	"Master System":                             "mastersystem",
	"Sega Mega Drive":                           "megadrive",
	"Mega Drive":                                "megadrive",
	"Mega Drive / Genesis":                      "megadrive",
	"Genesis":                                   "megadrive",
	"Sega Genesis":                              "megadrive",
	"Game Gear":                                 "gamegear",
	"Sega 32X":                                  "sega32x",
	"32X":                                       "sega32x",
	"Saturn":                                    "saturn",
	"Sega Saturn":                               "saturn",
	"Dreamcast":                                 "dreamcast",
	"Sega Dreamcast":                            "dreamcast",
	"PlayStation":                               "psx",
	"PSX":                                       "psx",
	"PS1":                                       "psx",
	"PSP":                                       "psp",
	"PC Engine":                                 "pcengine",
	"TurboGrafx-16":                             "pcengine",
	"TurboGrafx 16":                             "pcengine",
	"Neo Geo":                                   "neogeo",
	"Neo Geo Pocket":                            "ngp",
	"NGP":                                       "ngp",
	"Atari 2600":                                "atari2600",
	"Atari 7800":                                "atari7800",
	"Atari Lynx":                                "lynx",
	"Lynx":                                      "lynx",
	"WonderSwan":                                "wonderswan",
}

type PDRoms struct {
	http  *http.Client
	mu    sync.Mutex
	cache []ROM
	at    time.Time
}

func NewPDRoms() *PDRoms {
	return &PDRoms{http: &http.Client{Timeout: 15 * time.Second}}
}

func (p *PDRoms) ID() string   { return "pdroms" }
func (p *PDRoms) Name() string { return "PDRoms" }
func (p *PDRoms) Description() string {
	return "Flux RSS de pdroms.de — homebrew récent. Le téléchargement passe par le site (pdroms n'expose plus l'URL directe)."
}
func (p *PDRoms) Downloadable() bool { return false }
func (p *PDRoms) SupportedPlatforms() []string {
	seen := map[string]bool{}
	var out []string
	for _, pid := range pdRomsCategoryToPlatform {
		if !seen[pid] {
			seen[pid] = true
			out = append(out, pid)
		}
	}
	return out
}

// Browse returns the latest RSS items, filtered to those whose
// categories map to opts.PlatformID (or all if empty), optionally
// restricted by a substring query on the title.
func (p *PDRoms) Browse(ctx context.Context, opts BrowseOptions) (*Page, error) {
	items, err := p.feed(ctx)
	if err != nil {
		return nil, err
	}
	filtered := items
	if opts.PlatformID != "" {
		filtered = nil
		for _, it := range items {
			if it.PlatformID == opts.PlatformID {
				filtered = append(filtered, it)
			}
		}
	}
	if q := strings.TrimSpace(strings.ToLower(opts.Query)); q != "" {
		var out []ROM
		for _, it := range filtered {
			if strings.Contains(strings.ToLower(it.Title), q) {
				out = append(out, it)
			}
		}
		filtered = out
	}
	// RSS is small (< 60 items), one page is enough.
	return &Page{Items: filtered, HasMore: false, NextPage: 1}, nil
}

// Resolve always errors — PDRoms articles no longer expose the file URL.
func (p *PDRoms) Resolve(ctx context.Context, romID string) (string, error) {
	return "", ErrNotDownloadable
}

// feed fetches + parses the RSS, with a 30-minute in-memory cache so
// browsing platforms doesn't refetch the same XML.
func (p *PDRoms) feed(ctx context.Context) ([]ROM, error) {
	p.mu.Lock()
	if time.Since(p.at) < 30*time.Minute && p.cache != nil {
		out := p.cache
		p.mu.Unlock()
		return out, nil
	}
	p.mu.Unlock()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, pdRomsFeedURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "RETROX/0.1")
	res, err := p.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("pdroms %d", res.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(res.Body, 4<<20))
	if err != nil {
		return nil, err
	}

	type rssItem struct {
		Title       string   `xml:"title"`
		Link        string   `xml:"link"`
		Description string   `xml:"description"`
		Categories  []string `xml:"category"`
		PubDate     string   `xml:"pubDate"`
	}
	type rss struct {
		XMLName xml.Name `xml:"rss"`
		Items   []rssItem `xml:"channel>item"`
	}
	var parsed rss
	if err := xml.Unmarshal(body, &parsed); err != nil {
		return nil, err
	}

	stripHTML := regexp.MustCompile(`<[^>]*>|&[a-z]+;`)
	var out []ROM
	for _, it := range parsed.Items {
		platform := ""
		for _, c := range it.Categories {
			if pid, ok := pdRomsCategoryToPlatform[c]; ok {
				platform = pid
				break
			}
		}
		if platform == "" {
			// Last-chance match against catalog names so a "Mega Drive
			// / Genesis"-style cat we forgot to map still routes.
			for _, c := range it.Categories {
				if p, ok := platforms.ByID(strings.ToLower(c)); ok {
					platform = p.ID
					break
				}
			}
		}
		if platform == "" {
			continue // skip items we can't bucket
		}

		desc := truncate(strings.TrimSpace(stripHTML.ReplaceAllString(it.Description, " ")), 280)
		out = append(out, ROM{
			SourceID:     "pdroms",
			ID:           it.Link,
			Title:        strings.TrimSpace(it.Title),
			PlatformID:   platform,
			Description:  desc,
			Downloadable: false,
			ExternalURL:  it.Link,
		})
	}

	p.mu.Lock()
	p.cache = out
	p.at = time.Now()
	p.mu.Unlock()
	return out, nil
}

var _ = errors.New
