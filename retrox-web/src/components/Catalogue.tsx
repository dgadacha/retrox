import { useQuery } from "@tanstack/react-query"
import { Gamepad2, Search } from "lucide-react"
import { useMemo, useState } from "react"
import { Link, useSearchParams } from "react-router-dom"

import { Button, Spinner, inputClass } from "@/components/ui"
import { api, catalogCover } from "@/lib/api"
import { usePlatforms } from "@/lib/hooks"
import type { CatalogRelease } from "@/lib/types"

// CataloguePage browses OpenVGDB's 53k releases as a grid with platform
// filter + free-text search + pagination. Click a card → the per-game
// detail at /catalogue/:id, where the user can pick a download source.
export function CataloguePage() {
  const [params, setParams] = useSearchParams()
  const platformId = params.get("platform") ?? ""
  const q = params.get("q") ?? ""
  const page = Number(params.get("page")) || 1

  const [searchInput, setSearchInput] = useState(q)

  function patch(updates: Record<string, string | null>) {
    const next = new URLSearchParams(params)
    for (const [k, v] of Object.entries(updates)) {
      if (v === null || v === "") next.delete(k)
      else next.set(k, v)
    }
    // Any filter change resets pagination.
    if (!("page" in updates)) next.delete("page")
    setParams(next, { replace: true })
  }

  const platformsQ = usePlatforms()
  const catalogQ = useQuery({
    queryKey: ["catalog", platformId, q, page],
    queryFn: () => api.catalog({ platform: platformId, q, page }),
    placeholderData: (prev) => prev,
  })

  return (
    <div className="flex flex-1 flex-col">
      <header className="sticky top-0 z-10 flex flex-wrap items-end justify-between gap-4 border-b border-ink-700 bg-ink-950/85 px-6 py-5 backdrop-blur lg:px-10">
        <div>
          <h1 className="text-2xl font-bold tracking-tight">Catalogue</h1>
          <p className="mt-1 text-xs text-text-500">
            Tous les jeux référencés dans OpenVGDB (jaquettes officielles, descriptions). Clique
            un titre pour voir les sources de téléchargement disponibles.
          </p>
        </div>
        <div className="flex flex-wrap items-center gap-2">
          <form
            className="flex gap-2"
            onSubmit={(e) => {
              e.preventDefault()
              patch({ q: searchInput })
            }}
          >
            <input
              type="search"
              placeholder="Rechercher un titre…"
              value={searchInput}
              onChange={(e) => setSearchInput(e.target.value)}
              className={`${inputClass} w-60`}
            />
            <Button type="submit" variant="primary" aria-label="Lancer la recherche">
              <Search className="h-4 w-4" strokeWidth={2} />
            </Button>
          </form>
          <select
            className={`${inputClass} w-auto`}
            value={platformId}
            onChange={(e) => patch({ platform: e.target.value })}
          >
            <option value="">Toutes les plateformes</option>
            {(platformsQ.data ?? [])
              .filter((p) => p.openvgdbId > 0)
              .map((p) => (
                <option key={p.id} value={p.id}>
                  {p.name}
                </option>
              ))}
          </select>
        </div>
      </header>

      <div className="flex-1 px-6 py-6 lg:px-10">
        <CatalogueResults
          page={page}
          onPageChange={(p) => patch({ page: String(p) })}
          data={catalogQ.data}
          isLoading={catalogQ.isLoading}
          isError={catalogQ.isError}
          error={catalogQ.error as Error | null}
        />
      </div>
    </div>
  )
}

function CatalogueResults({
  page,
  onPageChange,
  data,
  isLoading,
  isError,
  error,
}: {
  page: number
  onPageChange: (p: number) => void
  data?: { items: CatalogRelease[]; total: number; hasMore: boolean }
  isLoading: boolean
  isError: boolean
  error: Error | null
}) {
  const items = data?.items ?? []

  if (isLoading && !data) {
    return (
      <div className="grid h-40 place-items-center">
        <Spinner className="h-6 w-6" />
      </div>
    )
  }
  if (isError) {
    return (
      <p className="rounded-xl border border-red-500/30 bg-red-500/10 p-4 text-sm text-red-300">
        {error?.message ?? "Erreur"}
      </p>
    )
  }
  if (items.length === 0) {
    return (
      <p className="rounded-xl border border-dashed border-ink-700 py-12 text-center text-sm text-text-500">
        Aucun résultat. Si OpenVGDB n'est pas encore téléchargée, va dans Réglages.
      </p>
    )
  }

  return (
    <div className="space-y-6">
      <p className="text-xs text-text-500">
        {data!.total.toLocaleString("fr")} jeux · page {page}
      </p>

      <div className="grid grid-cols-2 gap-4 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5 xl:grid-cols-6 2xl:grid-cols-7">
        {items.map((r) => (
          <CatalogueCard key={r.releaseId} r={r} />
        ))}
      </div>

      <div className="flex items-center justify-center gap-3 pt-2">
        <Button
          variant="ghost"
          disabled={page <= 1}
          onClick={() => onPageChange(Math.max(1, page - 1))}
        >
          Précédent
        </Button>
        <span className="text-xs text-text-500">Page {page}</span>
        <Button variant="ghost" disabled={!data!.hasMore} onClick={() => onPageChange(page + 1)}>
          Suivant
        </Button>
      </div>
    </div>
  )
}

function CatalogueCard({ r }: { r: CatalogRelease }) {
  const [failed, setFailed] = useState(false)
  // The OpenVGDB row always carries a URL but it may 404 on gamefaqs —
  // fall back to a placeholder tile to keep the grid even.
  const showImage = !failed && !!r.coverUrl
  const platformLabel = useMemo(() => r.systemShortName ?? "", [r.systemShortName])

  return (
    <Link
      to={`/catalogue/${r.releaseId}`}
      title={r.title}
      className="group relative aspect-[3/4] overflow-hidden rounded-lg bg-ink-800 ring-1 ring-inset ring-white/5 outline-none transition duration-200 hover:-translate-y-1 hover:ring-accent-500/60 hover:shadow-[0_10px_30px_-10px_rgba(139,92,246,0.45)] focus-visible:ring-accent-500"
    >
      {showImage ? (
        <img
          src={catalogCover(r.releaseId)}
          alt={r.title}
          loading="lazy"
          onError={() => setFailed(true)}
          className="h-full w-full object-cover"
        />
      ) : (
        <div className="flex h-full w-full flex-col items-center justify-center gap-3 bg-gradient-to-br from-ink-700 via-ink-800 to-ink-900 p-4 text-center">
          <Gamepad2 className="h-10 w-10 text-text-700" strokeWidth={1.5} />
          <span className="line-clamp-3 text-xs font-semibold text-text-300">{r.title}</span>
          {platformLabel && (
            <span className="text-[10px] uppercase tracking-wider text-text-700">
              {platformLabel}
            </span>
          )}
        </div>
      )}
      <div className="pointer-events-none absolute inset-x-0 bottom-0 bg-gradient-to-t from-black/95 via-black/50 to-transparent p-3 pt-10 opacity-0 transition group-hover:opacity-100">
        <p className="line-clamp-2 text-left text-xs font-semibold text-text-100">{r.title}</p>
        {platformLabel && (
          <p className="text-left text-[10px] uppercase tracking-wider text-text-500">
            {platformLabel}
          </p>
        )}
      </div>
    </Link>
  )
}
