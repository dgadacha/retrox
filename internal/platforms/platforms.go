// Package platforms is RETROX's static catalog of retro systems.
//
// Each Platform ties together three worlds:
//   - the filesystem  (which ROM extensions belong to it),
//   - metadata sources (OpenVGDB system id, libretro thumbnails folder),
//   - the emulator    (a libretro core name and/or a standalone app key).
//
// The scanner uses Guess() to map a ROM file to a platform id; the
// emulator package turns (platform, binding) into a launch command; and
// the metadata provider uses OpenVGDBID + LibretroThumbsName to fetch
// titles, descriptions and box art.
package platforms

import (
	"path/filepath"
	"strings"
)

// Platform is one retro system. Zero OpenVGDBID means "no OpenVGDB
// coverage" — the game is still listed and may still grab a cover from
// libretro thumbnails or IGDB, just without text metadata from OpenVGDB.
type Platform struct {
	ID   string `json:"id"`   // stable internal id, e.g. "snes"
	Name string `json:"name"` // human label, e.g. "Super Nintendo"

	// OpenVGDBID is the systemID in the bundled openvgdb.sqlite. Zero
	// means OpenVGDB doesn't cover this system (e.g. Dreamcast, PS2).
	OpenVGDBID int `json:"openvgdbId"`

	// IGDBID is the platform id at api.igdb.com/v4/platforms. Used as
	// a fallback metadata source — primarily for systems OpenVGDB
	// doesn't cover (PS2, Dreamcast, Wii, Neo Geo).
	IGDBID int `json:"igdbId"`

	// TGDBID is the platform id at api.thegamesdb.net/v1/Platforms.
	// Alternative metadata source — useful when IGDB isn't configured
	// (Twitch dev account hassle).
	TGDBID int `json:"tgdbId"`

	// LibretroThumbsName is the folder under https://thumbnails.libretro.com
	// that hosts box art for this system (e.g. "Nintendo - Super Nintendo
	// Entertainment System"). Empty when there's no matching repo.
	LibretroThumbsName string `json:"libretroThumbsName,omitempty"`

	// Exts are the lowercase, dot-prefixed ROM extensions. Several
	// systems share disc-image extensions (.iso/.chd/.cue) — those are
	// disambiguated by a folder-name hint at scan time, see Guess.
	Exts []string `json:"exts"`

	// Core is the libretro core name WITHOUT the platform suffix, e.g.
	// "snes9x" → snes9x_libretro.dylib. Empty when no good core exists
	// (the system then requires a Standalone emulator).
	Core string `json:"core"`

	// Standalone names a non-RetroArch emulator that gives a better
	// experience for this system (Dolphin for GC/Wii, PCSX2 for PS2…).
	// When set, the emulator package prefers it over the libretro core.
	Standalone string `json:"standalone,omitempty"`

	// Aliases are extra folder-name tokens that hint at this platform
	// when the extension alone is ambiguous ("PS2", "psx", "megadrive"…).
	// The id and Name are always treated as aliases too.
	Aliases []string `json:"-"`
}

