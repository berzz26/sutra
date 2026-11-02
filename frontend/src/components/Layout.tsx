import { Moon, Search, Sun, Video } from 'lucide-react'
import { useEffect, useState } from 'react'
import { Link, NavLink, Outlet } from 'react-router-dom'
import { Button } from './ui'

export function Layout() {
  const [dark, setDark] = useState(() => localStorage.getItem('sutra-theme') === 'dark')

  useEffect(() => {
    document.documentElement.dataset.theme = dark ? 'dark' : 'light'
    localStorage.setItem('sutra-theme', dark ? 'dark' : 'light')
  }, [dark])

  return (
    <div className="app-shell">
      <header className="app-header">
        <div className="header-inner">
          <Link to="/" className="brand" aria-label="Sutra home">
            <span className="brand-mark"><Video size={16} strokeWidth={2.5} /></span>
            <span>sutra<span className="brand-dot">.</span></span>
          </Link>
          <nav className="main-nav" aria-label="Primary navigation">
            <NavLink to="/" end><Search size={15} />Search</NavLink>
            <a href="/#library"><Video size={15} />Library</a>
            <NavLink to="/status">System status</NavLink>
          </nav>
          <Button variant="ghost" size="icon" aria-label={dark ? 'Switch to light theme' : 'Switch to dark theme'} onClick={() => setDark((value) => !value)}>
            {dark ? <Sun size={17} /> : <Moon size={17} />}
          </Button>
        </div>
      </header>
      <main className="page-frame"><Outlet /></main>
      <footer className="app-footer"><span>Local-first semantic video search</span><span>Built for your own archive</span></footer>
    </div>
  )
}
