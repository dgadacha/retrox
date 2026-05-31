import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { useEffect, useRef } from "react"

import { api } from "@/lib/api"

// Centralized query keys so mutations can invalidate precisely.
export const qk = {
  status: ["status"] as const,
  platforms: ["platforms"] as const,
  games: ["games"] as const,
  game: (id: number) => ["game", id] as const,
  downloads: ["downloads"] as const,
  emulators: ["emulators"] as const,
  settings: ["settings"] as const,
  scanStatus: ["scanStatus"] as const,
  history: (uid: string) => ["history", uid] as const,
  favorites: (uid: string) => ["favorites", uid] as const,
}

export const useStatus = () => useQuery({ queryKey: qk.status, queryFn: api.status })
export const usePlatforms = () =>
  useQuery({ queryKey: qk.platforms, queryFn: api.platforms, staleTime: Infinity })
export const useGames = () => useQuery({ queryKey: qk.games, queryFn: () => api.games() })
export const useGame = (id: number) =>
  useQuery({ queryKey: qk.game(id), queryFn: () => api.game(id), enabled: id > 0 })
export const useEmulators = () => useQuery({ queryKey: qk.emulators, queryFn: api.emulators })
export const useSettings = () => useQuery({ queryKey: qk.settings, queryFn: api.settings })

// useCurrentProfileUid returns the auto-provisioned single-instance profile
// id, exposed by the backend in /status. Components don't need to know a
// profile model exists — favorites/history just flow through this id.
export function useCurrentProfileUid(): string {
  const s = useStatus()
  return s.data?.defaultProfileUid ?? ""
}

export function useHistory() {
  const uid = useCurrentProfileUid()
  return useQuery({
    queryKey: qk.history(uid),
    queryFn: () => api.history(uid),
    enabled: !!uid,
  })
}

export function useFavorites() {
  const uid = useCurrentProfileUid()
  return useQuery({
    queryKey: qk.favorites(uid),
    queryFn: () => api.favorites(uid),
    enabled: !!uid,
  })
}

// useToggleFavorite returns a mutation that flips a game's favorite state.
export function useToggleFavorite(gameId: number, currentlyFav: boolean) {
  const uid = useCurrentProfileUid()
  const qc = useQueryClient()
  return useMutation({
    mutationFn: () =>
      currentlyFav ? api.removeFavorite(uid, gameId) : api.addFavorite(uid, gameId),
    onSuccess: () => qc.invalidateQueries({ queryKey: qk.favorites(uid) }),
  })
}

// useScanControl drives the library scan: it triggers a scan, polls the live
// progress (faster while running), and refreshes the game grid once a scan
// finishes so newly-indexed ROMs appear without a manual reload.
export function useScanControl() {
  const qc = useQueryClient()
  const prevRunning = useRef(false)

  const statusQ = useQuery({
    queryKey: qk.scanStatus,
    queryFn: api.scanStatus,
    refetchInterval: (query) => (query.state.data?.running ? 1000 : 4000),
  })
  const running = statusQ.data?.running ?? false

  useEffect(() => {
    if (prevRunning.current && !running) {
      qc.invalidateQueries({ queryKey: qk.games })
      qc.invalidateQueries({ queryKey: qk.status })
    }
    prevRunning.current = running
  }, [running, qc])

  const scanM = useMutation({
    mutationFn: api.scan,
    onSuccess: () => {
      prevRunning.current = true
      qc.invalidateQueries({ queryKey: qk.scanStatus })
      qc.invalidateQueries({ queryKey: qk.status })
    },
  })

  return {
    progress: statusQ.data,
    running,
    startScan: () => scanM.mutate(),
    starting: scanM.isPending,
    error: scanM.error as Error | null,
  }
}