// catalog is the source of truth. Order roughly by manufacturer so the
// home page rails come out in a familiar grouping when sorted by it.
//
// OpenVGDBID values come from `SELECT systemID, systemName FROM SYSTEMS`
// against the bundled openvgdb.sqlite. LibretroThumbsName values match
// the repo folder names at https://github.com/libretro-thumbnails.
var catalog = []Platform{
	// --- Nintendo ---
	{ID: "nes", Name: "Nintendo (NES)", OpenVGDBID: 25, IGDBID: 18, TGDBID: 7, LibretroThumbsName: "Nintendo - Nintendo Entertainment System", Exts: []string{".nes", ".fds", ".unf"}, Core: "nestopia", Aliases: []string{"famicom", "fc"}},
	{ID: "snes", Name: "Super Nintendo", OpenVGDBID: 26, IGDBID: 19, TGDBID: 6, LibretroThumbsName: "Nintendo - Super Nintendo Entertainment System", Exts: []string{".sfc", ".smc"}, Core: "snes9x", Aliases: []string{"superfamicom", "sfc", "super nintendo"}},
	{ID: "n64", Name: "Nintendo 64", OpenVGDBID: 23, IGDBID: 4, TGDBID: 3, LibretroThumbsName: "Nintendo - Nintendo 64", Exts: []string{".n64", ".z64", ".v64"}, Core: "mupen64plus_next", Aliases: []string{"nintendo64"}},
	{ID: "gb", Name: "Game Boy", OpenVGDBID: 19, IGDBID: 33, TGDBID: 4, LibretroThumbsName: "Nintendo - Game Boy", Exts: []string{".gb"}, Core: "gambatte", Aliases: []string{"gameboy"}},
	{ID: "gbc", Name: "Game Boy Color", OpenVGDBID: 21, IGDBID: 22, TGDBID: 41, LibretroThumbsName: "Nintendo - Game Boy Color", Exts: []string{".gbc"}, Core: "gambatte", Aliases: []string{"gameboycolor"}},
	{ID: "gba", Name: "Game Boy Advance", OpenVGDBID: 20, IGDBID: 24, TGDBID: 5, LibretroThumbsName: "Nintendo - Game Boy Advance", Exts: []string{".gba", ".srl"}, Core: "mgba", Aliases: []string{"gameboyadvance", "advance"}},
	{ID: "nds", Name: "Nintendo DS", OpenVGDBID: 24, IGDBID: 20, TGDBID: 8, LibretroThumbsName: "Nintendo - Nintendo DS", Exts: []string{".nds"}, Core: "melonds", Aliases: []string{"ds", "ndsi"}},
	{ID: "gamecube", Name: "GameCube", OpenVGDBID: 22, IGDBID: 21, TGDBID: 2, LibretroThumbsName: "Nintendo - GameCube", Exts: []string{".rvz", ".gcm", ".gcz"}, Core: "dolphin", Standalone: "dolphin", Aliases: []string{"gc", "ngc", "cube"}},
	{ID: "wii", Name: "Nintendo Wii", OpenVGDBID: 28, IGDBID: 5, TGDBID: 9, Exts: []string{".wbfs", ".wad"}, Core: "dolphin", Standalone: "dolphin", Aliases: []string{"nintendowii"}},

	// --- Sega ---
	{ID: "mastersystem", Name: "Master System", OpenVGDBID: 31, IGDBID: 64, TGDBID: 35, LibretroThumbsName: "Sega - Master System - Mark III", Exts: []string{".sms"}, Core: "genesis_plus_gx", Aliases: []string{"sms", "sega master system"}},
	{ID: "megadrive", Name: "Mega Drive / Genesis", OpenVGDBID: 33, IGDBID: 29, TGDBID: 36, LibretroThumbsName: "Sega - Mega Drive - Genesis", Exts: []string{".md", ".gen", ".smd"}, Core: "genesis_plus_gx", Aliases: []string{"genesis", "megadrive", "mega drive", "sega genesis"}},
	{ID: "gamegear", Name: "Game Gear", OpenVGDBID: 30, IGDBID: 35, TGDBID: 20, LibretroThumbsName: "Sega - Game Gear", Exts: []string{".gg"}, Core: "genesis_plus_gx", Aliases: []string{"gg", "gamegear"}},
	{ID: "sega32x", Name: "Sega 32X", OpenVGDBID: 29, IGDBID: 30, TGDBID: 33, LibretroThumbsName: "Sega - 32X", Exts: []string{".32x"}, Core: "picodrive", Aliases: []string{"32x"}},
	{ID: "saturn", Name: "Sega Saturn", OpenVGDBID: 34, IGDBID: 32, TGDBID: 17, LibretroThumbsName: "Sega - Saturn", Exts: []string{}, Core: "mednafen_saturn", Aliases: []string{"segasaturn"}},
	{ID: "dreamcast", Name: "Dreamcast", IGDBID: 23, TGDBID: 16, LibretroThumbsName: "Sega - Dreamcast", Exts: []string{".gdi", ".cdi"}, Core: "flycast", Aliases: []string{"dc", "segadreamcast"}},

	// --- Sony ---
	{ID: "psx", Name: "PlayStation", OpenVGDBID: 38, IGDBID: 7, TGDBID: 10, LibretroThumbsName: "Sony - PlayStation", Exts: []string{".pbp"}, Core: "swanstation", Aliases: []string{"ps1", "psone", "playstation", "psxe"}},
	{ID: "ps2", Name: "PlayStation 2", IGDBID: 8, TGDBID: 11, Exts: []string{}, Standalone: "pcsx2", Aliases: []string{"ps2", "playstation2"}},
	{ID: "psp", Name: "PlayStation Portable", OpenVGDBID: 39, IGDBID: 38, TGDBID: 13, LibretroThumbsName: "Sony - PlayStation Portable", Exts: []string{".cso"}, Core: "ppsspp", Standalone: "ppsspp", Aliases: []string{"psp", "playstationportable"}},

	// --- NEC / SNK / Atari / Bandai ---
	{ID: "pcengine", Name: "PC Engine / TurboGrafx-16", OpenVGDBID: 14, IGDBID: 86, TGDBID: 34, LibretroThumbsName: "NEC - PC Engine - TurboGrafx 16", Exts: []string{".pce"}, Core: "mednafen_pce", Aliases: []string{"turbografx", "tg16", "pcengine"}},
	{ID: "neogeo", Name: "Neo Geo", IGDBID: 80, TGDBID: 24, LibretroThumbsName: "SNK - Neo Geo", Exts: []string{".neo"}, Core: "fbneo", Aliases: []string{"neogeo", "aes", "mvs"}},
	{ID: "ngp", Name: "Neo Geo Pocket", OpenVGDBID: 36, IGDBID: 119, TGDBID: 25, LibretroThumbsName: "SNK - Neo Geo Pocket", Exts: []string{".ngp", ".ngc"}, Core: "mednafen_ngp", Aliases: []string{"neogeopocket"}},
	{ID: "atari2600", Name: "Atari 2600", OpenVGDBID: 3, IGDBID: 59, TGDBID: 22, LibretroThumbsName: "Atari - 2600", Exts: []string{".a26"}, Core: "stella", Aliases: []string{"atari", "2600", "vcs"}},
	{ID: "atari7800", Name: "Atari 7800", OpenVGDBID: 5, IGDBID: 60, TGDBID: 27, LibretroThumbsName: "Atari - 7800", Exts: []string{".a78"}, Core: "prosystem", Aliases: []string{"7800"}},
	{ID: "lynx", Name: "Atari Lynx", OpenVGDBID: 6, IGDBID: 61, TGDBID: 4924, LibretroThumbsName: "Atari - Lynx", Exts: []string{".lnx"}, Core: "handy", Aliases: []string{"atarilynx"}},
	{ID: "wonderswan", Name: "WonderSwan", OpenVGDBID: 9, IGDBID: 57, TGDBID: 4925, LibretroThumbsName: "Bandai - WonderSwan", Exts: []string{".ws", ".wsc"}, Core: "mednafen_wswan", Aliases: []string{"swan"}},
	{ID: "arcade", Name: "Arcade (MAME)", OpenVGDBID: 2, IGDBID: 52, TGDBID: 23, Exts: []string{}, Core: "mame2003_plus", Aliases: []string{"mame", "fbneo", "fba", "coin-op"}},
}

