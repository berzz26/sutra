import { useEffect, useState } from 'react'
import { getHealth } from '../api/endpoints'
import { ApiError } from '../api/client'

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

  if (error) return <p style={{ color: 'crimson' }}>Backend error: {error}</p>
  if (status === null) return <p>Checking backend…</p>
  return <p>Backend status: {status}</p>
}
