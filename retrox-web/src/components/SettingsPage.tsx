import { useMutation, useQueryClient } from "@tanstack/react-query"
import { Check, Plus, X } from "lucide-react"
import { useEffect, useState } from "react"
import type { ReactNode } from "react"

import { Badge, Button, Spinner, inputClass } from "@/components/ui"
import { api } from "@/lib/api"
import { qk, useEmulators, useScanControl, useSettings, useStatus } from "@/lib/hooks"

export function SettingsPage() {
  return (
    <div className="flex flex-1 flex-col">
      <header className="border-b border-ink-700 bg-ink-950/85 px-6 py-5 backdrop-blur lg:px-10">
        <h1 className="text-2xl font-bold tracking-tight">Réglages</h1>
        <p className="mt-1 text-xs text-text-500">
          Source de métadonnées, dossiers ROM, RetroArch et overrides par plateforme.
        </p>
      </header>

      <div className="mx-auto w-full max-w-4xl space-y-6 px-6 py-8 lg:px-10">
        <OpenVGDBSection />
        <IGDBSection />
        <ServerConfigForm />
        <ScanSection />
        <EmulatorEditor />
      </div>
    </div>
  )
}

function Card({ title, desc, children }: { title: string; desc?: ReactNode; children: ReactNode }) {
  return (
    <section className="overflow-hidden rounded-xl border border-ink-700 bg-ink-900/60">
      <header className="border-b border-ink-700 px-5 py-4">
        <h2 className="text-base font-semibold text-text-100">{title}</h2>
        {desc && <p className="mt-1 text-xs text-text-500">{desc}</p>}
      </header>
      <div className="space-y-4 p-5">{children}</div>
    </section>
  )
}

function Label({ children }: { children: ReactNode }) {
  return (
    <label className="mb-1 block text-[11px] font-semibold uppercase tracking-wider text-text-700">
      {children}
    </label>
  )
}

function OpenVGDBSection() {
  const statusQ = useStatus()
  const qc = useQueryClient()
  const downloadM = useMutation({
    mutationFn: () => api.downloadOpenVGDB(),
    onSuccess: () => qc.invalidateQueries({ queryKey: qk.status }),
  })

  const s = statusQ.data
  const ready = !!s?.openvgdbReady

  return (
    <Card
      title="Source de métadonnées — OpenVGDB"
      desc="Base SQLite de jaquettes, descriptions et infos jeu maintenue par la communauté. Téléchargée une fois, fonctionne entièrement hors-ligne, sans compte. Source : github.com/OpenVGDB/OpenVGDB."
    >
      <div className="flex flex-wrap items-center justify-between gap-4">
        <div className="space-y-2 text-sm">
          {ready ? (
            <>
              <Badge tone="success">Base prête</Badge>
              <p className="text-text-300">
                <span className="tabular-nums">{s!.openvgdbRoms.toLocaleString("fr")}</span> ROMs ·{" "}
                <span className="tabular-nums">{s!.openvgdbReleases.toLocaleString("fr")}</span>{" "}
                fiches
              </p>
              <p className="font-mono text-xs text-text-700" title={s!.openvgdbPath}>
                {s!.openvgdbPath}
              </p>
            </>
          ) : (
            <>
              <Badge tone="warn">Non téléchargée</Badge>
              <p className="text-text-300">
                Cliquez sur <strong>Télécharger</strong> pour récupérer ~9 MB depuis GitHub.
              </p>
            </>
          )}
          <p className="text-xs text-text-500">
            En complément, les jaquettes manquantes sont cherchées sur{" "}
            <code className="rounded bg-ink-800 px-1 text-text-300">thumbnails.libretro.com</code>
            (CDN public, sans compte).
          </p>
        </div>
        <Button
          variant="primary"
          onClick={() => downloadM.mutate()}
          disabled={downloadM.isPending}
        >
          {downloadM.isPending ? <Spinner className="h-4 w-4" /> : null}
          {ready ? "Mettre à jour" : "Télécharger"}
        </Button>
      </div>
      {downloadM.isSuccess && (
        <p className="text-sm text-emerald-400">
          Base mise à jour ({downloadM.data.roms.toLocaleString("fr")} ROMs).
        </p>
      )}
      {downloadM.isError && (
        <p className="text-sm text-red-400">{(downloadM.error as Error).message}</p>
      )}
    </Card>
  )
}

