import { ArrowLeft, Gamepad2, Star } from "lucide-react"
import { useMemo, useState } from "react"
import { Link, useNavigate, useParams } from "react-router-dom"

import { PlayButton } from "@/components/PlayButton"
import { Badge, Button, Spinner } from "@/components/ui"
import { gameImage } from "@/lib/api"
import { formatBytes, year } from "@/lib/format"
import { useFavorites, useGame, usePlatforms, useToggleFavorite } from "@/lib/hooks"
import type { Game } from "@/lib/types"

export function GameDetailPage() {
  const params = useParams()
  const navigate = useNavigate()
  const id = Number(params.id)
  const gameQ = useGame(id)
  const platformsQ = usePlatforms()
  const favoritesQ = useFavorites()
  const [tab, setTab] = useState<"overview" | "details" | "media">("overview")

  const isFav = (favoritesQ.data ?? []).some((f) => f.gameId === id)
  const favM = useToggleFavorite(id, isFav)

  const platformName = useMemo(() => {
    if (!gameQ.data) return ""
    return platformsQ.data?.find((p) => p.id === gameQ.data!.platformId)?.name ?? gameQ.data.platformId
  }, [gameQ.data, platformsQ.data])

  if (gameQ.isLoading) {
    return (
      <div className="grid flex-1 place-items-center">
        <Spinner className="h-7 w-7" />
      </div>
    )
  }
  if (gameQ.isError || !gameQ.data) {
    return (
      <div className="grid flex-1 place-items-center text-sm text-red-400">
        Jeu introuvable.{" "}
        <button onClick={() => navigate("/")} className="text-accent-400 hover:underline">
          Retour
        </button>
      </div>
    )
  }

  const game = gameQ.data
  const hasScreenshot = !!game.screenshotUrl

  return (
    <div className="flex flex-1 flex-col">
      <Hero game={game} platformName={platformName} />

      <div className="-mt-24 flex flex-1 flex-col px-6 pb-10 lg:px-10">
        <div className="relative z-10 flex flex-col gap-6 sm:flex-row sm:items-end">
          <Cover game={game} />
          <div className="flex-1 space-y-3">
            <Link
              to={`/?platform=${encodeURIComponent(game.platformId)}`}
              className="inline-block text-xs uppercase tracking-wider text-accent-300 hover:text-accent-400"
            >
              {platformName}
            </Link>
            <h1 className="text-3xl font-black tracking-tight text-text-100 sm:text-4xl">
              {game.title || game.fileName}
            </h1>
            <div className="flex flex-wrap items-center gap-3 text-sm text-text-300">
              {year(game.releaseDate) && <span>{year(game.releaseDate)}</span>}
              {game.region && <Badge>{game.region}</Badge>}
              {game.missing && <Badge tone="danger">Fichier manquant</Badge>}
            </div>
          </div>
          <div className="flex flex-wrap items-center gap-3 sm:self-center">
            <PlayButton gameId={game.id} disabled={game.missing} size="lg" />
            <Button variant="ghost" onClick={() => favM.mutate()} disabled={favM.isPending} size="lg">
              <Star
                className="h-4 w-4"
                strokeWidth={2}
                fill={isFav ? "currentColor" : "none"}
              />
              {isFav ? "Dans mes favoris" : "Ajouter aux favoris"}
            </Button>
          </div>
        </div>

        <div className="mt-8 flex gap-1 border-b border-ink-700">
          <TabButton active={tab === "overview"} onClick={() => setTab("overview")}>
            Aperçu
          </TabButton>
          <TabButton active={tab === "details"} onClick={() => setTab("details")}>
            Détails
          </TabButton>
          {hasScreenshot && (
            <TabButton active={tab === "media"} onClick={() => setTab("media")}>
              Capture
            </TabButton>
          )}
        </div>

        <div className="pt-6">
          {tab === "overview" && (
            <div className="grid gap-8 lg:grid-cols-[1fr_280px]">
              <div>
                {game.description ? (
                  <p className="whitespace-pre-line text-[15px] leading-relaxed text-text-300">
                    {game.description}
                  </p>
                ) : (
                  <p className="text-sm italic text-text-500">
                    Aucune description disponible. Téléchargez OpenVGDB dans les Réglages, puis
                    relancez une analyse.
                  </p>
                )}
              </div>
              <aside className="space-y-4 rounded-xl border border-ink-700 bg-ink-900/60 p-5">
                <DetailRow label="Genre" value={game.genre} />
                <DetailRow label="Développeur" value={game.developer} />
                <DetailRow label="Éditeur" value={game.publisher} />
                <DetailRow label="Sortie" value={game.releaseDate} />
              </aside>
            </div>
          )}

          {tab === "details" && (
            <dl className="grid grid-cols-1 gap-6 sm:grid-cols-2 lg:grid-cols-3">
              <DetailRow label="Plateforme" value={platformName} />
              <DetailRow label="Genre" value={game.genre} />
              <DetailRow label="Développeur" value={game.developer} />
              <DetailRow label="Éditeur" value={game.publisher} />
              <DetailRow label="Sortie" value={game.releaseDate} />
              <DetailRow label="Région" value={game.region} />
              <DetailRow label="Fichier" value={game.fileName} mono />
              <DetailRow label="Chemin" value={game.path} mono />
              <DetailRow label="Taille" value={formatBytes(game.fileSize)} />
              <DetailRow label="CRC32" value={game.crc} mono />
              <DetailRow label="MD5" value={game.md5} mono />
            </dl>
          )}

          {tab === "media" && hasScreenshot && (
            <div className="overflow-hidden rounded-xl border border-ink-700 bg-ink-900">
              <img
                src={gameImage(game.id, "screenshot")}
                alt="Capture"
                className="w-full object-contain"
              />
            </div>
          )}
        </div>
      </div>
    </div>
  )
}

