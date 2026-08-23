export function SkeletonLines({ count = 3, widths }: { count?: number; widths?: string[] }) {
  return (
    <div style={{ padding: "var(--sp-3)" }}>
      {Array.from({ length: count }).map((_, i) => (
        <div
          key={i}
          className={`skeleton skeleton-line ${widths?.[i] || (i === count - 1 ? "w60" : "w80")}`}
        />
      ))}
    </div>
  );
}

export function SkeletonTable({ rows = 5, cols = 4 }: { rows?: number; cols?: number }) {
  return (
    <div style={{ padding: "var(--sp-3)" }}>
      {Array.from({ length: rows }).map((_, i) => (
        <div key={i} style={{ display: "flex", gap: "var(--sp-4)", marginBottom: "var(--sp-3)" }}>
          {Array.from({ length: cols }).map((_, j) => (
            <div
              key={j}
              className="skeleton"
              style={{
                height: 14,
                flex: j === 0 ? "0 0 120px" : "1",
                borderRadius: "var(--r-sm)",
              }}
            />
          ))}
        </div>
      ))}
    </div>
  );
}


