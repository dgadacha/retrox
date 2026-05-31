import { useMutation } from "@tanstack/react-query"
import { Play } from "lucide-react"
import { useState } from "react"

import { Button, Spinner } from "@/components/ui"
import { api } from "@/lib/api"
import type { PlayResolved } from "@/lib/types"

interface Props {
  gameId: number
  disabled?: boolean
  size?: "md" | "lg"
}

// PlayButton fires POST /games/:id/play and surfaces what the backend
// actually launched (e.g. "RetroArch · snes9x") or the error if the
// emulator wasn't found.
export function PlayButton({ gameId, disabled, size = "md" }: Props) {
  const [msg, setMsg] = useState<{ ok: boolean; text: string } | null>(null)

  const playM = useMutation({
    mutationFn: () => api.play(gameId),
    onSuccess: (r: PlayResolved) => setMsg({ ok: true, text: `Lancé — ${r.display}` }),
    onError: (e) => setMsg({ ok: false, text: (e as Error).message }),
  })

  return (
    <div className="flex flex-col gap-1.5">
      <Button
        variant="success"
        size={size}
        disabled={disabled || playM.isPending}
        onClick={() => playM.mutate()}
      >
        {playM.isPending ? (
          <Spinner className="h-4 w-4" />
        ) : (
          <Play className="h-4 w-4" strokeWidth={2.5} fill="currentColor" />
        )}
        Jouer
      </Button>
      {msg && (
        <span className={msg.ok ? "text-xs text-emerald-400" : "text-xs text-red-400"}>
          {msg.text}
        </span>
      )}
    </div>
  )
}
