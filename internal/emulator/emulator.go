// Package emulator turns a (platform, ROM, optional override) tuple into
// a concrete launch command and spawns it on the host.
//
// RETROX is a self-hosted LAN launcher running on the same machine as the
// emulators, so "Play" literally fork/execs the native process (RetroArch
// with a libretro core, or a standalone like Dolphin / PCSX2 / PPSSPP).
// Resolution order for a platform:
//
//  1. An EmulatorBinding row (admin override) — full control over the
//     command + args, with {rom} and {core} placeholders.
//  2. The catalog's Standalone app, if installed.
//  3. The catalog's libretro Core, run through RetroArch.
//
// Everything is best-effort and overridable: the resolved command is
// returned so the UI/logs can show exactly what was launched.
package emulator

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"

	"retrox/internal/database/models"
	"retrox/internal/platforms"
)

// Config carries the optional, admin-set overrides for where RetroArch
// and its cores live. Empty fields fall back to per-OS auto-detection.
type Config struct {
	RetroArchBin   string // path to the RetroArch executable
	RetroArchCores string // directory holding *_libretro.{dylib,so,dll}
	// EmulatorsDir is the project-bundled emulators folder. When set,
	// .app bundles inside (RetroArch.app, Dolphin.app, …) are checked
	// before /Applications so a portable RETROX folder is preferred
	// over the host's system installs.
	EmulatorsDir string
}

// Resolved is the command RETROX will run (or did run). Display is a
// shell-ish rendering for the UI/logs — not meant to be re-parsed.
type Resolved struct {
	Command  string   `json:"command"`
	Args     []string `json:"args"`
	Display  string   `json:"display"`
	Emulator string   `json:"emulator"` // "retroarch" | standalone key
}

// Resolve computes the launch command without running it. Returns an
// error when the platform has no usable emulator (and no override).
func Resolve(cfg Config, platformID, romPath string, binding *models.EmulatorBinding) (Resolved, error) {
	// 1. Admin override.
	if binding != nil && strings.TrimSpace(binding.Command) != "" {
		corePath := ""
		if binding.Core != "" {
			corePath = resolveCorePath(cfg, binding.Core)
		}
		args := expandArgs(binding.Args, romPath, corePath)
		return finalize(binding.Command, args, "custom"), nil
	}

	p, ok := platforms.ByID(platformID)
	if !ok {
		return Resolved{}, fmt.Errorf("plateforme inconnue %q — configurez un émulateur dans les réglages", platformID)
	}

	// 2. Standalone (preferred when the catalog names one and it's found).
	if p.Standalone != "" {
		if bin := findStandalone(cfg, p.Standalone); bin != "" {
			return finalize(bin, standaloneArgs(p.Standalone, romPath), p.Standalone), nil
		}
	}

	// 3. RetroArch + libretro core.
	if p.Core != "" {
		ra := cfg.RetroArchBin
		if ra == "" {
			ra = findStandalone(cfg, "retroarch")
		}
		if ra == "" {
			return Resolved{}, fmt.Errorf("RetroArch introuvable — installez-le ou définissez le chemin dans les réglages")
		}
		core := resolveCorePath(cfg, p.Core)
		if core == "" {
			return Resolved{}, fmt.Errorf("core libretro %q introuvable — installez-le via RetroArch ou liez un émulateur", p.Core)
		}
		return finalize(ra, []string{"-L", core, romPath}, "retroarch"), nil
	}

	return Resolved{}, fmt.Errorf("aucun émulateur par défaut pour %q — configurez-en un dans les réglages", p.Name)
}

// Launch resolves then spawns the process, detached from RETROX so it
// keeps running (and dying) independently of the server. Returns the
// resolved command for logging/UI.
func Launch(cfg Config, platformID, romPath string, binding *models.EmulatorBinding) (Resolved, error) {
	r, err := Resolve(cfg, platformID, romPath, binding)
	if err != nil {
		return r, err
	}
	cmd := exec.Command(r.Command, r.Args...)
	// New process group so it survives independently and a server
	// shutdown doesn't take the game down with it.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		return r, fmt.Errorf("lancement de %s: %w", filepath.Base(r.Command), err)
	}
	// Reap the child in the background so it doesn't become a zombie.
	go func() { _ = cmd.Wait() }()
	return r, nil
}

// -----------------------------------------------------------------------------
// Standalone discovery
// -----------------------------------------------------------------------------

// systemStandalones maps a catalog key to the system-wide .app paths
// and PATH names we fall back to when no project-bundled copy exists.
// On macOS we point straight at the binary inside the .app; the PATH
// lookup at the end covers Homebrew shims / Linux installs.
var systemStandalones = map[string][]string{
	"retroarch": {
		"/Applications/RetroArch.app/Contents/MacOS/RetroArch",
		"retroarch",
	},
	"dolphin": {
		"/Applications/Dolphin.app/Contents/MacOS/Dolphin",
		"dolphin-emu", "Dolphin",
	},
	"pcsx2": {
		"/Applications/PCSX2.app/Contents/MacOS/PCSX2",
		"/Applications/PCSX2-Qt.app/Contents/MacOS/PCSX2-Qt",
		"pcsx2-qt", "PCSX2", "pcsx2",
	},
	"ppsspp": {
		"/Applications/PPSSPP.app/Contents/MacOS/PPSSPP",
		"/Applications/PPSSPPSDL.app/Contents/MacOS/PPSSPPSDL",
		"PPSSPPSDL", "ppsspp",
	},
	"duckstation": {
		"/Applications/DuckStation.app/Contents/MacOS/DuckStation",
		"duckstation-qt", "duckstation",
	},
}

