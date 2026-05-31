import { BrowserRouter, Navigate, Outlet, Route, Routes } from "react-router-dom"

import { CatalogueDetailPage } from "@/components/CatalogueDetail"
import { CataloguePage } from "@/components/Catalogue"
import { DownloadsPage } from "@/components/DownloadsPage"
import { GameDetailPage } from "@/components/GameDetail"
import { LibraryPage } from "@/components/Library"
import { SettingsPage } from "@/components/SettingsPage"
import { Sidebar } from "@/components/Sidebar"
import { Splash } from "@/components/ui"
import { useStatus } from "@/lib/hooks"

export function App() {
  return (
    <BrowserRouter>
      <Routes>
        <Route element={<AppShell />}>
          <Route path="/" element={<LibraryPage />} />
          <Route path="/game/:id" element={<GameDetailPage />} />
          <Route path="/catalogue" element={<CataloguePage />} />
          <Route path="/catalogue/:id" element={<CatalogueDetailPage />} />
          <Route path="/downloads" element={<DownloadsPage />} />
          <Route path="/settings" element={<SettingsPage />} />
          <Route path="*" element={<Navigate to="/" replace />} />
        </Route>
      </Routes>
    </BrowserRouter>
  )
}

// AppShell composes the persistent sidebar with the routed content. We
// gate it on /status so the backend's defaultProfileUid is available
// everywhere downstream without a null-check dance.
function AppShell() {
  const status = useStatus()
  if (!status.data) return <Splash />
  return (
    <div className="flex min-h-screen">
      <Sidebar />
      <main className="flex min-h-screen flex-1 flex-col overflow-x-hidden">
        <Outlet />
      </main>
    </div>
  )
}
