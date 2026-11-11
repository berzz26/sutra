import { useState, type CSSProperties } from 'react'
import { ArrowUpRight, Clock3, Video as VideoIcon } from 'lucide-react'
import type { SearchResult, Video } from '../../api/endpoints'
import { buildTranscriptExcerpt, escapeRegExp, formatMs, meaningfulQueryTerms } from '../../lib/format'

export function ResultCard({ result, video, query, index }: { result: SearchResult; video?: Video; query: string; index: number }) {
  const link = result.timestamp_url || result.original_url || video?.original_url || '#'
  const excerpt = buildTranscriptExcerpt(result.text, query)
  return (
    <article className="result-card" style={{ '--delay': `${index * 55}ms` } as CSSProperties}>
      <div className="result-media">
        <Thumbnail src={video?.thumbnail_url} />
        <span className="result-timestamp"><Clock3 size={11} /> {formatMs(result.start_ms)}</span>
      </div>
      <div className="result-content">
        <div className="result-meta">
          <span>{video?.source_type === 'youtube' || result.original_url ? 'YouTube' : 'Transcript file'}</span>
          {video?.channel && <span>{video.channel}</span>}
          {video?.language && <span>{video.language}</span>}
        </div>
        <h3>{result.video_title || video?.title || 'Untitled video'}</h3>
        <p className="result-excerpt"><Highlight text={excerpt} query={query} /></p>
        <div className="result-footer">
          <span>{formatMs(result.start_ms)} – {formatMs(result.end_ms)}</span>
          <a className="result-open" href={link} target="_blank" rel="noreferrer">Open moment <ArrowUpRight size={13} /></a>
        </div>
      </div>
    </article>
  )
}

function Thumbnail({ src }: { src?: string }) {
  const [failed, setFailed] = useState(!src)
  if (failed || !src) return <div className="thumbnail-fallback"><VideoIcon size={24} /><span>Video moment</span></div>
  return <img src={src} alt="" loading="lazy" onError={() => setFailed(true)} />
}

function Highlight({ text, query }: { text: string; query: string }) {
  const terms = meaningfulQueryTerms(query)
  if (!terms.length) return <>{text}</>
  const pattern = new RegExp(`\\b(${terms.map(escapeRegExp).join('|')})\\b`, 'gi')
  return <>{text.split(pattern).map((part, index) => terms.some((term) => part.toLocaleLowerCase() === term) ? <mark key={index}>{part}</mark> : part)}</>
}
