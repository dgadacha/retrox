import { LayoutGrid, List as ListIcon, PackageOpen } from "lucide-react"
import { useMemo } from "react"
import { useSearchParams } from "react-router-dom"

import { GameCard, GameRow } from "@/components/GameCard"
import { Button, Spinner, inputClass } from "@/components/ui"
import {
  useFavorites,
  useGames,
  useHistory,
  usePlatforms,
  useScanControl,
} from "@/lib/hooks"
import type { Game } from "@/lib/types"

type SortKey = "alpha" | "recent"
type ViewMode = "grid" | "list"

export function LibraryPage() {
  const [params, setParams] = useSearchParams()
  const platformId = params.get("platform") ?? ""
  const filter = params.get("filter") ?? ""
  const q = params.get("q") ?? ""
  const sort = (params.get("sort") as SortKey) || "alpha"
  const view = (params.get("view") as ViewMode) || "grid"

  const gamesQ = useGames()
  const platformsQ = usePlatforms()
  const favoritesQ = useFavorites()
  const historyQ = useHistory()

  const allGames = gamesQ.data ?? []
  const platformsById = useMemo(() => {
    const map = new Map<string, string>()
    for (const p of platformsQ.data ?? []) map.set(p.id, p.name)
    return map
  }, [platformsQ.data])

  const { title, games } = useMemo(() => {
    const base = filterGames(allGames, { platformId, filter, favorites: favoritesQ.data, history: historyQ.data })
    const searched = q ? base.filter((g) => g.title.toLowerCase().includes(q.toLowerCase())) : base
    const sorted = sortGames(searched, sort, historyQ.data)
    const title = pageTitle({ platformId, filter, platformName: platformsById.get(platformId) ?? "" })
    return { title, games: sorted }
  }, [allGames, platformId, filter, q, sort, favoritesQ.data, historyQ.data, platformsById])

  function patch(updates: Record<string, string | null>) {
    const next = new URLSearchParams(params)
    for (const [k, v] of Object.entries(updates)) {
      if (v === null || v === "") next.delete(k)
      else next.set(k, v)
    }
    setParams(next, { replace: true })
  }

  if (gamesQ.isLoading) {
    return (
      <div className="grid flex-1 place-items-center">
        <Spinner className="h-8 w-8" />
      </div>
    )
  }

  const isEmpty = allGames.length === 0
  if (isEmpty) return <EmptyLibrary />

  return (
    <div className="flex flex-1 flex-col">
      <header className="sticky top-0 z-10 flex flex-wrap items-end justify-between gap-4 border-b border-ink-700 bg-ink-950/85 px-6 py-5 backdrop-blur lg:px-10">
        <div>
          <h1 className="text-2xl font-bold tracking-tight">{title}</h1>
          <p className="mt-1 text-xs text-text-500">
            {games.length} {games.length > 1 ? "jeux" : "jeu"}
            {q && ` correspondant à « ${q} »`}
          </p>
        </div>

        <div className="flex flex-wrap items-center gap-2">
          <input
            type="search"
            placeholder="Rechercher…"
            value={q}
            onChange={(e) => patch({ q: e.target.value })}
            className={`${inputClass} w-52`}
          />
          <select
            className={`${inputClass} w-auto`}
            value={sort}
            onChange={(e) => patch({ sort: e.target.value })}
          >
            <option value="alpha">A → Z</option>
            <option value="recent">Récemment joués</option>
          </select>
          <div className="flex overflow-hidden rounded-md border border-ink-600">
            <ViewToggle current={view} mode="grid" onChange={(m) => patch({ view: m })} />
            <ViewToggle current={view} mode="list" onChange={(m) => patch({ view: m })} />
          </div>
        </div>
      </header>

      <div className="flex-1 px-6 py-6 lg:px-10">
        {games.length === 0 ? (
          <p className="py-16 text-center text-sm text-text-500">Aucun jeu pour ce filtre.</p>
        ) : view === "grid" ? (
          <div className="grid grid-cols-2 gap-4 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5 xl:grid-cols-6 2xl:grid-cols-7">
            {games.map((g) => (
              <GameCard
                key={g.id}
                id={g.id}
                title={g.title || g.fileName}
                platformName={platformsById.get(g.platformId) ?? ""}
              />
            ))}
          </div>
        ) : (
          <div className="space-y-1">
            {games.map((g) => (
              <GameRow
                key={g.id}
                id={g.id}
                title={g.title || g.fileName}
                platformName={platformsById.get(g.platformId) ?? ""}
              />
            ))}
          </div>
        )}
      </div>
    </div>
  )
}