// bundledAppPaths returns the .app binary paths to try inside the
// project's emulators dir for a given catalog key. The names follow the
// Homebrew cask layout so a `cp -R` from /opt/homebrew/Caskroom/<cask>
// into ./emulators/ just works.
func bundledAppPaths(emulatorsDir, key string) []string {
	if emulatorsDir == "" {
		return nil
	}
	join := func(parts ...string) string { return filepath.Join(append([]string{emulatorsDir}, parts...)...) }
	switch key {
	case "retroarch":
		return []string{join("RetroArch.app", "Contents", "MacOS", "RetroArch")}
	case "dolphin":
		return []string{join("Dolphin.app", "Contents", "MacOS", "Dolphin")}
	case "pcsx2":
		return []string{
			join("PCSX2.app", "Contents", "MacOS", "PCSX2"),
			join("PCSX2-Qt.app", "Contents", "MacOS", "PCSX2-Qt"),
		}
	case "ppsspp":
		return []string{
			join("PPSSPP.app", "Contents", "MacOS", "PPSSPP"),
			join("PPSSPPSDL.app", "Contents", "MacOS", "PPSSPPSDL"),
		}
	case "duckstation":
		return []string{join("DuckStation.app", "Contents", "MacOS", "DuckStation")}
	}
	return nil
}

// findStandalone tries the project-bundled .app bundles first, then
// the system-wide ones, then PATH. Returns "" if nothing exists.
func findStandalone(cfg Config, key string) string {
	for _, cand := range bundledAppPaths(cfg.EmulatorsDir, key) {
		if fileExists(cand) {
			return cand
		}
	}
	for _, cand := range systemStandalones[key] {
		if filepath.IsAbs(cand) {
			if fileExists(cand) {
				return cand
			}
			continue
		}
		if p, err := exec.LookPath(cand); err == nil {
			return p
		}
	}
	return ""
}

// standaloneArgs returns the launch args for a standalone emulator,
// putting the ROM where each one expects it.
func standaloneArgs(key, rom string) []string {
	switch key {
	case "dolphin":
		// -b: exit when the game stops, -e: boot the file immediately.
		return []string{"-b", "-e", rom}
	case "pcsx2":
		return []string{"-batch", "--", rom}
	case "duckstation":
		return []string{"-batch", "--", rom}
	default: // ppsspp and anything else take the ROM positionally
		return []string{rom}
	}
}

// -----------------------------------------------------------------------------
// libretro core discovery
// -----------------------------------------------------------------------------

// coreSearchDirs returns the directories to look for libretro cores in,
// most-specific first: an explicit override, the user's RetroArch data
// dir, then the app bundle's bundled cores.
func coreSearchDirs(cfg Config) []string {
	var dirs []string
	if cfg.RetroArchCores != "" {
		dirs = append(dirs, cfg.RetroArchCores)
	}
	home, _ := os.UserHomeDir()
	switch runtime.GOOS {
	case "darwin":
		if home != "" {
			dirs = append(dirs, filepath.Join(home, "Library", "Application Support", "RetroArch", "cores"))
		}
		dirs = append(dirs, "/Applications/RetroArch.app/Contents/Resources/cores")
	default: // linux & co.
		if home != "" {
			dirs = append(dirs,
				filepath.Join(home, ".config", "retroarch", "cores"),
				filepath.Join(home, ".local", "share", "libretro", "cores"),
			)
		}
		dirs = append(dirs, "/usr/lib/libretro", "/usr/local/lib/libretro")
	}
	return dirs
}

func coreExt() string {
	switch runtime.GOOS {
	case "darwin":
		return ".dylib"
	case "windows":
		return ".dll"
	default:
		return ".so"
	}
}

// resolveCorePath finds the on-disk path for a core name ("snes9x" →
// .../snes9x_libretro.dylib). Returns "" if not found in any search dir.
func resolveCorePath(cfg Config, core string) string {
	if filepath.IsAbs(core) && fileExists(core) {
		return core // binding gave a full path already
	}
	name := core
	if !strings.HasSuffix(name, "_libretro") {
		name += "_libretro"
	}
	file := name + coreExt()
	for _, dir := range coreSearchDirs(cfg) {
		if p := filepath.Join(dir, file); fileExists(p) {
			return p
		}
	}
	return ""
}

// -----------------------------------------------------------------------------
// Helpers
// -----------------------------------------------------------------------------

// expandArgs splits a binding's arg string and substitutes {rom}/{core}.
// If no {rom} placeholder is present the ROM is appended last.
func expandArgs(raw, rom, core string) []string {
	fields := strings.Fields(raw)
	var out []string
	hasROM := false
	for _, f := range fields {
		f = strings.ReplaceAll(f, "{rom}", rom)
		f = strings.ReplaceAll(f, "{core}", core)
		if strings.Contains(f, rom) {
			hasROM = true
		}
		out = append(out, f)
	}
	if !hasROM {
		out = append(out, rom)
	}
	return out
}

func finalize(command string, args []string, emu string) Resolved {
	return Resolved{
		Command:  command,
		Args:     args,
		Emulator: emu,
		Display:  command + " " + strings.Join(args, " "),
	}
}

func fileExists(p string) bool {
	info, err := os.Stat(p)
	return err == nil && !info.IsDir()
}
