import { useQuery } from "@tanstack/react-query"
import { ArrowLeft, Gamepad2, Search } from "lucide-react"
import { useState } from "react"
import { Link, useSearchParams } from "react-router-dom"

import { Button, inputClass } from "@/components/ui"
import { api, catalogCover } from "@/lib/api"
import type { CatalogPlatform, CatalogRelease } from "@/lib/types"

// CataloguePage has two modes driven by ?platform:
//   - empty   → platform picker (grid of manufacturer-coloured tiles)
//   - present → game grid for that platform, with a back-to-picker link
//
// The dedicated picker replaces what used to be a dropdown so the user
// always lands on a visual menu of consoles instead of an opaque list.
export function CataloguePage() {
  const [params, setParams] = useSearchParams()
  const platformId = params.get("platform") ?? ""

  if (!platformId) return <PlatformPicker />
  return (
    <GamesView
      platformId={platformId}
      params={params}
      onPatch={(updates) => {
        const next = new URLSearchParams(params)
        for (const [k, v] of Object.entries(updates)) {
          if (v === null || v === "") next.delete(k)
          else next.set(k, v)
        }
        if (!("page" in updates)) next.delete("page")
        setParams(next, { replace: true })
      }}
    />
  )
}

// -----------------------------------------------------------------------------
// Platform picker
// -----------------------------------------------------------------------------

// platformAccent maps each catalog id to a discreet manufacturer accent
// shown as a thin top bar — keeps a hint of brand colour without making
// the whole card scream. New platforms fall back to the violet accent.
const platformAccent: Record<string, string> = {
  nes: "bg-red-600",
  snes: "bg-violet-600",
  n64: "bg-emerald-500",
  gb: "bg-stone-500",
  gbc: "bg-fuchsia-500",
  gba: "bg-purple-500",
  nds: "bg-sky-500",
  gamecube: "bg-indigo-500",
  wii: "bg-zinc-300",
  mastersystem: "bg-blue-500",
  megadrive: "bg-cyan-500",
  gamegear: "bg-cyan-400",
  sega32x: "bg-rose-500",
  saturn: "bg-slate-400",
  dreamcast: "bg-orange-500",
  psx: "bg-zinc-400",
  ps2: "bg-blue-500",
  psp: "bg-zinc-500",
  pcengine: "bg-orange-500",
  neogeo: "bg-yellow-500",
  ngp: "bg-orange-400",
  atari2600: "bg-amber-600",
  atari7800: "bg-red-500",
  lynx: "bg-yellow-400",
  wonderswan: "bg-sky-500",
  arcade: "bg-fuchsia-500",
}

function PlatformPicker() {
  const platsQ = useQuery({
    queryKey: ["catalog-platforms"],
    queryFn: api.catalogPlatforms,
  })

  return (
    <div className="flex flex-1 flex-col">
      <header className="sticky top-0 z-10 border-b border-ink-700 bg-ink-950/85 px-6 py-5 backdrop-blur lg:px-10">
        <h1 className="text-2xl font-bold tracking-tight">Catalogue</h1>
        <p className="mt-1 text-xs text-text-500">
          Choisis une plateforme pour parcourir les jeux disponibles.
        </p>
      </header>

      <div className="flex-1 px-6 py-8 lg:px-10">
        {platsQ.isLoading && (
          <div className="grid grid-cols-2 gap-4 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5 xl:grid-cols-6">
            {Array.from({ length: 18 }).map((_, i) => (
              <PlatformTileSkeleton key={i} />
            ))}
          </div>
        )}
        {platsQ.isError && (
          <p className="rounded-xl border border-red-500/30 bg-red-500/10 p-4 text-sm text-red-300">
            {(platsQ.error as Error).message}
          </p>
        )}
        {platsQ.data && (
          <div className="grid grid-cols-2 gap-4 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5 xl:grid-cols-6">
            {platsQ.data.map((p) => (
              <PlatformTile key={p.id} platform={p} />
            ))}
          </div>
        )}
      </div>
    </div>
  )
}

// Skeleton placeholder that mirrors the PlatformTile layout so the
// page doesn't reflow when real data lands.
function PlatformTileSkeleton() {
  return (
    <div className="flex animate-pulse flex-col overflow-hidden rounded-xl bg-ink-800 ring-1 ring-inset ring-white/5">
      <span className="block h-0.5 w-full bg-ink-700" />
      <div className="h-32 bg-zinc-200/5" />
      <div className="flex items-center justify-between gap-2 px-4 py-3">
        <div className="h-3 w-24 rounded bg-ink-700" />
        <div className="h-3 w-10 rounded bg-ink-700" />
      </div>
    </div>
  )
}

function PlatformTile({ platform }: { platform: CatalogPlatform }) {
  const accent = platformAccent[platform.id] ?? "bg-accent-500"
  const [logoFailed, setLogoFailed] = useState(false)
  // Logos live under public/static/consoles/ so they're served by the
  // existing /static/* Go route in prod. Missing files (saturn, arcade)
  // 404 → onError → Gamepad2 fallback.
  const logoSrc = `/static/consoles/${platform.id}.svg`

  return (
    <Link
      to={`/catalogue?platform=${encodeURIComponent(platform.id)}`}
      className="group flex flex-col overflow-hidden rounded-xl bg-ink-800 ring-1 ring-inset ring-white/5 outline-none transition hover:-translate-y-1 hover:ring-accent-500/60 hover:shadow-[0_18px_40px_-15px_rgba(0,0,0,0.6)] focus-visible:ring-accent-500"
    >
      <span className={`block h-0.5 w-full ${accent}`} />
      <div className="flex h-32 items-center justify-center bg-zinc-100 px-6">
        {!logoFailed ? (
          <img
            src={logoSrc}
            alt={platform.name}
            onError={() => setLogoFailed(true)}
            className="max-h-16 w-auto max-w-full object-contain transition group-hover:scale-105"
          />
        ) : (
          <Gamepad2 className="h-12 w-12 text-zinc-400" strokeWidth={1.5} />
        )}
      </div>
      <div className="flex items-center justify-between gap-2 px-4 py-3">
        <p className="line-clamp-1 text-sm font-semibold text-text-100">
          {platform.name}
        </p>
        <p className="shrink-0 text-[11px] font-semibold uppercase tracking-wider text-text-500">
          {platform.count.toLocaleString("fr")}
        </p>
      </div>
    </Link>
  )
}