function Hero({ game, platformName }: { game: Game; platformName: string }) {
  const initial = game.screenshotUrl ? gameImage(game.id, "screenshot") : null
  const [src, setSrc] = useState<string | null>(initial)

  return (
    <div className="relative h-72 w-full overflow-hidden bg-ink-900 sm:h-96">
      {src ? (
        <img
          src={src}
          alt=""
          onError={() => setSrc(null)}
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
          to="/"
          className="inline-flex items-center gap-1.5 rounded-md bg-black/40 px-3 py-1.5 text-xs font-semibold text-text-300 backdrop-blur transition hover:bg-black/60 hover:text-text-100"
        >
          <ArrowLeft className="h-3.5 w-3.5" strokeWidth={2.5} />
          Bibliothèque
        </Link>
        {platformName && (
          <p className="mt-3 text-[11px] uppercase tracking-[0.2em] text-text-500">
            {platformName}
          </p>
        )}
      </div>
    </div>
  )
}

function Cover({ game }: { game: Game }) {
  const [failed, setFailed] = useState(false)
  if (failed || !game.coverUrl) {
    return (
      <div className="grid aspect-[3/4] w-36 shrink-0 place-items-center rounded-xl bg-gradient-to-br from-ink-700 to-ink-900 shadow-card ring-1 ring-white/10 sm:w-44">
        <Gamepad2 className="h-14 w-14 text-text-700" strokeWidth={1.5} />
      </div>
    )
  }
  return (
    <img
      src={gameImage(game.id, "cover")}
      alt={game.title}
      onError={() => setFailed(true)}
      className="aspect-[3/4] w-36 shrink-0 rounded-xl object-cover shadow-card ring-1 ring-white/10 sm:w-44"
    />
  )
}

function TabButton({
  active,
  onClick,
  children,
}: {
  active: boolean
  onClick: () => void
  children: React.ReactNode
}) {
  return (
    <button
      onClick={onClick}
      className={`-mb-px border-b-2 px-4 py-2 text-sm font-semibold transition ${
        active
          ? "border-accent-400 text-text-100"
          : "border-transparent text-text-500 hover:text-text-100"
      }`}
    >
      {children}
    </button>
  )
}

function DetailRow({
  label,
  value,
  mono,
}: {
  label: string
  value?: string
  mono?: boolean
}) {
  if (!value) return null
  return (
    <div>
      <dt className="text-[11px] font-semibold uppercase tracking-wider text-text-700">
        {label}
      </dt>
      <dd
        className={`mt-1 break-words text-text-100 ${mono ? "font-mono text-xs" : "text-sm"}`}
        title={value}
      >
        {value}
      </dd>
    </div>
  )
}
