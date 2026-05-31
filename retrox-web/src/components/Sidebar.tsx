import clsx from "clsx"
import {
  Download as DownloadIcon,
  Globe,
  History,
  Library as LibraryIcon,
  Settings as SettingsIcon,
  Star,
} from "lucide-react"
import type { ComponentType, ReactNode } from "react"
import { NavLink, useSearchParams } from "react-router-dom"

import { Spinner, Wordmark } from "@/components/ui"
import { useFavorites, useGames, useHistory, usePlatforms, useStatus } from "@/lib/hooks"

type IconCmp = ComponentType<{ className?: string; size?: number; strokeWidth?: number }>

// Sidebar is the persistent navigation: brand, library shortcuts, the
// platform list (with counts), and tool routes. It's modeled on Steam's
// left rail but uses our own typography + violet accent.
export function Sidebar() {
  const gamesQ = useGames()
  const platformsQ = usePlatforms()
  const favoritesQ = useFavorites()
  const historyQ = useHistory()
  const statusQ = useStatus()

  const games = gamesQ.data ?? []
  const platformCounts = new Map<string, number>()
  for (const g of games) {
    platformCounts.set(g.platformId || "", (platformCounts.get(g.platformId || "") ?? 0) + 1)
  }

  const platforms = (platformsQ.data ?? []).filter((p) => (platformCounts.get(p.id) ?? 0) > 0)
  const unsortedCount = platformCounts.get("") ?? 0

  return (
    <aside className="sticky top-0 hidden h-screen w-64 shrink-0 self-start flex-col border-r border-ink-700 bg-ink-900 lg:flex">
      <div className="flex items-center gap-2 px-5 py-5">
        <Wordmark className="text-2xl" />
        {statusQ.data?.scanning && <Spinner className="ml-auto h-4 w-4" />}
      </div>

      <nav className="flex-1 overflow-y-auto px-3 pb-5">
        <Section title="Bibliothèque">
          <SidebarLink to="/" end Icon={LibraryIcon}>
            Tous les jeux
            <Count>{games.length}</Count>
          </SidebarLink>
          <SidebarLink to="/?filter=favorites" Icon={Star} matchSearch="filter=favorites">
            Favoris
            <Count>{favoritesQ.data?.length ?? 0}</Count>
          </SidebarLink>
          <SidebarLink to="/?filter=recent" Icon={History} matchSearch="filter=recent">
            Récemment joués
            <Count>{historyQ.data?.length ?? 0}</Count>
          </SidebarLink>
        </Section>

        {platforms.length > 0 && (
          <Section title="Plateformes">
            {platforms.map((p) => (
              <SidebarLink
                key={p.id}
                to={`/?platform=${encodeURIComponent(p.id)}`}
                matchSearch={`platform=${p.id}`}
              >
                {p.name}
                <Count>{platformCounts.get(p.id)}</Count>
              </SidebarLink>
            ))}
            {unsortedCount > 0 && (
              <SidebarLink to="/?platform=__none__" matchSearch="platform=__none__">
                Non classé
                <Count>{unsortedCount}</Count>
              </SidebarLink>
            )}
          </Section>
        )}

        <Section title="Système">
          <SidebarLink to="/sources" Icon={Globe}>
            Sources
          </SidebarLink>
          <SidebarLink to="/downloads" Icon={DownloadIcon}>
            Téléchargements
          </SidebarLink>
          <SidebarLink to="/settings" Icon={SettingsIcon}>
            Réglages
          </SidebarLink>
        </Section>
      </nav>

      <div className="border-t border-ink-700 px-5 py-3 text-[11px] text-text-700">
        v{statusQ.data?.version ?? "0.0"} · {statusQ.data?.gamesScraped ?? 0}/
        {statusQ.data?.games ?? 0} scrappés
      </div>
    </aside>
  )
}

function Section({ title, children }: { title: string; children: ReactNode }) {
  return (
    <div className="mt-4 first:mt-2">
      <p className="px-3 pb-1 text-[11px] font-semibold uppercase tracking-wider text-text-700">
        {title}
      </p>
      <div className="space-y-0.5">{children}</div>
    </div>
  )
}

function SidebarLink({
  to,
  end,
  Icon,
  matchSearch,
  children,
}: {
  to: string
  end?: boolean
  Icon?: IconCmp
  matchSearch?: string
  children: ReactNode
}) {
  // NavLink only considers pathname for activity; query-string filters
  // need a manual check so "Favoris" lights up only on its own URL and
  // "Tous les jeux" stops being active when a filter is selected.
  const [search] = useSearchParams()
  const here = window.location.pathname === to.split("?")[0]
  const hasFilterOrPlatform = search.has("filter") || search.has("platform")

  return (
    <NavLink
      to={to}
      end={end}
      className={({ isActive }) => {
        let active = isActive
        if (matchSearch) {
          const [k, v] = matchSearch.split("=")
          active = here && search.get(k) === v
        } else if (end) {
          active = here && !hasFilterOrPlatform
        } else if (to === "/") {
          active = here && !hasFilterOrPlatform
        }
        return clsx(
          "group flex items-center gap-2 rounded-md px-3 py-1.5 text-sm font-medium transition",
          active
            ? "bg-accent-500/15 text-text-100 ring-1 ring-inset ring-accent-500/30"
            : "text-text-300 hover:bg-white/5 hover:text-text-100",
        )
      }}
    >
      {Icon && <Icon className="h-4 w-4 shrink-0 opacity-70" strokeWidth={2} />}
      <span className="flex w-full items-center justify-between gap-2">{children}</span>
    </NavLink>
  )
}

function Count({ children }: { children: ReactNode }) {
  return (
    <span className="ml-auto rounded bg-ink-700 px-1.5 text-[10px] font-semibold tabular-nums text-text-500">
      {children}
    </span>
  )
}
