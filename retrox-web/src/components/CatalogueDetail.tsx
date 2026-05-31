import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { ArrowLeft, Download as DownloadIcon, ExternalLink, Gamepad2 } from "lucide-react"
import { useState } from "react"
import { Link, useNavigate, useParams } from "react-router-dom"

import { Badge, Button, Spinner } from "@/components/ui"
import { api, catalogCover } from "@/lib/api"
import { formatBytes, year } from "@/lib/format"
import { qk } from "@/lib/hooks"
import type { CatalogReleaseDetail, CatalogSourceGroup, SourceROM } from "@/lib/types"

// CatalogueDetailPage is the per-game page reached from /catalogue/:id.
// It shows the OpenVGDB metadata (official cover, synopsis, dev/publisher,
// region) and a "Télécharger" button that lazily fetches the candidates
// from every registered source — Internet Archive, PDRoms, … — so the
// user picks the variant they trust instead of browsing raw catalogues.
export function CatalogueDetailPage() {
  const { id: idParam } = useParams()
  const navigate = useNavigate()
  const id = idParam ?? ""

  const detailQ = useQuery({
    queryKey: ["catalog-detail", id],
    queryFn: () => api.catalogGet(id),
    enabled: id !== "",
  })
  const [showSources, setShowSources] = useState(false)
  const sourcesQ = useQuery({
    queryKey: ["catalog-sources", id],
    queryFn: () => api.catalogSources(id),
    enabled: showSources && id !== "",
  })

  if (detailQ.isLoading) {
    return (
      <div className="grid flex-1 place-items-center">
        <Spinner className="h-7 w-7" />
      </div>
    )
  }
  if (detailQ.isError || !detailQ.data) {
    return (
      <div className="grid flex-1 place-items-center text-sm text-red-400">
        Jeu introuvable —{" "}
        <button onClick={() => navigate("/catalogue")} className="text-accent-400 hover:underline">
          retour au catalogue
        </button>
      </div>
    )
  }

  const r = detailQ.data

  return (
    <div className="flex flex-1 flex-col">
      <Hero release={r} />

      <div className="-mt-24 flex flex-1 flex-col px-6 pb-10 lg:px-10">
        <div className="relative z-10 flex flex-col gap-6 sm:flex-row sm:items-end">
          <Cover release={r} />
          <div className="flex-1 space-y-3">
            <Link
              to={`/catalogue?platform=${encodeURIComponent(r.platformId)}`}
              className="inline-block text-xs uppercase tracking-wider text-accent-300 hover:text-accent-400"
            >
              {r.systemShortName || r.platformId || "Plateforme inconnue"}
            </Link>
            <h1 className="text-3xl font-black tracking-tight text-text-100 sm:text-4xl">
              {r.title}
            </h1>
            <div className="flex flex-wrap items-center gap-3 text-sm text-text-300">
              {year(r.releaseDate ?? "") && <span>{year(r.releaseDate!)}</span>}
              {r.region && <Badge>{r.region}</Badge>}
              {!r.platformId && (
                <Badge tone="warn">
                  Cette plateforme n'est pas supportée par RETROX — téléchargement désactivé
                </Badge>
              )}
            </div>
          </div>
          <div className="flex flex-wrap items-center gap-3 sm:self-center">
            <Button
              variant="success"
              size="lg"
              disabled={!r.platformId}
              onClick={() => setShowSources(true)}
            >
              <DownloadIcon className="h-4 w-4" strokeWidth={2.5} />
              Télécharger
            </Button>
          </div>
        </div>

        <div className="grid gap-8 pt-8 lg:grid-cols-[1fr_280px]">
          <div className="space-y-6">
            {showSources && (
              <SourcesPanel
                releaseId={r.releaseId}
                platformId={r.platformId}
                title={r.title}
                groups={sourcesQ.data}
                isLoading={sourcesQ.isLoading}
                isError={sourcesQ.isError}
                error={sourcesQ.error as Error | null}
              />
            )}

            {r.description ? (
              <p className="whitespace-pre-line text-[15px] leading-relaxed text-text-300">
                {r.description}
              </p>
            ) : (
              <p className="text-sm italic text-text-500">
                Aucune description disponible dans OpenVGDB pour ce jeu.
              </p>
            )}
          </div>

          <aside className="space-y-4 self-start rounded-xl border border-ink-700 bg-ink-900/60 p-5">
            <DetailRow label="Genre" value={r.genre} />
            <DetailRow label="Développeur" value={r.developer} />
            <DetailRow label="Éditeur" value={r.publisher} />
            <DetailRow label="Sortie" value={r.releaseDate} />
            <DetailRow label="Région" value={r.region} />
            <DetailRow label="Plateforme" value={r.systemShortName} />
          </aside>
        </div>
      </div>
    </div>
  )
}

