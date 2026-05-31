// Types mirror the JSON the Go backend emits (see internal/database/models
// and internal/handlers). The API envelope is { data } / { error }; these
// describe the unwrapped `data` payloads.

export interface Game {
  id: number
  createdAt: string
  updatedAt: string
  path: string
  fileName: string
  fileSize: number
  crc: string
  md5: string
  sha1: string
  platformId: string
  title: string
  description: string
  genre: string
  developer: string
  publisher: string
  releaseDate: string
  region: string
  coverUrl: string
  screenshotUrl: string
  scraped: boolean
  missing: boolean
}

export interface Platform {
  id: string
  name: string
  openvgdbId: number
  libretroThumbsName?: string
  exts: string[]
  core: string
  standalone?: string
}

export interface PlayHistory {
  id: number
  profileUid: string
  gameId: number
  playCount: number
  title: string
  platformId: string
  coverUrl: string
}

export interface Favorite {
  id: number
  profileUid: string
  gameId: number
  title: string
  platformId: string
  coverUrl: string
}

export type DownloadStatus = "queued" | "downloading" | "done" | "error" | "canceled"

export interface Download {
  id: number
  url: string
  destPath: string
  platformId: string
  title: string
  status: DownloadStatus
  progress: number // 0..1
  bytesDone: number
  bytesTotal: number
  error: string
}

export interface EmulatorBinding {
  platformId: string
  command: string
  args: string
  core: string
  updatedAt: string
}

export interface EmulatorView {
  platformId: string
  name: string
  defaultCore?: string
  defaultStandalone?: string
  override?: EmulatorBinding
}

export interface Status {
  app: string
  version: string
  metadataReady: boolean
  openvgdbReady: boolean
  openvgdbRoms: number
  openvgdbReleases: number
  openvgdbPath: string
  datadir: string
  romDirs: string[]
  games: number
  gamesScraped: number
  scanning: boolean
  defaultProfileUid: string
}

export interface ScanProgress {
  running: boolean
  startedAt?: string
  total: number
  current: number
  currentFile?: string
  scraped: number
  unscraped: number
}

export interface Settings {
  romDirs: string[]
  retroarchBin: string
  retroarchCores: string
  openvgdbPath: string
}

export interface PlayResolved {
  command: string
  args: string[]
  display: string
  emulator: string
}

export interface OpenVGDBDownloadResult {
  ready: boolean
  roms: number
  releases: number
  path: string
}

export interface SourceInfo {
  id: string
  name: string
  description: string
  downloadable: boolean
  supportedPlatforms: string[]
}

export interface SourceROM {
  sourceId: string
  id: string
  title: string
  platformId: string
  description?: string
  coverUrl?: string
  sizeBytes?: number
  downloadable: boolean
  externalUrl: string
}

export interface CatalogRelease {
  releaseId: number
  title: string
  coverUrl?: string
  openvgdbSystemId: number
  systemShortName?: string
  region?: string
  platformId: string
  variantCount?: number
}

export interface CatalogPlatform {
  id: string
  name: string
  openvgdbId: number
  count: number
}

export interface CatalogReleaseDetail extends CatalogRelease {
  description?: string
  genre?: string
  developer?: string
  publisher?: string
  releaseDate?: string
  backCoverUrl?: string
}

export interface CatalogPage {
  items: CatalogRelease[]
  total: number
  page: number
  hasMore: boolean
}

export interface CatalogSourceGroup {
  source: SourceInfo
  items: SourceROM[]
  error?: string
}