// ambiguousExts are disc/archive extensions shared by several systems.
// A file with one of these is only platform-tagged when a folder-name
// hint resolves it; otherwise the scanner leaves PlatformID empty.
var ambiguousExts = map[string][]string{
	".iso": {"ps2", "psp", "gamecube", "wii", "dreamcast", "saturn"},
	".chd": {"psx", "ps2", "dreamcast", "saturn", "segacd", "pcenginecd"},
	".cue": {"psx", "saturn", "megadrive", "pcengine", "neogeo"},
	".bin": {"psx", "saturn", "megadrive"},
	".img": {"psx", "ps2"},
	".zip": {"arcade", "neogeo"},
	".7z":  {"arcade", "neogeo"},
}

// Lookup indexes built once at init.
var (
	byID      = map[string]Platform{}
	byExt     = map[string][]string{} // ext → []platformID (unambiguous catalog exts)
	aliasToID = map[string]string{}   // normalized alias token → platformID
)

func init() {
	for _, p := range catalog {
		byID[p.ID] = p
		for _, e := range p.Exts {
			byExt[e] = append(byExt[e], p.ID)
		}
		// id + name + explicit aliases all map back to the id.
		aliasToID[normalizeToken(p.ID)] = p.ID
		aliasToID[normalizeToken(p.Name)] = p.ID
		for _, a := range p.Aliases {
			aliasToID[normalizeToken(a)] = p.ID
		}
	}
	// Merge the ambiguous-ext candidate lists so byExt is complete; these
	// only resolve with a folder hint (handled in Guess).
	for e, ids := range ambiguousExts {
		byExt[e] = append(byExt[e], ids...)
	}
}

