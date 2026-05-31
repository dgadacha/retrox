import clsx from "clsx"
import type { ButtonHTMLAttributes, ReactNode } from "react"
import { useEffect } from "react"

export function Spinner({ className }: { className?: string }) {
  return (
    <div
      className={clsx(
        "inline-block animate-spin rounded-full border-2 border-ink-600 border-t-accent-500",
        className ?? "h-5 w-5",
      )}
    />
  )
}

export function Wordmark({ className }: { className?: string }) {
  return (
    <span className={clsx("select-none font-black tracking-tight", className)}>
      <span className="text-text-100">RETRO</span>
      <span className="text-gradient">X</span>
    </span>
  )
}

export function Splash({ label = "Chargement…" }: { label?: string }) {
  return (
    <div className="flex min-h-screen flex-col items-center justify-center gap-5">
      <Wordmark className="text-5xl" />
      <Spinner className="h-7 w-7" />
      <p className="text-sm text-text-500">{label}</p>
    </div>
  )
}

type ButtonProps = ButtonHTMLAttributes<HTMLButtonElement> & {
  variant?: "primary" | "ghost" | "subtle" | "success" | "danger"
  size?: "sm" | "md" | "lg"
}

export function Button({ variant = "subtle", size = "md", className, ...rest }: ButtonProps) {
  const styles = {
    primary:
      "bg-accent-500 hover:bg-accent-400 text-white shadow-[0_6px_20px_-6px_rgba(139,92,246,0.6)]",
    ghost: "bg-white/5 hover:bg-white/10 text-text-100 backdrop-blur-sm",
    subtle: "bg-ink-700 hover:bg-ink-600 text-text-100",
    success:
      "bg-success hover:bg-emerald-400 text-white shadow-[0_6px_20px_-6px_rgba(16,185,129,0.55)]",
    danger: "bg-danger/90 hover:bg-danger text-white",
  }
  const sizes = {
    sm: "px-3 py-1.5 text-xs",
    md: "px-4 py-2 text-sm",
    lg: "px-6 py-3 text-base",
  }
  return (
    <button
      className={clsx(
        "inline-flex items-center justify-center gap-2 rounded-md font-semibold transition disabled:cursor-not-allowed disabled:opacity-50",
        styles[variant],
        sizes[size],
        className,
      )}
      {...rest}
    />
  )
}

export const inputClass =
  "w-full rounded-md border border-ink-600 bg-ink-800 px-3 py-2 text-sm text-text-100 outline-none transition placeholder:text-text-700 focus:border-accent-500 focus:ring-2 focus:ring-accent-500/25"

export function Modal({ children, onClose }: { children: ReactNode; onClose: () => void }) {
  useEffect(() => {
    function onKey(e: KeyboardEvent) {
      if (e.key === "Escape") onClose()
    }
    document.addEventListener("keydown", onKey)
    document.body.style.overflow = "hidden"
    return () => {
      document.removeEventListener("keydown", onKey)
      document.body.style.overflow = ""
    }
  }, [onClose])

  return (
    <div
      className="fixed inset-0 z-50 flex items-start justify-center overflow-y-auto bg-black/80 p-4 backdrop-blur-sm sm:p-8"
      onClick={onClose}
    >
      <div
        className="relative my-2 w-full max-w-3xl overflow-hidden rounded-xl bg-ink-900 shadow-card sm:my-6"
        onClick={(e) => e.stopPropagation()}
      >
        {children}
      </div>
    </div>
  )
}

export function Badge({
  children,
  tone = "neutral",
}: {
  children: ReactNode
  tone?: "neutral" | "accent" | "success" | "warn" | "danger"
}) {
  const tones = {
    neutral: "bg-ink-700 text-text-300",
    accent: "bg-accent-500/15 text-accent-300 ring-1 ring-inset ring-accent-500/40",
    success: "bg-success/15 text-emerald-300 ring-1 ring-inset ring-success/40",
    warn: "bg-warn/15 text-amber-300 ring-1 ring-inset ring-warn/40",
    danger: "bg-danger/15 text-red-300 ring-1 ring-inset ring-danger/40",
  }
  return (
    <span
      className={clsx(
        "inline-flex items-center rounded-md px-2 py-0.5 text-[11px] font-semibold uppercase tracking-wide",
        tones[tone],
      )}
    >
      {children}
    </span>
  )
}
