import { ChartNoAxesColumnIncreasing, Folder, Moon, Search, Sun } from 'lucide-react'
import { useEffect, useState } from 'react'
import { Link, useLocation } from 'react-router-dom'

function isActivePath(pathname: string, search: string, path: string, exact?: boolean) {
  if (pathname !== path) return false
  if (exact) return search === '' || !search.includes('tab=')
  return search.includes('tab=library')
}

export function Navbar() {
  const { pathname, search } = useLocation()
  const [dark, setDark] = useState(() => localStorage.getItem('sutra-theme') === 'dark')

  useEffect(() => {
    document.documentElement.dataset.theme = dark ? 'dark' : 'light'
    localStorage.setItem('sutra-theme', dark ? 'dark' : 'light')
  }, [dark])

  return (
    <header className="app-header">
      <div className="header-inner">
        <Link to="/" className="brand" aria-label="Sutra home">
          <span className="brand-mark">
            <svg
              width="15"
              height="15"
              viewBox="0 0 24 24"
              fill={dark ? '#17191F' : '#FFFFFF'}
              aria-hidden="true"
            >
              <rect x="2" y="5" width="14" height="14" rx="3.5" />
              <path d="M17 10.2l4.1-2.4a1 1 0 0 1 1.5.87v6.66a1 1 0 0 1-1.5.87L17 13.8z" />
            </svg>
          </span>
          <span className="brand-name">sutra<span className="brand-dot">.</span></span>
        </Link>
        <nav className="main-nav" aria-label="Primary navigation">
          <Link to="/" className={`nav-item ${isActivePath(pathname, search, '/', true) ? 'active' : ''}`}>
            <Search size={15} /><span className="nav-label">Search</span>
          </Link>
          <Link to="/?tab=library" className={`nav-item ${isActivePath(pathname, search, '/') ? 'active' : ''}`}>
            <Folder size={15} /><span className="nav-label">Library</span>
          </Link>
          <Link to="/status" className={`nav-item ${pathname === '/status' ? 'active' : ''}`}>
            <ChartNoAxesColumnIncreasing size={15} /><span className="nav-label">System</span>
          </Link>
        </nav>
        <div className="header-actions">
          <button type="button" className="icon-button" aria-label={dark ? 'Switch to light theme' : 'Switch to dark theme'} onClick={() => setDark((value) => !value)}>
            {dark ? <Sun size={17} /> : <Moon size={17} />}
          </button>
          {/* <button type="button" className="avatar" aria-label="Profile" title="Local workspace">A</button> */}
        </div>
      </div>
    </header>
  )
}