function IGDBSection() {
  const settingsQ = useSettings()
  const statusQ = useStatus()
  const qc = useQueryClient()

  const [clientId, setClientId] = useState("")
  const [clientSecret, setClientSecret] = useState("")
  const s = settingsQ.data

  useEffect(() => {
    if (s) setClientId(s.igdbClientId ?? "")
  }, [s])

  const saveM = useMutation({
    mutationFn: () => api.setIGDBCredentials({ clientId, clientSecret }),
    onSuccess: () => {
      setClientSecret("")
      qc.invalidateQueries({ queryKey: qk.settings })
      qc.invalidateQueries({ queryKey: qk.status })
      qc.invalidateQueries({ queryKey: ["catalog-platforms"] })
    },
  })

  const configured = !!statusQ.data?.igdbConfigured

  return (
    <Card
      title="Source complémentaire — IGDB (Twitch)"
      desc={
        <>
          Pour les plateformes qu'OpenVGDB ne couvre pas (PS2, Dreamcast, Wii, Neo Geo, …).
          Crée une app sur{" "}
          <a
            href="https://dev.twitch.tv/console/apps/create"
            target="_blank"
            rel="noreferrer noopener"
            className="text-accent-400 hover:underline"
          >
            dev.twitch.tv/console/apps
          </a>
          {" "}
          (5 min, gratuit), copie le Client ID et génère un Client Secret.
        </>
      }
    >
      <div className="flex flex-wrap items-center gap-2">
        {configured ? (
          <Badge tone="success">Connecté</Badge>
        ) : (
          <Badge tone="warn">Non configuré</Badge>
        )}
      </div>
      <form
        className="space-y-4"
        onSubmit={(e) => {
          e.preventDefault()
          saveM.mutate()
        }}
      >
        <div className="grid gap-4 sm:grid-cols-2">
          <div>
            <Label>Client ID</Label>
            <input
              className={inputClass}
              value={clientId}
              onChange={(e) => setClientId(e.target.value)}
              placeholder="xxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"
            />
          </div>
          <div>
            <Label>Client Secret</Label>
            <input
              type="password"
              className={inputClass}
              value={clientSecret}
              onChange={(e) => setClientSecret(e.target.value)}
              placeholder={s?.igdbClientSecretSet ? "•••••• (défini)" : "non défini"}
            />
          </div>
        </div>
        <div className="flex items-center gap-3">
          <Button type="submit" variant="primary" disabled={saveM.isPending}>
            {saveM.isPending ? <Spinner className="h-4 w-4" /> : "Vérifier + enregistrer"}
          </Button>
          {saveM.isSuccess && (
            <span className="inline-flex items-center gap-1.5 text-sm text-emerald-400">
              <Check className="h-4 w-4" strokeWidth={2.5} />
              IGDB connecté
            </span>
          )}
          {saveM.isError && (
            <span className="text-sm text-red-400">{(saveM.error as Error).message}</span>
          )}
        </div>
      </form>
    </Card>
  )
}