// All returns the catalog (copy-safe — callers get the backing slice but
// Platform is a value type, so mutating an element doesn't corrupt the
// index maps).
func All() []Platform { return catalog }

// ByID returns the platform and ok=false if the id is unknown.
func ByID(id string) (Platform, bool) {
	p, ok := byID[id]
	return p, ok
}

// IsROMExtension reports whether a file's extension is one RETROX might
// treat as a ROM — either a catalog extension or a shared disc/archive
// one. The scanner uses it to skip sidecar files (.srm, .nfo, .jpg…)
// without hashing them.
func IsROMExtension(path string) bool {
	_, ok := byExt[strings.ToLower(filepath.Ext(path))]
	return ok
}

// Name returns the display name for an id, or the id itself if unknown
// (so the UI never renders a blank rail header).
func Name(id string) string {
	if p, ok := byID[id]; ok {
		return p.Name
	}
	if id == "" {
		return "Non classé"
	}
	return id
}

// Guess maps a ROM file path to a platform id. Strategy:
//
//  1. Unambiguous extension (.nes, .sfc…) → that platform, done.
//  2. Ambiguous extension (.iso, .chd, .zip…) → resolve via the nearest
//     ancestor folder name that aliases one of the candidate platforms.
//  3. Any extension → fall back to a folder-name hint anywhere in the
//     path (covers extension-less or unknown-extension dumps dropped in
//     a clearly-named "SNES/" folder).
//
// Returns ok=false when nothing resolves; the scanner still records the
// game with an empty PlatformID so it shows up under "Non classé".
func Guess(path string) (string, bool) {
	ext := strings.ToLower(filepath.Ext(path))

	// 1 + 2: extension-driven.
	if candidates, ok := byExt[ext]; ok && len(candidates) > 0 {
		if len(dedupe(candidates)) == 1 {
			return candidates[0], true
		}
		if hinted, ok := folderHint(path); ok && contains(candidates, hinted) {
			return hinted, true
		}
		// Ambiguous and no decisive hint — defer to a global folder hint
		// below, else give up (caller stores empty platform).
	}

	// 3: folder-name hint regardless of extension.
	if hinted, ok := folderHint(path); ok {
		return hinted, true
	}
	return "", false
}

// folderHint scans the path's directory segments from the deepest
// upward, returning the first one that aliases a known platform.
func folderHint(path string) (string, bool) {
	dir := filepath.Dir(path)
	segs := strings.Split(dir, string(filepath.Separator))
	for i := len(segs) - 1; i >= 0; i-- {
		if id, ok := aliasToID[normalizeToken(segs[i])]; ok {
			return id, true
		}
	}
	return "", false
}

// normalizeToken lowercases and strips non-alphanumerics so "Mega Drive",
// "mega-drive" and "MegaDrive" all collapse to the same alias key.
func normalizeToken(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func contains(xs []string, x string) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}

func dedupe(xs []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, x := range xs {
		if !seen[x] {
			seen[x] = true
			out = append(out, x)
		}
	}
	return out
}
