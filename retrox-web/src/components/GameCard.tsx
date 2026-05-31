import clsx from "clsx"
import { Gamepad2 } from "lucide-react"
import { useState } from "react"
import { Link } from "react-router-dom"

import { gameImage } from "@/lib/api"

interface Props {
  id: number
  title: string
  platformName?: string
}

// GameCard is the Steam-style capsule used in the library grid. Cover
// 404s gracefully fall back to a titled gradient tile so a missing
// scrape never leaves an empty hole on the page.
export function GameCard({ id, title, platformName }: Props) {
  const [failed, setFailed] = useState(false)

  return (
    <Link
      to={`/game/${id}`}
      title={title}
      className="group relative aspect-[3/4] overflow-hidden rounded-lg bg-ink-800 ring-1 ring-inset ring-white/5 outline-none transition duration-200 hover:-translate-y-1 hover:ring-accent-500/60 hover:shadow-[0_10px_30px_-10px_rgba(139,92,246,0.45)] focus-visible:ring-accent-500"
    >
      {!failed ? (
        <img
          src={gameImage(id, "cover")}
          alt={title}
          loading="lazy"
          onError={() => setFailed(true)}
          className="h-full w-full object-cover"
        />
      ) : (
        <div className="flex h-full w-full flex-col items-center justify-center gap-3 bg-gradient-to-br from-ink-700 via-ink-800 to-ink-900 p-4 text-center">
          <Gamepad2 className="h-10 w-10 text-text-700" strokeWidth={1.5} />
          <span className="line-clamp-3 text-xs font-semibold text-text-300">{title}</span>
          {platformName && (
            <span className="text-[10px] uppercase tracking-wider text-text-700">
              {platformName}
            </span>
          )}
        </div>
      )}

      <div
        className={clsx(
          "pointer-events-none absolute inset-x-0 bottom-0 bg-gradient-to-t from-black/95 via-black/50 to-transparent p-3 pt-10 transition",
          failed ? "opacity-0" : "opacity-0 group-hover:opacity-100",
        )}
      >
        <p className="line-clamp-2 text-left text-xs font-semibold text-text-100">{title}</p>
        {platformName && (
          <p className="text-left text-[10px] uppercase tracking-wider text-text-500">
            {platformName}
          </p>
        )}
      </div>
    </Link>
  )
}

// GameRow is the list-view variant: horizontal strip with a tiny cover,
// title and platform. Density goal: ~10 rows per screen at 1080p.
export function GameRow({ id, title, platformName }: Props) {
  const [failed, setFailed] = useState(false)
  return (
    <Link
      to={`/game/${id}`}
      title={title}
      className="group flex items-center gap-4 rounded-md bg-ink-800/40 px-3 py-2 ring-1 ring-inset ring-transparent transition hover:bg-ink-800 hover:ring-accent-500/40"
    >
      <div className="h-14 w-10 shrink-0 overflow-hidden rounded bg-ink-700">
        {!failed ? (
          <img
            src={gameImage(id, "cover")}
            alt=""
            loading="lazy"
            onError={() => setFailed(true)}
            className="h-full w-full object-cover"
          />
        ) : (
          <div className="grid h-full w-full place-items-center">
            <Gamepad2 className="h-5 w-5 text-text-700" strokeWidth={1.5} />
          </div>
        )}
      </div>
      <div className="min-w-0 flex-1">
        <p className="truncate text-sm font-semibold text-text-100 group-hover:text-accent-300">
          {title}
        </p>
        {platformName && (
          <p className="truncate text-[11px] uppercase tracking-wider text-text-700">
            {platformName}
          </p>
        )}
      </div>
    </Link>
  )
}
