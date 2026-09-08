import { useEffect, useRef, useState, useCallback, useMemo } from "react"
import { Button } from "./components/atoms/Button"

export interface PaletteItem {
  id: string
  label: string
  category: "instance" | "screen" | "action"
  hint?: string
  icon?: string
  action: () => void
}

interface CommandPaletteProps {
  open: boolean
  onClose: () => void
  items: PaletteItem[]
}

function fuzzyMatch(query: string, text: string): boolean {
  if (!query) return true
  const q = query.toLowerCase()
  const t = text.toLowerCase()
  let qi = 0
  for (let ti = 0; ti < t.length && qi < q.length; ti++) {
    if (t[ti] === q[qi]) qi++
  }
  return qi === q.length
}

export function CommandPalette({ open, onClose, items }: CommandPaletteProps) {
  const [query, setQuery] = useState("")
  const [selectedIdx, setSelectedIdx] = useState(0)
  const inputRef = useRef<HTMLInputElement>(null)

  const filtered = useMemo(() => {
    if (!query) return items
    return items.filter((item) =>
      fuzzyMatch(query, item.label) || fuzzyMatch(query, item.category) || (item.hint && fuzzyMatch(query, item.hint))
    )
  }, [items, query])

  // Group by category
  const grouped = useMemo(() => {
    const groups: Record<string, PaletteItem[]> = {}
    for (const item of filtered) {
      const cat = item.category
      if (!groups[cat]) groups[cat] = []
      groups[cat].push(item)
    }
    return groups
  }, [filtered])

  const flatList = useMemo(() => filtered, [filtered])

  useEffect(() => {
    if (open) {
      setQuery("")
      setSelectedIdx(0)
      setTimeout(() => inputRef.current?.focus(), 50)
    }
  }, [open])

  useEffect(() => {
    setSelectedIdx(0)
  }, [query])

  const handleKeyDown = useCallback(
    (e: React.KeyboardEvent) => {
      if (e.key === "ArrowDown") {
        e.preventDefault()
        setSelectedIdx((i) => Math.min(i + 1, flatList.length - 1))
      } else if (e.key === "ArrowUp") {
        e.preventDefault()
        setSelectedIdx((i) => Math.max(i - 1, 0))
      } else if (e.key === "Enter") {
        e.preventDefault()
        if (flatList[selectedIdx]) {
          flatList[selectedIdx].action()
          onClose()
        }
      } else if (e.key === "Escape") {
        onClose()
      }
    },
    [flatList, selectedIdx, onClose]
  )

  if (!open) return null

  const categoryLabels: Record<string, string> = {
    instance: "Instances",
    screen: "Screens",
    action: "Actions",
  }

  let flatIdx = -1

  return (
    <div className="fixed inset-0 z-50 flex items-start justify-center pt-[15vh]" onClick={onClose}>
      <div className="fixed inset-0 bg-black/50 backdrop-blur-sm" />
      <div
        className="relative w-full max-w-lg bg-card rounded-2xl shadow-2xl border overflow-hidden"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="flex items-center gap-3 px-4 py-3 border-b">
          <span className="text-muted-foreground text-sm">⌘K</span>
          <input
            ref={inputRef}
            className="flex-1 bg-transparent outline-none text-sm"
            placeholder="Search instances, screens, actions…"
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            onKeyDown={handleKeyDown}
          />
          <Button variant="ghost" size="sm" onClick={onClose}>
            Esc
          </Button>
        </div>
        <div className="max-h-80 overflow-y-auto p-2">
          {flatList.length === 0 && (
            <div className="text-center text-muted-foreground text-sm py-8">No results</div>
          )}
          {Object.entries(grouped).map(([cat, catItems]) => (
            <div key={cat}>
              <div className="text-xs text-muted-foreground font-medium px-2 py-1 mt-1">
                {categoryLabels[cat] || cat}
              </div>
              {catItems.map((item) => {
                flatIdx++
                const idx = flatIdx
                return (
                  <button
                    key={item.id}
                    className={`w-full text-left px-3 py-2 rounded-lg text-sm flex items-center gap-2 transition-colors ${
                      idx === selectedIdx ? "bg-accent text-accent-foreground" : "hover:bg-muted"
                    }`}
                    onClick={() => { item.action(); onClose() }}
                    onMouseEnter={() => setSelectedIdx(idx)}
                  >
                    {item.icon && <span className="text-base">{item.icon}</span>}
                    <span className="flex-1 truncate">{item.label}</span>
                    {item.hint && (
                      <span className="text-xs text-muted-foreground truncate max-w-[200px]">{item.hint}</span>
                    )}
                  </button>
                )
              })}
            </div>
          ))}
        </div>
        <div className="border-t px-4 py-2 text-xs text-muted-foreground flex gap-4">
          <span>↑↓ navigate</span>
          <span>↵ select</span>
          <span>esc close</span>
        </div>
      </div>
    </div>
  )
}
