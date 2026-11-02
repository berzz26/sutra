import { useEffect, useState } from 'react'
import { getHealth } from '../api/endpoints'
import { ApiError } from '../api/client'
import { Activity, Loader2 } from 'lucide-react'
import { Card } from '../components/ui'

export function StatusPage() {
  const [status, setStatus] = useState<string | null>(null)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    let cancelled = false
    getHealth()
      .then((result) => {
        if (!cancelled) setStatus(result.status)
      })
      .catch((cause: unknown) => {
        if (!cancelled) {
          setError(cause instanceof ApiError ? cause.message : String(cause))
        }
      })
    return () => {
      cancelled = true
    }
  }, [])

  return <section className="status-page"><p className="eyebrow">System status</p><Card className="status-card"><Activity size={22} color="var(--accent)" /><h1>Everything in one place.</h1><p className="hero-description">A quick check on the local Searcher service and its indexing pipeline.</p>{error ? <div className="status-indicator error-copy">Backend error: {error}</div> : status === null ? <div className="status-indicator"><Loader2 className="spin" size={15} /> Checking backend…</div> : <div className="status-indicator"><span className="status-dot" /> Backend {status}</div>}</Card></section>
}
