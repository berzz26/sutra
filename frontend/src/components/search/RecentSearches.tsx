import { useMemo, useState } from 'react'
import { Bookmark, ChevronLeft, ChevronRight, Clock3, Video as VideoIcon } from 'lucide-react'
import type { RecentSearch } from '../../lib/recentSearches'
import { formatMs } from '../../lib/format'

const PAGE_SIZE = 2

export function RecentSearches({ items, onRun }: { items: RecentSearch[]; onRun: (entry: RecentSearch) => void }) {
  const [offset, setOffset] = useState(0)

  const visible = useMemo(() => {
    if (items.length <= PAGE_SIZE) return items
    const safe = offset % items.length
    return Array.from({ length: Math.min(PAGE_SIZE, items.length) }, (_, index) => items[(safe + index) % items.length])
  }, [items, offset])

  if (items.length === 0) return null

  const multiplePages = items.length > PAGE_SIZE
  const canGoBack = multiplePages && (offset % items.length !== 0)

  return (
    <section className="recent-section" aria-labelledby="recent-heading">
      <div className="section-heading">
        <h2 id="recent-heading"><Bookmark size={17} /> Recent searches</h2>
      </div>
      <div className="recent-grid">
        {visible.map((entry) => <RecentSearchCard key={entry.id} entry={entry} onRun={() => onRun(entry)} />)}
        <div className="recent-nav">
          {canGoBack && (
            <button type="button" className="recent-next" aria-label="Previous recent searches" onClick={() => setOffset((value) => value - 1)}>
              <ChevronLeft size={17} />
            </button>
          )}
          {multiplePages && (
            <button type="button" className="recent-next" aria-label="More recent searches" onClick={() => setOffset((value) => value + 1)}>
              <ChevronRight size={17} />
            </button>
          )}
        </div>
      </div>
    </section>
  )
}

function RecentSearchCard({ entry, onRun }: { entry: RecentSearch; onRun: () => void }) {
  return (
    <button type="button" className="recent-card" onClick={onRun}>
      <span className="recent-thumb">
        {entry.thumbnailUrl
          ? <img src={entry.thumbnailUrl} alt="" loading="lazy" onError={(event) => { event.currentTarget.style.display = 'none' }} />
          : <span className="recent-fallback"><VideoIcon size={22} /></span>}
        {entry.durationMs ? <span className="recent-duration">{formatMs(entry.durationMs)}</span> : null}
      </span>
      <span className="recent-copy">
        <span className="recent-query">{entry.videoTitle || entry.query}</span>
        {entry.videoTitle && <span className="recent-sub">{entry.query}</span>}
        <span className="recent-meta">
          <span>{entry.scope === 'video' || entry.sourceType === 'youtube' ? 'YouTube' : 'Library'}</span>
          <span className="divider" aria-hidden="true" />
          {entry.durationMs ? <><Clock3 size={11} /> {formatMs(entry.durationMs)}</> : <span>Search</span>}
        </span>
      </span>
    </button>
  )
}
