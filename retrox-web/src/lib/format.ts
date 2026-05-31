export function formatBytes(n: number): string {
  if (!n || n <= 0) return "—"
  const units = ["o", "Ko", "Mo", "Go", "To"]
  let v = n
  let i = 0
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024
    i++
  }
  return `${v.toFixed(i === 0 || v >= 100 ? 0 : 1)} ${units[i]}`
}

// year extracts a 4-digit year from a free-form release-date string
// (OpenVGDB stores things like "July 1993", "Mar 22, 1996", "1996").
export function year(releaseDate: string): string {
  const m = releaseDate?.match(/\d{4}/)
  return m ? m[0] : ""
}
