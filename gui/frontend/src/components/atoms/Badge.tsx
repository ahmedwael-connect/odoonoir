import * as React from "react"
import { cva, type VariantProps } from "class-variance-authority"
import { cn } from "./Button"

const badgeVariants = cva(
  "inline-flex items-center gap-1.5 rounded-full px-2.5 py-0.5 text-xs font-medium border transition-colors",
  {
    variants: {
      variant: {
        default: "bg-muted text-muted-foreground border-transparent",
        primary: "bg-primary/10 text-primary border-primary/20",
        success: "bg-success/10 text-success border-success/20",
        warning: "bg-warning/10 text-warning border-warning/20",
        destructive: "bg-destructive/10 text-destructive border-destructive/20",
        info: "bg-info/10 text-info border-info/20",
        outline: "bg-transparent border-border text-foreground",
      },
    },
    defaultVariants: { variant: "default" },
  }
)

export function Badge({ className, variant, ...props }: React.HTMLAttributes<HTMLDivElement> & VariantProps<typeof badgeVariants>) {
  return <div className={cn(badgeVariants({ variant }), className)} {...props} />
}

export function StatusDot({ running, healthColor, title }: { running: boolean; healthColor?: string; title?: string }) {
  const color = healthColor || (running ? "var(--success)" : "var(--destructive)")
  const label = title || (running ? "running" : "stopped")
  return (
    <span
      className="inline-flex h-2 w-2 rounded-full shrink-0"
      style={{ background: color, boxShadow: `0 0 6px ${color}` }}
      aria-label={label}
      title={label}
    />
  )
}
