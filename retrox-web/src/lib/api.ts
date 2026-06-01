import type {
  CatalogPage,
  CatalogPlatform,
  CatalogReleaseDetail,
  CatalogSourceGroup,
  Download,
  EmulatorView,
  Favorite,
  Game,
  OpenVGDBDownloadResult,
  PlayHistory,
  PlayResolved,
  Platform,
  ScanProgress,
  Settings,
  SourceInfo,
  Status,
} from "@/lib/types"

const BASE = "/api/v1"

// req unwraps the { data } / { error } envelope and throws the server's
// French error message on a non-2xx response so callers can surface it.
async function req<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(BASE + path, {
    headers: init?.body ? { "Content-Type": "application/json" } : undefined,
    ...init,
  })
  let body: any
  const text = await res.text()
  if (text) {
    try {
      body = JSON.parse(text)
    } catch {
      body = undefined
    }
  }
  if (!res.ok) throw new Error(body?.error || `Erreur ${res.status}`)
  return (body?.data ?? body) as T
}

export interface UpdateSettingsInput {
  romDirs: string[]
  retroarchBin: string
  retroarchCores: string
}

export const api = {
  status: () => req<Status>("/status"),
  platforms: () => req<Platform[]>("/platforms"),

  games: (platform?: string) =>
    req<Game[]>("/games" + (platform ? `?platform=${encodeURIComponent(platform)}` : "")),
  game: (id: number) => req<Game>(`/games/${id}`),
  play: (id: number) => req<PlayResolved>(`/games/${id}/play`, { method: "POST" }),

  scan: () => req<{ started: boolean }>("/library/scan", { method: "POST" }),
  scanStatus: () => req<ScanProgress>("/library/scan/status"),

  downloads: () => req<Download[]>("/downloads"),
  createDownload: (input: { url: string; platformId: string; title: string }) =>
    req<Download>("/downloads", { method: "POST", body: JSON.stringify(input) }),
  cancelDownload: (id: number) => req<unknown>(`/downloads/${id}`, { method: "DELETE" }),

  emulators: () => req<EmulatorView[]>("/emulators"),
  setEmulator: (platformId: string, input: { command: string; args: string; core: string }) =>
    req<unknown>(`/emulators/${encodeURIComponent(platformId)}`, {
      method: "PUT",
      body: JSON.stringify(input),
    }),
  deleteEmulator: (platformId: string) =>
    req<unknown>(`/emulators/${encodeURIComponent(platformId)}`, { method: "DELETE" }),

  history: (uid: string) => req<PlayHistory[]>(`/profiles/${encodeURIComponent(uid)}/history`),
  favorites: (uid: string) => req<Favorite[]>(`/profiles/${encodeURIComponent(uid)}/favorites`),
  addFavorite: (uid: string, gameId: number) =>
    req<unknown>(`/profiles/${encodeURIComponent(uid)}/favorites`, {
      method: "POST",
      body: JSON.stringify({ gameId }),
    }),
  removeFavorite: (uid: string, gameId: number) =>
    req<unknown>(`/profiles/${encodeURIComponent(uid)}/favorites/${gameId}`, { method: "DELETE" }),

  settings: () => req<Settings>("/settings"),
  updateSettings: (input: UpdateSettingsInput) =>
    req<Settings>("/settings", { method: "PUT", body: JSON.stringify(input) }),

  downloadOpenVGDB: () =>
    req<OpenVGDBDownloadResult>("/metadata/openvgdb/download", { method: "POST" }),

  sources: () => req<SourceInfo[]>("/sources"),
  sourceDownload: (id: string, input: { romId: string; platformId: string; title: string }) =>
    req<Download>(`/sources/${encodeURIComponent(id)}/download`, {
      method: "POST",
      body: JSON.stringify(input),
    }),

  catalog: (params: { platform?: string; q?: string; page?: number }) => {
    const qs = new URLSearchParams()
    if (params.platform) qs.set("platform", params.platform)
    if (params.q) qs.set("q", params.q)
    if (params.page && params.page > 1) qs.set("page", String(params.page))
    const suffix = qs.toString() ? `?${qs}` : ""
    return req<CatalogPage>(`/catalog${suffix}`)
  },
  catalogPlatforms: () => req<CatalogPlatform[]>("/catalog/platforms"),
  catalogGet: (id: string) => req<CatalogReleaseDetail>(`/catalog/${encodeURIComponent(id)}`),
  catalogSources: (id: string) =>
    req<CatalogSourceGroup[]>(`/catalog/${encodeURIComponent(id)}/sources`),

  setIGDBCredentials: (input: { clientId: string; clientSecret: string }) =>
    req<Settings>("/metadata/igdb/credentials", {
      method: "PUT",
      body: JSON.stringify(input),
    }),
  setTGDBKey: (input: { key: string }) =>
    req<Settings>("/metadata/tgdb/key", {
      method: "PUT",
      body: JSON.stringify(input),
    }),
  setRAWGKey: (input: { key: string }) =>
    req<Settings>("/metadata/rawg/key", {
      method: "PUT",
      body: JSON.stringify(input),
    }),
  setMetadataPreference: (input: { preference: string }) =>
    req<Settings>("/metadata/preference", {
      method: "PUT",
      body: JSON.stringify(input),
    }),
}

// catalogCover builds the proxy URL for a release's box art. Pass the
// platformId so the backend can try libretro-thumbnails (proper box
// scans) before falling back to the source's own image (which for
// RAWG is a gameplay screenshot, not a cover).
export function catalogCover(releaseId: string, platformId?: string): string {
  const suffix = platformId ? `?platform=${encodeURIComponent(platformId)}` : ""
  return `${BASE}/catalog/${encodeURIComponent(releaseId)}/cover${suffix}`
}

export type ImageKind = "cover" | "screenshot"

// gameImage points an <img> at the backend media proxy (which disk-caches
// the upstream OpenVGDB / libretro-thumbnails fetches).
export function gameImage(id: number, kind: ImageKind): string {
  return `${BASE}/games/${id}/image/${kind}`
}