function ServerConfigForm() {
  const settingsQ = useSettings()
  const qc = useQueryClient()

  const [f, setF] = useState({
    retroarchBin: "",
    retroarchCores: "",
  })
  const [romDirs, setRomDirs] = useState<string[]>([""])

  const s = settingsQ.data
  useEffect(() => {
    if (!s) return
    setF({
      retroarchBin: s.retroarchBin,
      retroarchCores: s.retroarchCores,
    })
    setRomDirs(s.romDirs.length ? s.romDirs : [""])
  }, [s])

  const saveM = useMutation({
    mutationFn: () =>
      api.updateSettings({ ...f, romDirs: romDirs.map((d) => d.trim()).filter(Boolean) }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: qk.settings })
      qc.invalidateQueries({ queryKey: qk.status })
    },
  })

  const set = (k: keyof typeof f) => (e: { target: { value: string } }) =>
    setF((prev) => ({ ...prev, [k]: e.target.value }))

  return (
    <form
      className="space-y-6"
      onSubmit={(e) => {
        e.preventDefault()
        saveM.mutate()
      }}
    >
      <Card
        title="Bibliothèque"
        desc="Dossiers analysés pour trouver des ROMs. Le premier dossier reçoit les téléchargements."
      >
        <div className="space-y-2">
          {romDirs.map((dir, i) => (
            <div key={i} className="flex gap-2">
              <input
                className={inputClass}
                placeholder="/chemin/vers/roms"
                value={dir}
                onChange={(e) => setRomDirs(romDirs.map((d, j) => (j === i ? e.target.value : d)))}
              />
              <Button
                type="button"
                variant="ghost"
                aria-label="Retirer ce dossier"
                onClick={() =>
                  setRomDirs(romDirs.length > 1 ? romDirs.filter((_, j) => j !== i) : [""])
                }
              >
                <X className="h-4 w-4" strokeWidth={2.5} />
              </Button>
            </div>
          ))}
          <button
            type="button"
            onClick={() => setRomDirs([...romDirs, ""])}
            className="inline-flex items-center gap-1.5 text-sm text-accent-400 transition hover:text-accent-300"
          >
            <Plus className="h-4 w-4" strokeWidth={2.5} />
            Ajouter un dossier
          </button>
        </div>
      </Card>

      <Card
        title="RetroArch"
        desc="Chemin du binaire RetroArch et du dossier des cœurs libretro (laisser vide pour l'emplacement par défaut macOS/Linux)."
      >
        <div className="grid gap-4">
          <div>
            <Label>Binaire RetroArch</Label>
            <input
              className={inputClass}
              placeholder="/Applications/RetroArch.app/Contents/MacOS/RetroArch"
              value={f.retroarchBin}
              onChange={set("retroarchBin")}
            />
          </div>
          <div>
            <Label>Dossier des cœurs</Label>
            <input
              className={inputClass}
              placeholder="~/Library/Application Support/RetroArch/cores"
              value={f.retroarchCores}
              onChange={set("retroarchCores")}
            />
          </div>
        </div>
      </Card>

      <div className="flex items-center gap-3">
        <Button type="submit" variant="primary" disabled={saveM.isPending} size="lg">
          {saveM.isPending ? <Spinner className="h-4 w-4" /> : "Enregistrer"}
        </Button>
        {saveM.isSuccess && (
          <span className="inline-flex items-center gap-1.5 text-sm text-emerald-400">
            <Check className="h-4 w-4" strokeWidth={2.5} />
            Enregistré
          </span>
        )}
        {saveM.isError && (
          <span className="text-sm text-red-400">{(saveM.error as Error).message}</span>
        )}
      </div>
    </form>
  )
}

function ScanSection() {
  const statusQ = useStatus()
  const { running, progress, startScan, starting, error } = useScanControl()
  const s = statusQ.data
  const pct =
    progress && progress.total > 0 ? Math.round((progress.current / progress.total) * 100) : 0

  return (
    <Card
      title="Analyse de la bibliothèque"
      desc="Indexe les dossiers ROM puis récupère les métadonnées depuis OpenVGDB et libretro-thumbnails."
    >
      <div className="flex items-center justify-between gap-4">
        <div className="text-sm text-text-300">
          {s ? (
            <span>
              {s.games} jeux · {s.gamesScraped} avec métadonnées
            </span>
          ) : (
            "—"
          )}
          {s && !s.metadataReady && (
            <div className="mt-2">
              <Badge tone="warn">Aucune source de métadonnées</Badge>
              <p className="mt-1 text-xs text-text-500">
                Téléchargez OpenVGDB ci-dessus, puis relancez une analyse.
              </p>
            </div>
          )}
        </div>
        <Button variant="primary" onClick={startScan} disabled={running || starting}>
          {running || starting ? <Spinner className="h-4 w-4" /> : null}
          {running ? "Analyse…" : "Lancer une analyse"}
        </Button>
      </div>
      {running && progress && (
        <div>
          <div className="h-1.5 w-full overflow-hidden rounded-full bg-ink-700">
            <div
              className="h-full bg-accent-gradient transition-all"
              style={{ width: progress.total > 0 ? `${pct}%` : "12%" }}
            />
          </div>
          <p className="mt-1 truncate text-xs text-text-500">
            {progress.current}/{progress.total} — {progress.currentFile}
          </p>
        </div>
      )}
      {error && <p className="text-sm text-red-400">{error.message}</p>}
    </Card>
  )
}

