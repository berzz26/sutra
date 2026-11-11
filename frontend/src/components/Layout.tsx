import { Outlet } from 'react-router-dom'
import { Navbar } from './Navbar'

export function Layout() {
  return (
    <div className="app-shell">
      <div className="ambient" aria-hidden="true" />
      <Navbar />
      <main className="page-frame"><Outlet /></main>
      <footer className="app-footer">
        <span>Local-first semantic video search</span>
        <span className="divider" aria-hidden="true" />
        <span>Your archive stays on your machine</span>
      </footer>
    </div>
  )
}