function ViewToggle({
  current,
  mode,
  onChange,
}: {
  current: ViewMode
  mode: ViewMode
  onChange: (m: ViewMode) => void
}) {
  const active = current === mode
  const Icon = mode === "grid" ? LayoutGrid : ListIcon
  return (
    <button
      type="button"
      onClick={() => onChange(mode)}
      aria-pressed={active}
      aria-label={mode === "grid" ? "Vue grille" : "Vue liste"}
      className={`px-3 py-2 transition ${
        active ? "bg-accent-500/20 text-accent-300" : "text-text-500 hover:text-text-100"
      }`}
    >
      <Icon className="h-4 w-4" strokeWidth={2} />
    </button>
  )
}

function pageTitle({
  platformId,
  filter,
  platformName,
}: {
  platformId: string
  filter: string
  platformName: string
}) {
  if (platformId === "__none__") return "Non classé"
  if (platformId) return platformName || platformId
  if (filter === "favorites") return "Favoris"
  if (filter === "recent") return "Récemment joués"
  return "Bibliothèque"
}

function filterGames(
  all: Game[],
  opts: {
    platformId: string
    filter: string
    favorites?: { gameId: number }[]
    history?: { gameId: number }[]
  },
): Game[] {
  if (opts.platformId === "__none__") return all.filter((g) => !g.platformId)
  if (opts.platformId) return all.filter((g) => g.platformId === opts.platformId)
  if (opts.filter === "favorites") {
    const ids = new Set((opts.favorites ?? []).map((f) => f.gameId))
    return all.filter((g) => ids.has(g.id))
  }
  if (opts.filter === "recent") {
    const ids = new Set((opts.history ?? []).map((h) => h.gameId))
    return all.filter((g) => ids.has(g.id))
  }
  return all
}

function sortGames(games: Game[], key: SortKey, history?: { gameId: number; updatedAt?: string }[]): Game[] {
  const list = [...games]
  if (key === "alpha") {
    list.sort((a, b) => (a.title || a.fileName).localeCompare(b.title || b.fileName, "fr"))
  } else if (key === "recent") {
    const idx = new Map<number, number>()
    ;(history ?? []).forEach((h, i) => idx.set(h.gameId, i))
    list.sort((a, b) => (idx.get(a.id) ?? Infinity) - (idx.get(b.id) ?? Infinity))
  }
  return list
}

function EmptyLibrary() {
  const { running, progress, startScan, starting, error } = useScanControl()
  const pct = progress && progress.total > 0 ? Math.round((progress.current / progress.total) * 100) : 0

  return (
    <div className="grid flex-1 place-items-center px-6">
      <div className="max-w-md space-y-4 rounded-2xl border border-ink-700 bg-ink-900 p-8 text-center">
        <span className="mx-auto inline-flex h-16 w-16 items-center justify-center rounded-full bg-accent-500/10">
          <PackageOpen className="h-8 w-8 text-accent-300" strokeWidth={1.75} />
        </span>
        <h2 className="text-xl font-semibold">Bibliothèque vide</h2>
        <p className="text-sm text-text-300">
          Ajoutez vos ROMs dans les dossiers configurés sous <strong>Réglages</strong>, téléchargez
          la base OpenVGDB, puis lancez une analyse pour les indexer.
        </p>
        <Button variant="primary" onClick={startScan} disabled={running || starting} size="lg">
          {running || starting ? <Spinner className="h-4 w-4" /> : null}
          {running ? "Analyse en cours…" : "Lancer une analyse"}
        </Button>
        {running && progress && (
          <div>
            <div className="h-1.5 w-full overflow-hidden rounded-full bg-ink-700">
              <div
                className="h-full bg-accent-gradient transition-all"
                style={{ width: progress.total > 0 ? `${pct}%` : "12%" }}
              />
            </div>
            <p className="mt-2 truncate text-xs text-text-500">
              {progress.current}/{progress.total} — {progress.currentFile}
            </p>
          </div>
        )}
        {error && <p className="text-sm text-red-400">{error.message}</p>}
      </div>
    </div>
  )
}
