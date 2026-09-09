import { useEffect, useCallback } from "react"

interface ShortcutMap {
  [key: string]: () => void
}

export function useKeyboardShortcuts(shortcuts: ShortcutMap) {
  const handler = useCallback((e: KeyboardEvent) => {
    // Don't trigger in input fields
    const target = e.target as HTMLElement
    if (target.tagName === "INPUT" || target.tagName === "TEXTAREA" || target.isContentEditable) return

    const parts: string[] = []
    if (e.metaKey || e.ctrlKey) parts.push("mod")
    if (e.shiftKey) parts.push("shift")
    if (e.altKey) parts.push("alt")
    parts.push(e.key.toLowerCase())
    const combo = parts.join("+")

    if (shortcuts[combo]) {
      e.preventDefault()
      shortcuts[combo]()
    }
  }, [shortcuts])

  useEffect(() => {
    window.addEventListener("keydown", handler)
    return () => window.removeEventListener("keydown", handler)
  }, [handler])
}
