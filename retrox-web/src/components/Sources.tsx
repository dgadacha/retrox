import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { Download as DownloadIcon, ExternalLink, Search } from "lucide-react"
import { useMemo, useState } from "react"

import { Badge, Button, Spinner, inputClass } from "@/components/ui"
import { api } from "@/lib/api"
import { formatBytes } from "@/lib/format"
import { qk, usePlatforms } from "@/lib/hooks"
import type { SourceInfo, SourceROM } from "@/lib/types"

// Sources lets the user browse remote ROM catalogs (Internet Archive,
// PDRoms) from inside RETROX. Downloadable sources hand off to the
// existing /api/v1/downloads queue so the file lands in roms/<platform>/
// and the library is rescanned automatically. Non-downloadable sources
// (RSS-only feeds) show an "open in source" button instead.
export function SourcesPage() {
  const sourcesQ = useQuery({ queryKey: ["sources"], queryFn: api.sources })
  const platformsQ = usePlatforms()
  const [sourceId, setSourceId] = useState<string>("")
  const [platformId, setPlatformId] = useState<string>("")
  const [search, setSearch] = useState<string>("")
  const [submittedSearch, setSubmittedSearch] = useState<string>("")

  const sources = sourcesQ.data ?? []
  const current: SourceInfo | undefined =
    sources.find((s) => s.id === sourceId) ?? sources[0]
  const activeId = current?.id ?? ""

  if (sourcesQ.isLoading) {
    return (
      <div className="grid flex-1 place-items-center">
        <Spinner className="h-7 w-7" />
      </div>
    )
  }

  return (
    <div className="flex flex-1 flex-col">
      <header className="border-b border-ink-700 bg-ink-950/85 px-6 py-5 backdrop-blur lg:px-10">
        <h1 className="text-2xl font-bold tracking-tight">Sources</h1>
        <p className="mt-1 text-xs text-text-500">
          Catalogues distants à parcourir directement depuis RETROX. Téléchargement direct quand
          la source l'expose, sinon ouverture du lien d'origine.
        </p>
      </header>

      <div className="mx-auto flex w-full max-w-6xl flex-1 flex-col gap-6 px-6 py-6 lg:px-10">
        <div className="flex flex-wrap items-end gap-3">
          <Field label="Source">
            <select
              className={`${inputClass} w-56`}
              value={activeId}
              onChange={(e) => {
                setSourceId(e.target.value)
                setSubmittedSearch("")
                setSearch("")
              }}
            >
              {sources.map((s) => (
                <option key={s.id} value={s.id}>
                  {s.name}
                  {!s.downloadable && " (lien externe)"}
                </option>
              ))}
            </select>
          </Field>

          <Field label="Plateforme">
            <select
              className={`${inputClass} w-56`}
              value={platformId}
              onChange={(e) => setPlatformId(e.target.value)}
            >
              <option value="">Toutes les plateformes</option>
              {(platformsQ.data ?? [])
                .filter((p) => current?.supportedPlatforms.includes(p.id))
                .map((p) => (
                  <option key={p.id} value={p.id}>
                    {p.name}
                  </option>
                ))}
            </select>
          </Field>

          <Field label="Recherche">
            <form
              className="flex gap-2"
              onSubmit={(e) => {
                e.preventDefault()
                setSubmittedSearch(search)
              }}
            >
              <input
                className={`${inputClass} w-64`}
                placeholder="Mots-clés…"
                value={search}
                onChange={(e) => setSearch(e.target.value)}
              />
              <Button type="submit" variant="primary">
                <Search className="h-4 w-4" strokeWidth={2} />
              </Button>
            </form>
          </Field>
        </div>

        {current && (
          <p className="text-sm text-text-500">{current.description}</p>
        )}

        {current && (
          <SourceResults
            source={current}
            platformId={platformId}
            query={submittedSearch}
          />
        )}
      </div>
    </div>
  )
}

function Field({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <label className="flex flex-col gap-1">
      <span className="text-[11px] font-semibold uppercase tracking-wider text-text-700">
        {label}
      </span>
      {children}
    </label>
  )
}