function EmulatorEditor() {
  const emulatorsQ = useEmulators()
  const qc = useQueryClient()
  const [selected, setSelected] = useState("")
  const [command, setCommand] = useState("")
  const [core, setCore] = useState("")
  const [args, setArgs] = useState("")

  const views = emulatorsQ.data ?? []
  const current = views.find((v) => v.platformId === selected)

  useEffect(() => {
    const v = views.find((x) => x.platformId === selected)
    setCommand(v?.override?.command ?? "")
    setCore(v?.override?.core ?? "")
    setArgs(v?.override?.args ?? "")
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [selected, emulatorsQ.data])

  const saveM = useMutation({
    mutationFn: () => api.setEmulator(selected, { command, core, args }),
    onSuccess: () => qc.invalidateQueries({ queryKey: qk.emulators }),
  })
  const clearM = useMutation({
    mutationFn: () => api.deleteEmulator(selected),
    onSuccess: () => qc.invalidateQueries({ queryKey: qk.emulators }),
  })

  return (
    <Card
      title="Émulateurs"
      desc="RETROX choisit un cœur RetroArch (ou un émulateur autonome) par défaut. Forcez une commande par plateforme si besoin."
    >
      <select
        className={inputClass}
        value={selected}
        onChange={(e) => setSelected(e.target.value)}
      >
        <option value="">Choisir une plateforme…</option>
        {views.map((v) => (
          <option key={v.platformId} value={v.platformId}>
            {v.name}
            {v.override ? " — personnalisé" : ""}
          </option>
        ))}
      </select>

      {current && (
        <div className="space-y-3">
          <p className="text-xs text-text-500">
            Défaut :{" "}
            {current.defaultStandalone
              ? `autonome « ${current.defaultStandalone} »`
              : current.defaultCore
                ? `cœur ${current.defaultCore}`
                : "aucun"}
          </p>
          <div>
            <Label>Commande (vide = défaut)</Label>
            <input
              className={inputClass}
              placeholder="ex. /Applications/RetroArch.app/Contents/MacOS/RetroArch"
              value={command}
              onChange={(e) => setCommand(e.target.value)}
            />
          </div>
          <div>
            <Label>Cœur libretro</Label>
            <input
              className={inputClass}
              placeholder={current.defaultCore || "ex. snes9x"}
              value={core}
              onChange={(e) => setCore(e.target.value)}
            />
          </div>
          <div>
            <Label>
              Arguments (<code>{"{rom}"}</code> et <code>{"{core}"}</code> sont remplacés)
            </Label>
            <input
              className={inputClass}
              placeholder="-L {core} {rom}"
              value={args}
              onChange={(e) => setArgs(e.target.value)}
            />
          </div>
          <div className="flex items-center gap-3">
            <Button
              type="button"
              variant="primary"
              onClick={() => saveM.mutate()}
              disabled={saveM.isPending}
            >
              {saveM.isPending ? <Spinner className="h-4 w-4" /> : "Enregistrer l'override"}
            </Button>
            {current.override && (
              <Button
                type="button"
                variant="ghost"
                onClick={() => clearM.mutate()}
                disabled={clearM.isPending}
              >
                Réinitialiser
              </Button>
            )}
            {saveM.isSuccess && <Check className="h-4 w-4 text-emerald-400" strokeWidth={2.5} />}
          </div>
        </div>
      )}
    </Card>
  )
}
