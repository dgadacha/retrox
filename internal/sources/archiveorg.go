package sources

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"retrox/internal/platforms"
)

const (
	iaSearchURL   = "https://archive.org/advancedsearch.php"
	iaMetadataURL = "https://archive.org/metadata"
	iaDownloadURL = "https://archive.org/download"
	iaSearchLimit = 12 // candidates surfaced per game per source
)

// archiveOrgSubject is the value we put in `subject:"…"` Lucene queries
// for each of our platform IDs. Picked by sampling search counts —
// IA users tend to tag with the short name (`nes` beats `Nintendo
// Entertainment System` 4-to-1).
var archiveOrgSubject = map[string]string{
	"nes":          "nes",
	"snes":         "snes",
	"n64":          "Nintendo 64",
	"gb":           "gameboy",
	"gbc":          "gameboy color",
	"gba":          "Game Boy Advance",
	"nds":          "Nintendo DS",
	"gamecube":     "GameCube",
	"wii":          "Wii",
	"mastersystem": "Master System",
	"megadrive":    "Mega Drive",
	"gamegear":     "Game Gear",
	"sega32x":      "Sega 32X",
	"saturn":       "Saturn",
	"dreamcast":    "Dreamcast",
	"psx":          "PlayStation",
	"ps2":          "PlayStation 2",
	"psp":          "PSP",
	"pcengine":     "PC Engine",
	"neogeo":       "Neo Geo",
	"ngp":          "Neo Geo Pocket",
	"atari2600":    "Atari 2600",
	"atari7800":    "Atari 7800",
	"lynx":         "Atari Lynx",
	"wonderswan":   "WonderSwan",
	"arcade":       "Arcade",
}

type ArchiveOrg struct {
	http *http.Client
}

func NewArchiveOrg() *ArchiveOrg {
	return &ArchiveOrg{http: &http.Client{Timeout: 15 * time.Second}}
}

func (a *ArchiveOrg) ID() string   { return "archiveorg" }
func (a *ArchiveOrg) Name() string { return "Internet Archive" }
func (a *ArchiveOrg) Description() string {
	return "Recherche dans archive.org — collections legales (homebrew, public domain, abandonware autorisé)."
}
func (a *ArchiveOrg) Downloadable() bool { return true }
func (a *ArchiveOrg) SupportedPlatforms() []string {
	out := make([]string, 0, len(archiveOrgSubject))
	for k := range archiveOrgSubject {
		out = append(out, k)
	}
	return out
}

// Search asks IA's advancedsearch.php for items matching the title for
// the given platform. We always include `mediatype:software` and the
// platform subject; the title goes into a relevance-ranked phrase
// search. Empty title → nil result (the caller should never be asking
// "what does IA have on platform X" without a game in mind).
func (a *ArchiveOrg) Search(ctx context.Context, title, platformID string) ([]ROM, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		return nil, nil
	}
	subj, ok := archiveOrgSubject[platformID]
	if !ok {
		// Unsupported platform → don't search (would return junk).
		return nil, nil
	}

	cleanTitle := stripRegionTags(title)
	q := fmt.Sprintf(`mediatype:software AND subject:%q AND title:%q`, subj, cleanTitle)

	u, _ := url.Parse(iaSearchURL)
	qs := u.Query()
	qs.Set("q", q)
	qs.Set("output", "json")
	qs.Set("rows", fmt.Sprintf("%d", iaSearchLimit))
	qs.Set("sort[]", "downloads desc")
	for _, fl := range []string{"identifier", "title", "description", "item_size"} {
		qs.Add("fl[]", fl)
	}
	u.RawQuery = qs.Encode()

	body, err := a.get(ctx, u.String())
	if err != nil {
		return nil, err
	}
	var parsed struct {
		Response struct {
			Docs []struct {
				Identifier  string `json:"identifier"`
				Title       any    `json:"title"`
				Description any    `json:"description"`
				ItemSize    any    `json:"item_size"`
			} `json:"docs"`
		} `json:"response"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, err
	}

	items := make([]ROM, 0, len(parsed.Response.Docs))
	for _, d := range parsed.Response.Docs {
		items = append(items, ROM{
			SourceID:     a.ID(),
			ID:           d.Identifier,
			Title:        firstString(d.Title),
			PlatformID:   platformID,
			Description:  truncate(firstString(d.Description), 280),
			CoverURL:     fmt.Sprintf("https://archive.org/services/img/%s", d.Identifier),
			SizeBytes:    asInt64(d.ItemSize),
			Downloadable: true,
			ExternalURL:  fmt.Sprintf("https://archive.org/details/%s", d.Identifier),
		})
	}
	return items, nil
}

// Resolve looks up an item's metadata, picks the first file whose
// extension matches a known ROM extension (or .zip/.7z fallback), and
// returns its direct archive.org/download URL.
func (a *ArchiveOrg) Resolve(ctx context.Context, romID string) (string, error) {
	if romID == "" {
		return "", errors.New("archiveorg: empty rom id")
	}
	body, err := a.get(ctx, fmt.Sprintf("%s/%s", iaMetadataURL, url.PathEscape(romID)))
	if err != nil {
		return "", err
	}
	var parsed struct {
		Files []struct {
			Name string `json:"name"`
		} `json:"files"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return "", err
	}

	var rom, archive string
	for _, f := range parsed.Files {
		if platforms.IsROMExtension(f.Name) {
			rom = f.Name
			break
		}
		low := strings.ToLower(f.Name)
		if archive == "" && (strings.HasSuffix(low, ".zip") || strings.HasSuffix(low, ".7z")) {
			archive = f.Name
		}
	}
	pick := rom
	if pick == "" {
		pick = archive
	}
	if pick == "" {
		return "", fmt.Errorf("archiveorg: no ROM file in item %q", romID)
	}
	return fmt.Sprintf("%s/%s/%s", iaDownloadURL, url.PathEscape(romID), url.PathEscape(pick)), nil
}

func (a *ArchiveOrg) get(ctx context.Context, u string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "RETROX/0.1 (+https://github.com/dgadacha/retrox)")
	res, err := a.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("archiveorg %d", res.StatusCode)
	}
	return io.ReadAll(io.LimitReader(res.Body, 4<<20))
}

// IA's response fields are []string | string depending on the document.
func firstString(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case []any:
		if len(x) > 0 {
			if s, ok := x[0].(string); ok {
				return s
			}
		}
	}
	return ""
}

func asInt64(v any) int64 {
	switch x := v.(type) {
	case float64:
		return int64(x)
	case string:
		var n int64
		_, _ = fmt.Sscan(x, &n)
		return n
	}
	return 0
}

func truncate(s string, max int) string {
	s = strings.TrimSpace(s)
	if len(s) <= max {
		return s
	}
	return s[:max-1] + "…"
}

// stripRegionTags removes the trailing "(USA)" / "(Europe) (Rev 1)" etc.
// tags that OpenVGDB titles carry — IA titles usually don't include them
// so leaving them in tanks the relevance score.
func stripRegionTags(s string) string {
	for {
		i := strings.LastIndexAny(s, "([")
		if i <= 0 {
			break
		}
		// Only strip if the bracket is near the end (room for a short tag).
		if len(s)-i > 30 {
			break
		}
		s = strings.TrimSpace(s[:i])
	}
	return s
}
