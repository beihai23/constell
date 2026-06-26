import type { LucideIcon } from "lucide-react"

import { cn } from "@/lib/utils"
import { Button } from "@/components/ui/button"

// =============================================================================
// EmptyState — centered "nothing here" affordance: icon + title + optional
// description + optional action. Replaces hand-rolled "No messages yet" etc.
// so every empty surface looks and reads consistently.
// =============================================================================

interface EmptyStateProps {
  icon?: LucideIcon
  title: string
  description?: string
  action?: { label: string; onClick: () => void }
  className?: string
  /** Compact variant for tight lists (smaller text, less padding). */
  compact?: boolean
}

function EmptyState({
  icon: Icon,
  title,
  description,
  action,
  className,
  compact,
}: EmptyStateProps) {
  return (
    <div
      role="status"
      data-slot="empty-state"
      className={cn(
        "flex flex-1 flex-col items-center justify-center gap-1 text-center",
        compact ? "p-4" : "p-8",
        className,
      )}
    >
      {Icon && (
        <Icon
          className={cn(
            "shrink-0 text-muted-foreground",
            compact ? "size-5" : "size-7",
          )}
        />
      )}
      <p
        className={cn(
          "text-muted-foreground",
          compact ? "text-xs" : "text-sm",
        )}
      >
        {title}
      </p>
      {description && (
        <p className="text-xs text-muted-foreground/70">{description}</p>
      )}
      {action && (
        <Button
          variant="outline"
          size="sm"
          className="mt-2"
          onClick={action.onClick}
        >
          {action.label}
        </Button>
      )}
    </div>
  )
}

// =============================================================================
// ErrorState — inline error affordance with a Retry action. Use for failed
// fetches that leave a surface empty (message history, member list, search).
// Distinguished from a toast: this is for errors the user must recover from
// to use the surface.
// =============================================================================

interface ErrorStateProps {
  title?: string
  description?: string
  retry?: () => void
  busy?: boolean
  className?: string
  compact?: boolean
}

function ErrorState({
  title = "Something went wrong",
  description = "Please try again.",
  retry,
  busy,
  className,
  compact,
}: ErrorStateProps) {
  return (
    <div
      role="alert"
      data-slot="error-state"
      className={cn(
        "flex flex-1 flex-col items-center justify-center gap-1 text-center",
        compact ? "p-4" : "p-8",
        className,
      )}
    >
      <p
        className={cn(
          "text-destructive",
          compact ? "text-xs" : "text-sm",
        )}
      >
        {title}
      </p>
      <p className="text-xs text-muted-foreground">{description}</p>
      {retry && (
        <Button
          variant="outline"
          size="sm"
          className="mt-2"
          onClick={retry}
          disabled={busy}
        >
          {busy ? "Retrying…" : "Retry"}
        </Button>
      )}
    </div>
  )
}

export { EmptyState, ErrorState }