function Hero({ release }: { release: CatalogReleaseDetail }) {
  const [failed, setFailed] = useState(false)
  return (
    <div className="relative h-72 w-full overflow-hidden bg-ink-900 sm:h-96">
      {release.coverUrl && !failed ? (
        <img
          src={catalogCover(release.releaseId)}
          alt=""
          onError={() => setFailed(true)}
          className="h-full w-full scale-105 object-cover blur-[1px]"
        />
      ) : (
        <div className="h-full w-full bg-gradient-to-br from-ink-900 via-ink-800 to-accent-700/30">
          <div className="flex h-full w-full items-end justify-end p-10">
            <Gamepad2 className="h-32 w-32 text-text-100/5" strokeWidth={1.5} />
          </div>
        </div>
      )}
      <div className="absolute inset-0 bg-hero-fade" />
      <div className="absolute left-6 top-5 lg:left-10">
        <Link
          to="/catalogue"
          className="inline-flex items-center gap-1.5 rounded-md bg-black/40 px-3 py-1.5 text-xs font-semibold text-text-300 backdrop-blur transition hover:bg-black/60 hover:text-text-100"
        >
          <ArrowLeft className="h-3.5 w-3.5" strokeWidth={2.5} />
          Catalogue
        </Link>
      </div>
    </div>
  )
}

function Cover({ release }: { release: CatalogReleaseDetail }) {
  const [failed, setFailed] = useState(false)
  if (failed || !release.coverUrl) {
    return (
      <div className="grid aspect-[3/4] w-36 shrink-0 place-items-center rounded-xl bg-gradient-to-br from-ink-700 to-ink-900 shadow-card ring-1 ring-white/10 sm:w-44">
        <Gamepad2 className="h-14 w-14 text-text-700" strokeWidth={1.5} />
      </div>
    )
  }
  return (
    <img
      src={catalogCover(release.releaseId)}
      alt={release.title}
      onError={() => setFailed(true)}
      className="aspect-[3/4] w-36 shrink-0 rounded-xl object-cover shadow-card ring-1 ring-white/10 sm:w-44"
    />
  )
}

function DetailRow({ label, value }: { label: string; value?: string }) {
  if (!value) return null
  return (
    <div>
      <dt className="text-[11px] font-semibold uppercase tracking-wider text-text-700">{label}</dt>
      <dd className="mt-1 break-words text-sm text-text-100">{value}</dd>
    </div>
  )
}

function SourcesPanel({
  releaseId,
  platformId,
  title,
  groups,
  isLoading,
  isError,
  error,
}: {
  releaseId: string
  platformId: string
  title: string
  groups?: CatalogSourceGroup[]
  isLoading: boolean
  isError: boolean
  error: Error | null
}) {
  if (isLoading) {
    return (
      <div className="rounded-xl border border-ink-700 bg-ink-900/60 p-5">
        <div className="flex items-center gap-3 text-sm text-text-300">
          <Spinner className="h-4 w-4" />
          Recherche dans les sources…
        </div>
      </div>
    )
  }
  if (isError) {
    return (
      <div className="rounded-xl border border-red-500/30 bg-red-500/10 p-5 text-sm text-red-300">
        {error?.message ?? "Erreur de recherche"}
      </div>
    )
  }
  const all = (groups ?? []).map((g) => ({ ...g, items: g.items ?? [] }))
  const total = all.reduce((s, g) => s + g.items.length, 0)

  return (
    <section className="space-y-4 rounded-xl border border-accent-500/30 bg-ink-900/60 p-5">
      <header>
        <h2 className="text-sm font-semibold text-text-100">
          Sources de téléchargement
          <span className="ml-2 text-xs text-text-500">
            {total} candidat{total > 1 ? "s" : ""}
          </span>
        </h2>
        <p className="mt-1 text-xs text-text-500">
          RETROX a interrogé chaque source pour « {title} » sur {platformId.toUpperCase()}.
          Choisis la version que tu veux.
        </p>
      </header>

      {all.map((g) => (
        <SourceGroup key={g.source.id} group={g} releaseId={releaseId} />
      ))}

      {total === 0 && (
        <p className="text-sm italic text-text-500">
          Aucune source n'a de candidat pour ce jeu. Tu peux toujours coller une URL manuelle dans
          la page Téléchargements.
        </p>
      )}
    </section>
  )
}

