import React from "react"

export function AppShell({ header, lift, topnav, main, eventLog }: { header: React.ReactNode; lift: React.ReactNode; topnav?: React.ReactNode; main: React.ReactNode; eventLog: React.ReactNode }) {
  return (
    <div className="flex flex-col h-screen bg-background text-foreground overflow-hidden">
      <div className="p-3 pb-0">
        {header}
      </div>
      {topnav && <div className="px-3 pt-3">{topnav}</div>}
      <div className="flex flex-1 min-h-0 gap-3 p-3 bg-background">
        {lift}
        <main className="flex-1 overflow-y-auto min-w-0 bg-transparent rounded-[24px]" role="main" aria-label="Instance details">
          <div className="space-y-4">
            {main}
          </div>
        </main>
        {eventLog}
      </div>
    </div>
  )
}
