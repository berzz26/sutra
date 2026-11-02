import { Link, NavLink, Outlet } from 'react-router-dom'

export function Layout() {
  return (
    <div className="app-shell">
      <header>
        <Link to="/" className="logo">
          searcher
        </Link>
        <nav>
          <NavLink to="/">Search</NavLink>
          <NavLink to="/status">Status</NavLink>
        </nav>
      </header>
      <main>
        <Outlet />
      </main>
    </div>
  )
}