function SourceGroup({ group, releaseId }: { group: CatalogSourceGroup; releaseId: string }) {
  return (
    <div className="space-y-2">
      <div className="flex items-center gap-2">
        <h3 className="text-xs font-semibold uppercase tracking-wider text-accent-300">
          {group.source.name}
        </h3>
        {!group.source.downloadable && (
          <Badge>Lien externe seulement</Badge>
        )}
        {group.error && <span className="text-xs text-red-400">{group.error}</span>}
      </div>
      {group.items.length === 0 && !group.error && (
        <p className="text-xs italic text-text-500">Aucun candidat ici.</p>
      )}
      <div className="space-y-2">
        {group.items.map((rom) => (
          <SourceCandidate key={rom.id} rom={rom} releaseId={releaseId} />
        ))}
      </div>
    </div>
  )
}

function SourceCandidate({ rom, releaseId }: { rom: SourceROM; releaseId: string }) {
  const qc = useQueryClient()
  const downloadM = useMutation({
    mutationFn: () =>
      api.sourceDownload(rom.sourceId, {
        romId: rom.id,
        platformId: rom.platformId,
        title: rom.title,
      }),
    onSuccess: () => qc.invalidateQueries({ queryKey: qk.downloads }),
  })

  // Avoid lint noise on the unused releaseId — kept in the signature so
  // future caching or analytics can correlate the click back to the row.
  void releaseId

  return (
    <div className="flex flex-wrap items-center justify-between gap-3 rounded-md bg-ink-800/60 px-3 py-2">
      <div className="min-w-0 flex-1">
        <p className="truncate text-sm font-medium text-text-100" title={rom.title}>
          {rom.title}
        </p>
        <p className="text-[11px] text-text-500">
          {rom.sizeBytes ? formatBytes(rom.sizeBytes) : "taille inconnue"}
          {rom.description && ` · ${rom.description.split("\n")[0].slice(0, 80)}`}
        </p>
      </div>
      <div className="flex shrink-0 items-center gap-2">
        <a
          href={rom.externalUrl}
          target="_blank"
          rel="noreferrer noopener"
          aria-label="Voir sur la source"
          className="inline-flex h-7 w-7 items-center justify-center rounded-md text-text-500 transition hover:bg-ink-700 hover:text-text-100"
        >
          <ExternalLink className="h-3.5 w-3.5" strokeWidth={2.5} />
        </a>
        {rom.downloadable ? (
          <Button
            size="sm"
            variant="primary"
            onClick={() => downloadM.mutate()}
            disabled={downloadM.isPending || downloadM.isSuccess}
          >
            {downloadM.isPending ? (
              <Spinner className="h-3 w-3" />
            ) : (
              <DownloadIcon className="h-3.5 w-3.5" strokeWidth={2.5} />
            )}
            {downloadM.isSuccess ? "En file" : "Télécharger"}
          </Button>
        ) : (
          <a
            href={rom.externalUrl}
            target="_blank"
            rel="noreferrer noopener"
            className="inline-flex items-center gap-1.5 rounded-md bg-ink-700 px-3 py-1 text-xs font-semibold text-text-100 transition hover:bg-ink-600"
          >
            Ouvrir
          </a>
        )}
        {downloadM.isError && (
          <span className="w-full text-xs text-red-400">{(downloadM.error as Error).message}</span>
        )}
      </div>
    </div>
  )
}
