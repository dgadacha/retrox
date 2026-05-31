package scanner

import (
	"hash/crc32"
	"os"
	"path/filepath"
	"testing"
)

// helper: write `body` to a temp file under dir with name, return path.
func writeROM(t *testing.T, dir, name string, body []byte) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, body, 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// canonicalROM is the unheadered, big-endian body whose hash should
// match across every header / byteswap variant we test.
func canonicalROM() []byte {
	// 64 KiB of patterned bytes so swaps produce visibly different
	// CRCs from the canonical one (a constant fill would mask 2-byte
	// swap bugs).
	b := make([]byte, 64*1024)
	for i := range b {
		b[i] = byte(i ^ (i >> 8))
	}
	return b
}

func crcOf(b []byte) string {
	c := crc32.NewIEEE()
	_, _ = c.Write(b)
	return formatCRC(c.Sum32())
}

func formatCRC(v uint32) string {
	const hex = "0123456789ABCDEF"
	out := []byte{0, 0, 0, 0, 0, 0, 0, 0}
	for i := 7; i >= 0; i-- {
		out[i] = hex[v&0xF]
		v >>= 4
	}
	return string(out)
}

// TestHashFile_RawROM is the baseline: no header, no swap → CRC matches
// a direct crc32 of the bytes.
func TestHashFile_RawROM(t *testing.T) {
	dir := t.TempDir()
	body := canonicalROM()
	want := crcOf(body)

	p := writeROM(t, dir, "raw.bin", body)
	got, _, _, err := hashFile(p, "")
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("raw rom: got CRC %s, want %s", got, want)
	}
}

// TestHashFile_INES verifies the 16-byte iNES header is stripped.
func TestHashFile_INES(t *testing.T) {
	dir := t.TempDir()
	body := canonicalROM()
	want := crcOf(body)

	header := append([]byte{'N', 'E', 'S', 0x1A}, make([]byte, 12)...)
	full := append(header, body...)

	p := writeROM(t, dir, "rom.nes", full)
	got, _, _, err := hashFile(p, "nes")
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("iNES: got CRC %s, want %s (header should be stripped)", got, want)
	}
}

// TestHashFile_FDS verifies the FDS header is stripped.
func TestHashFile_FDS(t *testing.T) {
	dir := t.TempDir()
	body := canonicalROM()
	want := crcOf(body)

	header := append([]byte{'F', 'D', 'S', 0x1A}, make([]byte, 12)...)
	full := append(header, body...)

	p := writeROM(t, dir, "rom.fds", full)
	got, _, _, err := hashFile(p, "nes")
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("FDS: got CRC %s, want %s", got, want)
	}
}

// TestHashFile_SNES_SMCHeader verifies the 512-byte SMC copier header
// is stripped when fileSize % 1024 == 512 on the snes platform.
func TestHashFile_SNES_SMCHeader(t *testing.T) {
	dir := t.TempDir()
	body := canonicalROM()
	want := crcOf(body)

	header := make([]byte, 512) // garbage copier header
	for i := range header {
		header[i] = 0xAA
	}
	full := append(header, body...)
	if len(full)%1024 != 512 {
		t.Fatalf("test setup: SMC-headered file should be %%1024==512, got %d", len(full)%1024)
	}

	p := writeROM(t, dir, "rom.smc", full)
	got, _, _, err := hashFile(p, "snes")
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("SNES SMC header: got CRC %s, want %s", got, want)
	}
}

// TestHashFile_SNES_NoHeader verifies a headerless SNES ROM (file size
// multiple of 1024) is hashed as-is — no false-positive header strip.
func TestHashFile_SNES_NoHeader(t *testing.T) {
	dir := t.TempDir()
	body := canonicalROM() // exactly 64 KiB, %1024 == 0
	want := crcOf(body)

	p := writeROM(t, dir, "rom.sfc", body)
	got, _, _, err := hashFile(p, "snes")
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("SNES headerless: got CRC %s, want %s (should NOT strip)", got, want)
	}
}

// TestHashFile_N64_Z64 verifies the .z64 (canonical big-endian) variant
// hashes as-is. The first 4 bytes are the N64 magic 80 37 12 40.
func TestHashFile_N64_Z64(t *testing.T) {
	dir := t.TempDir()
	body := append([]byte{0x80, 0x37, 0x12, 0x40}, canonicalROM()...)
	want := crcOf(body)

	p := writeROM(t, dir, "rom.z64", body)
	got, _, _, err := hashFile(p, "n64")
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("N64 .z64: got %s, want %s", got, want)
	}
}

// TestHashFile_N64_V64 verifies a 2-byte-swapped (.v64) ROM produces
// the same hash as its .z64 sibling.
func TestHashFile_N64_V64(t *testing.T) {
	dir := t.TempDir()
	z64 := append([]byte{0x80, 0x37, 0x12, 0x40}, canonicalROM()...)
	want := crcOf(z64)

	v64 := make([]byte, len(z64))
	copy(v64, z64)
	for i := 0; i+1 < len(v64); i += 2 {
		v64[i], v64[i+1] = v64[i+1], v64[i]
	}
	if v64[0] != 0x37 || v64[1] != 0x80 {
		t.Fatalf("test setup: v64 head should be 37 80 …, got %02X %02X", v64[0], v64[1])
	}

	p := writeROM(t, dir, "rom.v64", v64)
	got, _, _, err := hashFile(p, "n64")
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("N64 .v64: got %s, want %s (should unswap to z64)", got, want)
	}
}

// TestHashFile_N64_N64 verifies a 4-byte-swapped (.n64) ROM produces
// the same hash as its .z64 sibling.
func TestHashFile_N64_N64(t *testing.T) {
	dir := t.TempDir()
	z64 := append([]byte{0x80, 0x37, 0x12, 0x40}, canonicalROM()...)
	want := crcOf(z64)

	n64 := make([]byte, len(z64))
	copy(n64, z64)
	for i := 0; i+3 < len(n64); i += 4 {
		n64[i], n64[i+1], n64[i+2], n64[i+3] = n64[i+3], n64[i+2], n64[i+1], n64[i]
	}
	if n64[0] != 0x40 || n64[1] != 0x12 {
		t.Fatalf("test setup: n64 head should be 40 12 …, got %02X %02X", n64[0], n64[1])
	}

	p := writeROM(t, dir, "rom.n64", n64)
	got, _, _, err := hashFile(p, "n64")
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("N64 .n64: got %s, want %s (should unswap to z64)", got, want)
	}
}

// TestHashFile_LYNX verifies the 64-byte LNX header is stripped.
func TestHashFile_LYNX(t *testing.T) {
	dir := t.TempDir()
	body := canonicalROM()
	want := crcOf(body)

	header := make([]byte, 64)
	copy(header, []byte("LYNX"))
	full := append(header, body...)

	p := writeROM(t, dir, "rom.lnx", full)
	got, _, _, err := hashFile(p, "lynx")
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("Lynx LNX: got %s, want %s", got, want)
	}
}

// TestHashFile_A78 verifies the 128-byte A78 header is stripped.
// A78 magic lives at offset 1: "ATARI7800".
func TestHashFile_A78(t *testing.T) {
	dir := t.TempDir()
	body := canonicalROM()
	want := crcOf(body)

	header := make([]byte, 128)
	copy(header[1:], []byte("ATARI7800"))
	full := append(header, body...)

	p := writeROM(t, dir, "rom.a78", full)
	got, _, _, err := hashFile(p, "atari7800")
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("A78: got %s, want %s", got, want)
	}
}
