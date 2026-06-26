import { Loader2 } from "lucide-react"

import { cn } from "@/lib/utils"

/** Inline loading spinner. Prefer Skeleton for content-area loading; use this
 *  for buttons, inline "loading more", and small affordances. */
function Spinner({ className }: { className?: string }) {
  return <Loader2 className={cn("size-4 animate-spin", className)} />
}

export { Spinner }
