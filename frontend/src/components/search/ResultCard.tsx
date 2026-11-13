import { memo, useMemo, useState, type CSSProperties } from 'react'
import { ArrowUpRight, Clock3, Video as VideoIcon } from 'lucide-react'
import type { SearchResult, Video } from '../../api/endpoints'
import { buildTranscriptExcerpt, escapeRegExp, formatMs, meaningfulQueryTerms } from '../../lib/format'

export const ResultCard = memo(function ResultCard({
  result,
  video,
  query,
  index,
}: {
  result: SearchResult
  video?: Video
  query: string
  index: number
}) {
  const link = result.timestamp_url || result.original_url || video?.original_url || '#'

  // FIX: memoize excerpt — buildTranscriptExcerpt scans potentially long
  // transcript text; recomputing on every render (especially per keystroke)
  // was the main freeze trigger with multi-word queries.
  const excerpt = useMemo(
    () => buildTranscriptExcerpt(result.text, query),
    [result.text, query],
  )

  return (
    <article className="result-card" style={{ '--delay': `${index * 55}ms` } as CSSProperties}>
      <div className="result-media">
        <Thumbnail src={video?.thumbnail_url} />
        <span className="result-timestamp">
          <Clock3 size={11} /> {formatMs(result.start_ms)}
        </span>
      </div>
      <div className="result-content">
        <div className="result-meta">
          <span>
            {video?.source_type === 'youtube' || result.original_url ? 'YouTube' : 'Transcript file'}
          </span>
          {video?.channel && <span>{video.channel}</span>}
          {video?.language && <span>{video.language}</span>}
        </div>
        <h3>{result.video_title || video?.title || 'Untitled video'}</h3>
        <p className="result-excerpt">
          <Highlight text={excerpt} query={query} />
        </p>
        <div className="result-footer">
          <span>
            {formatMs(result.start_ms)} – {formatMs(result.end_ms)}
          </span>
          <a className="result-open" href={link} target="_blank" rel="noreferrer">
            Open moment <ArrowUpRight size={13} />
          </a>
        </div>
      </div>
    </article>
  )
})

// Thumbnail is stable as long as src doesn't change — memo prevents
// re-mounting the img (and re-firing onError) on parent re-renders.
const Thumbnail = memo(function Thumbnail({ src }: { src?: string }) {
  const [failed, setFailed] = useState(!src)
  if (failed || !src)
    return (
      <div className="thumbnail-fallback">
        <VideoIcon size={24} />
        <span>Video moment</span>
      </div>
    )
  return <img src={src} alt="" loading="lazy" onError={() => setFailed(true)} />
})

// FIX: memoize the entire Highlight component.
// Previously: new RegExp + text.split on every render, for every card, for
// every keystroke. With 3+ query words the regex alternation is O(terms × textLength)
// and ran 10× (one per result card) on every single character typed.
// Now it only recomputes when text or query actually changes.
const Highlight = memo(function Highlight({ text, query }: { text: string; query: string }) {
  const segments = useMemo(() => {
    const terms = meaningfulQueryTerms(query)
    if (!terms.length) return null

    const pattern = new RegExp(`\\b(${terms.map(escapeRegExp).join('|')})\\b`, 'gi')
    const parts = text.split(pattern)
    const termSet = new Set(terms) // O(1) lookup instead of .some() O(n) per part

    return parts.map((part, i) =>
      termSet.has(part.toLocaleLowerCase())
        ? <mark key={i}>{part}</mark>
        : part,
    )
  }, [text, query])

  if (!segments) return <>{text}</>
  return <>{segments}</>
})