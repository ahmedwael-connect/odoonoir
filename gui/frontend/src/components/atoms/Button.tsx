import * as React from "react"
import { Slot } from "@radix-ui/react-slot"
import { cva, type VariantProps } from "class-variance-authority"
import { clsx } from "clsx"
import { twMerge } from "tailwind-merge"
import { Loader2 } from "lucide-react"

function cn(...inputs: (string | undefined | false)[]) {
  return twMerge(clsx(inputs))
}

const buttonVariants = cva(
  "inline-flex items-center justify-center gap-2 rounded-[14px] text-sm font-semibold tracking-tight transition-all duration-200 ease-[var(--ease-spring)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-background disabled:opacity-40 disabled:pointer-events-none active:scale-[0.97] hover:scale-[1.01] shadow-sm hover:shadow-[0_4px_16px_rgba(0,0,0,0.25)]",
  {
    variants: {
      variant: {
        primary: "bg-primary text-primary-foreground hover:bg-primary/90 shadow-[0_4px_16px_rgba(52,211,153,0.25)] hover:shadow-[0_6px_20px_rgba(52,211,153,0.35)] border border-primary/20",
        secondary: "bg-card text-foreground hover:bg-accent border border-border shadow-[0_2px_8px_rgba(0,0,0,0.15)] hover:shadow-[0_4px_12px_rgba(0,0,0,0.2)]",
        ghost: "bg-transparent hover:bg-accent hover:text-accent-foreground border border-transparent",
        outline: "border border-border bg-card hover:bg-accent hover:text-accent-foreground shadow-sm",
        destructive: "bg-destructive text-destructive-foreground hover:bg-destructive/90 shadow-sm border border-destructive/20",
        success: "bg-success text-white hover:bg-success/90 shadow-sm",
        warning: "bg-warning text-warning-foreground hover:bg-warning/90",
        link: "bg-transparent text-primary underline-offset-4 hover:underline p-0 h-auto shadow-none hover:shadow-none",
        bento: "bg-card text-foreground border border-border rounded-[18px] shadow-[0_4px_20px_rgba(0,0,0,0.3)] hover:shadow-[0_8px_28px_rgba(0,0,0,0.4)] hover:border-primary/20",
        bentoPrimary: "bg-primary text-primary-foreground rounded-[18px] shadow-[0_6px_20px_rgba(52,211,153,0.3)] hover:shadow-[0_8px_28px_rgba(52,211,153,0.4)]",
      },
      size: {
        sm: "h-8 px-3.5 text-xs rounded-[12px]",
        md: "h-9 px-5 py-2 rounded-[14px]",
        lg: "h-11 px-7 text-base rounded-[16px]",
        icon: "h-9 w-9 p-0 rounded-[12px]",
        "icon-sm": "h-8 w-8 p-0 rounded-[12px]",
        bento: "h-12 px-6 text-sm rounded-[16px]",
      },
    },
    defaultVariants: {
      variant: "secondary",
      size: "md",
    },
  }
)

export interface ButtonProps
  extends React.ButtonHTMLAttributes<HTMLButtonElement>,
    VariantProps<typeof buttonVariants> {
  asChild?: boolean
  loading?: boolean
  iconLeft?: React.ReactNode
  iconRight?: React.ReactNode
}

const Button = React.forwardRef<HTMLButtonElement, ButtonProps>(
  ({ className, variant, size, asChild = false, loading, iconLeft, iconRight, children, disabled, ...props }, ref) => {
    const Comp = asChild ? Slot : "button"
    return (
      <Comp
        className={cn(buttonVariants({ variant, size, className }))}
        ref={ref}
        disabled={disabled || loading}
        aria-busy={loading || undefined}
        {...props}
      >
        {loading ? <Loader2 className="h-4 w-4 animate-spin" /> : iconLeft}
        {children}
        {!loading && iconRight}
      </Comp>
    )
  }
)
Button.displayName = "Button"

export { Button, buttonVariants, cn }