function SourceResults({
  source,
  platformId,
  query,
}: {
  source: SourceInfo
  platformId: string
  query: string
}) {
  const [page, setPage] = useState(1)
  // Reset to page 1 whenever the filters change.
  const cacheKey = useMemo(
    () => ["source-browse", source.id, platformId, query, page],
    [source.id, platformId, query, page],
  )
  const browseQ = useQuery({
    queryKey: cacheKey,
    queryFn: () => api.sourceBrowse(source.id, { platform: platformId, q: query, page }),
  })

  if (browseQ.isLoading) {
    return (
      <div className="grid h-40 place-items-center">
        <Spinner className="h-6 w-6" />
      </div>
    )
  }
  if (browseQ.isError) {
    return <p className="text-sm text-red-400">{(browseQ.error as Error).message}</p>
  }

  const data = browseQ.data
  if (!data || data.items.length === 0) {
    return (
      <p className="rounded-xl border border-dashed border-ink-700 py-10 text-center text-sm text-text-500">
        Aucun résultat pour ces filtres.
      </p>
    )
  }

  return (
    <div className="space-y-4">
      <p className="text-xs text-text-500">
        {data.items.length} résultat{data.items.length > 1 ? "s" : ""}
        {data.hasMore && " · plus disponible"}
      </p>
      <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4">
        {data.items.map((rom) => (
          <SourceCard key={rom.id} rom={rom} sourceName={source.name} />
        ))}
      </div>
      {data.hasMore && (
        <div className="flex justify-center pt-2">
          <Button variant="ghost" onClick={() => setPage((p) => p + 1)}>
            Charger la suite (page {data.nextPage})
          </Button>
        </div>
      )}
    </div>
  )
}

function SourceCard({ rom, sourceName }: { rom: SourceROM; sourceName: string }) {
  const [coverFailed, setCoverFailed] = useState(false)
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

  return (
    <div className="flex flex-col overflow-hidden rounded-xl border border-ink-700 bg-ink-900/60">
      {rom.coverUrl && !coverFailed && (
        <img
          src={rom.coverUrl}
          alt=""
          loading="lazy"
          onError={() => setCoverFailed(true)}
          className="aspect-video w-full bg-ink-800 object-cover"
        />
      )}

      <div className="flex flex-1 flex-col gap-2 p-4">
        <div>
          <p className="line-clamp-2 text-sm font-semibold text-text-100" title={rom.title}>
            {rom.title}
          </p>
          <p className="mt-0.5 text-[11px] uppercase tracking-wider text-text-700">
            {sourceName}
            {rom.sizeBytes ? ` · ${formatBytes(rom.sizeBytes)}` : ""}
          </p>
        </div>

        {rom.description && (
          <p className="line-clamp-3 text-xs text-text-500">{rom.description}</p>
        )}

        <div className="mt-auto flex flex-wrap items-center gap-2 pt-2">
          {rom.downloadable ? (
            <Button
              variant="primary"
              size="sm"
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
              className="inline-flex items-center gap-1.5 rounded-md bg-ink-700 px-3 py-1.5 text-xs font-semibold text-text-100 transition hover:bg-ink-600"
            >
              <ExternalLink className="h-3.5 w-3.5" strokeWidth={2.5} />
              Voir sur la source
            </a>
          )}
          {rom.downloadable && (
            <a
              href={rom.externalUrl}
              target="_blank"
              rel="noreferrer noopener"
              aria-label="Page d'origine"
              className="inline-flex h-7 w-7 items-center justify-center rounded-md text-text-500 transition hover:bg-ink-700 hover:text-text-100"
            >
              <ExternalLink className="h-3.5 w-3.5" strokeWidth={2.5} />
            </a>
          )}
          {downloadM.isError && (
            <p className="w-full text-xs text-red-400">{(downloadM.error as Error).message}</p>
          )}
        </div>

        {!rom.platformId && (
          <Badge tone="warn">Plateforme inconnue — choisissez-en une dans le filtre</Badge>
        )}
      </div>
    </div>
  )
}