// -----------------------------------------------------------------------------
// Games grid (platform selected)
// -----------------------------------------------------------------------------

function GamesView({
  platformId,
  params,
  onPatch,
}: {
  platformId: string
  params: URLSearchParams
  onPatch: (updates: Record<string, string | null>) => void
}) {
  const q = params.get("q") ?? ""
  const page = Number(params.get("page")) || 1
  const [searchInput, setSearchInput] = useState(q)

  const platsQ = useQuery({
    queryKey: ["catalog-platforms"],
    queryFn: api.catalogPlatforms,
  })
  const platform = platsQ.data?.find((p) => p.id === platformId)
  const platformName = platform?.name ?? platformId.toUpperCase()

  const catalogQ = useQuery({
    queryKey: ["catalog", platformId, q, page],
    queryFn: () => api.catalog({ platform: platformId, q, page }),
    placeholderData: (prev) => prev,
  })

  return (
    <div className="flex flex-1 flex-col">
      <header className="sticky top-0 z-10 flex flex-wrap items-end justify-between gap-4 border-b border-ink-700 bg-ink-950/85 px-6 py-5 backdrop-blur lg:px-10">
        <div>
          <Link
            to="/catalogue"
            className="mb-2 inline-flex items-center gap-1.5 text-xs uppercase tracking-wider text-accent-300 hover:text-accent-400"
          >
            <ArrowLeft className="h-3.5 w-3.5" strokeWidth={2.5} />
            Toutes les plateformes
          </Link>
          <h1 className="text-2xl font-bold tracking-tight">{platformName}</h1>
          <p className="mt-1 text-xs text-text-500">
            {catalogQ.data
              ? `${catalogQ.data.total.toLocaleString("fr")} jeux`
              : "—"}
          </p>
        </div>
        <form
          className="flex gap-2"
          onSubmit={(e) => {
            e.preventDefault()
            onPatch({ q: searchInput })
          }}
        >
          <input
            type="search"
            placeholder="Rechercher un titre…"
            value={searchInput}
            onChange={(e) => setSearchInput(e.target.value)}
            className={`${inputClass} w-60`}
          />
          <Button type="submit" variant="primary" aria-label="Rechercher">
            <Search className="h-4 w-4" strokeWidth={2} />
          </Button>
        </form>
      </header>

      <div className="flex-1 px-6 py-6 lg:px-10">
        <CatalogueResults
          page={page}
          onPageChange={(p) => onPatch({ page: String(p) })}
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
      <div className="grid grid-cols-2 gap-4 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5 xl:grid-cols-6 2xl:grid-cols-7">
        {Array.from({ length: 24 }).map((_, i) => (
          <GameCardSkeleton key={i} />
        ))}
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
        Aucun résultat.
      </p>
    )
  }

  return (
    <div className="space-y-6">
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

function GameCardSkeleton() {
  return (
    <div className="aspect-[3/4] animate-pulse overflow-hidden rounded-lg bg-ink-800 ring-1 ring-inset ring-white/5" />
  )
}

function CatalogueCard({ r }: { r: CatalogRelease }) {
  const [failed, setFailed] = useState(false)
  const showImage = !failed && !!r.coverUrl

  return (
    <Link
      to={`/catalogue/${encodeURIComponent(r.releaseId)}`}
      title={r.title}
      className="group relative aspect-[3/4] overflow-hidden rounded-lg bg-ink-800 ring-1 ring-inset ring-white/5 outline-none transition duration-200 hover:-translate-y-1 hover:ring-accent-500/60 hover:shadow-[0_10px_30px_-10px_rgba(139,92,246,0.45)] focus-visible:ring-accent-500"
    >
      {showImage ? (
        <img
          src={catalogCover(r.releaseId, r.platformId)}
          alt={r.title}
          loading="lazy"
          onError={() => setFailed(true)}
          className="h-full w-full object-cover"
        />
      ) : (
        <div className="flex h-full w-full flex-col items-center justify-center gap-3 bg-gradient-to-br from-ink-700 via-ink-800 to-ink-900 p-4 text-center">
          <Gamepad2 className="h-10 w-10 text-text-700" strokeWidth={1.5} />
          <span className="line-clamp-3 text-xs font-semibold text-text-300">{r.title}</span>
        </div>
      )}
      {r.variantCount && r.variantCount > 1 && (
        <span className="absolute right-2 top-2 rounded-md bg-black/70 px-1.5 py-0.5 text-[10px] font-semibold text-text-100 backdrop-blur">
          {r.variantCount}×
        </span>
      )}
      <div className="pointer-events-none absolute inset-x-0 bottom-0 bg-gradient-to-t from-black/95 via-black/50 to-transparent p-3 pt-10 opacity-0 transition group-hover:opacity-100">
        <p className="line-clamp-2 text-left text-xs font-semibold text-text-100">{r.title}</p>
      </div>
    </Link>
  )
}
