import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { useState } from "react"

import { Badge, Button, Spinner, inputClass } from "@/components/ui"
import { api } from "@/lib/api"
import { formatBytes } from "@/lib/format"
import { qk, usePlatforms } from "@/lib/hooks"
import type { Download, DownloadStatus } from "@/lib/types"

const STATUS_LABEL: Record<DownloadStatus, string> = {
  queued: "En file",
  downloading: "En cours",
  done: "Terminé",
  error: "Erreur",
  canceled: "Annulé",
}

const STATUS_TONE: Record<DownloadStatus, "neutral" | "accent" | "success" | "warn" | "danger"> = {
  queued: "neutral",
  downloading: "accent",
  done: "success",
  error: "danger",
  canceled: "neutral",
}

export function DownloadsPage() {
  const platformsQ = usePlatforms()
  const qc = useQueryClient()
  const downloadsQ = useQuery({
    queryKey: qk.downloads,
    queryFn: api.downloads,
    refetchInterval: 1000,
  })

  const [url, setUrl] = useState("")
  const [platformId, setPlatformId] = useState("")
  const [title, setTitle] = useState("")

  const createM = useMutation({
    mutationFn: () => api.createDownload({ url: url.trim(), platformId, title: title.trim() }),
    onSuccess: () => {
      setUrl("")
      setTitle("")
      qc.invalidateQueries({ queryKey: qk.downloads })
    },
  })
  const cancelM = useMutation({
    mutationFn: (id: number) => api.cancelDownload(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: qk.downloads }),
  })

  const downloads = downloadsQ.data ?? []

  return (
    <div className="flex flex-1 flex-col">
      <header className="border-b border-ink-700 bg-ink-950/85 px-6 py-5 backdrop-blur lg:px-10">
        <h1 className="text-2xl font-bold tracking-tight">Téléchargements</h1>
        <p className="mt-1 text-xs text-text-500">
          Collez une URL directe vers une ROM. Le fichier atterrit dans votre premier dossier ROM
          configuré, puis la bibliothèque est ré-analysée automatiquement.
        </p>
      </header>

      <div className="mx-auto w-full max-w-4xl space-y-8 px-6 py-8 lg:px-10">
        <form
          className="space-y-3 rounded-xl border border-ink-700 bg-ink-900/60 p-5"
          onSubmit={(e) => {
            e.preventDefault()
            if (url.trim()) createM.mutate()
          }}
        >
          <input
            className={inputClass}
            placeholder="https://exemple.org/mon-jeu.zip"
            value={url}
            onChange={(e) => setUrl(e.target.value)}
          />
          <div className="flex flex-col gap-3 sm:flex-row">
            <select
              className={`${inputClass} sm:w-56`}
              value={platformId}
              onChange={(e) => setPlatformId(e.target.value)}
            >
              <option value="">Plateforme (auto)</option>
              {(platformsQ.data ?? []).map((p) => (
                <option key={p.id} value={p.id}>
                  {p.name}
                </option>
              ))}
            </select>
            <input
              className={inputClass}
              placeholder="Titre (optionnel)"
              value={title}
              onChange={(e) => setTitle(e.target.value)}
            />
            <Button
              type="submit"
              variant="primary"
              disabled={!url.trim() || createM.isPending}
              className="sm:px-6"
            >
              {createM.isPending ? <Spinner className="h-4 w-4" /> : "Télécharger"}
            </Button>
          </div>
          {createM.isError && (
            <p className="text-sm text-red-400">{(createM.error as Error).message}</p>
          )}
        </form>

        <section className="space-y-2">
          <h2 className="px-1 text-[11px] font-semibold uppercase tracking-wider text-text-700">
            File d'attente
          </h2>
          {downloads.length === 0 ? (
            <p className="rounded-xl border border-dashed border-ink-700 py-10 text-center text-sm text-text-500">
              Aucun téléchargement pour le moment.
            </p>
          ) : (
            <div className="space-y-2">
              {downloads.map((d) => (
                <DownloadRow key={d.id} d={d} onCancel={() => cancelM.mutate(d.id)} />
              ))}
            </div>
          )}
        </section>
      </div>
    </div>
  )
}

function DownloadRow({ d, onCancel }: { d: Download; onCancel: () => void }) {
  const cancellable = d.status === "queued" || d.status === "downloading"
  const pct = Math.round((d.progress || 0) * 100)

  return (
    <div className="rounded-xl border border-ink-700 bg-ink-900/60 p-4">
      <div className="flex items-start justify-between gap-4">
        <div className="min-w-0">
          <p className="truncate font-medium text-text-100" title={d.title || d.url}>
            {d.title || d.url}
          </p>
          <p className="truncate text-xs text-text-500" title={d.url}>
            {d.url}
          </p>
        </div>
        <div className="flex shrink-0 items-center gap-3">
          <Badge tone={STATUS_TONE[d.status]}>{STATUS_LABEL[d.status]}</Badge>
          {cancellable && (
            <button onClick={onCancel} className="text-xs text-text-500 hover:text-red-400">
              Annuler
            </button>
          )}
        </div>
      </div>

      {d.status === "downloading" && (
        <div className="mt-3">
          <div className="h-1.5 w-full overflow-hidden rounded-full bg-ink-700">
            <div
              className="h-full bg-accent-gradient transition-all"
              style={{ width: d.bytesTotal > 0 ? `${pct}%` : "100%" }}
            />
          </div>
          <p className="mt-1 text-xs text-text-500">
            {formatBytes(d.bytesDone)}
            {d.bytesTotal > 0 ? ` / ${formatBytes(d.bytesTotal)} · ${pct}%` : " téléchargés"}
          </p>
        </div>
      )}
      {d.status === "error" && d.error && (
        <p className="mt-2 text-xs text-red-400">{d.error}</p>
      )}
    </div>
  )
}
