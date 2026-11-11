import { memo, useEffect, useState } from 'react'
import { ArrowUpRight, Copy, Download, Loader2 } from 'lucide-react'
import { toast } from 'sonner'
import type { Cue, Video } from '../../api/endpoints'
import { Badge, Button } from '../ui'
import { copyToClipboard, downloadTextFile, formatMs, formatTranscript, safeFilename } from '../../lib/format'
import { Video as VideoIcon } from 'lucide-react'

function Thumbnail({ src }: { src?: string }) {
  const [failed, setFailed] = useState(!src)
  if (failed || !src) return <div className="thumbnail-fallback"><VideoIcon size={24} /><span>No preview</span></div>
  return <img src={src} alt="" loading="lazy" onError={() => setFailed(true)} />
}

export const BrowseMetadata = memo(function BrowseMetadata({ video }: { video: Video }) {
  const items: Array<[string, string]> = [
    ['Source', video.source_type === 'youtube' ? 'YouTube' : 'Transcript file'],
    ['Channel', video.channel || '—'],
    ['Duration', video.duration_ms ? formatMs(video.duration_ms) : '—'],
    ['Language', video.language || '—'],
    ['Cues', String(video.transcript?.cue_count ?? 0)],
    ['Extraction', video.transcript?.extraction_method ?? '—'],
    ['Indexed', new Date(video.created_at).toLocaleDateString(undefined, { year: 'numeric', month: 'short', day: 'numeric' })],
  ]
  const link = video.original_url || (video.source_type === 'youtube' ? `https://www.youtube.com/watch?v=${video.source_ref}` : null)
  return (
    <section className="browse-metadata" aria-label="Video metadata">
      <div className="browse-metadata-media"><Thumbnail src={video.thumbnail_url} /></div>
      <div className="browse-metadata-body">
        <div className="browse-metadata-title">
          <h3 title={video.title}>{video.title || 'Untitled video'}</h3>
          <Badge tone={video.status === 'ready' || video.status === 'succeeded' ? 'success' : 'muted'}>{video.status}</Badge>
        </div>
        <dl className="browse-meta-grid">{items.map(([label, value]) => <div key={label}><dt>{label}</dt><dd title={value}>{value}</dd></div>)}</dl>
        {link && <a className="browse-original-link" href={link} target="_blank" rel="noreferrer">Open original <ArrowUpRight size={13} /></a>}
      </div>
    </section>
  )
})

const RENDER_BATCH = 400

// Full transcripts can exceed 15k cues; render progressively while
// copy/download always encode the complete set.
export const BrowseTranscript = memo(function BrowseTranscript({ video, cues, loading }: { video: Video; cues: Cue[]; loading: boolean }) {
  const [renderLimit, setRenderLimit] = useState(RENDER_BATCH)
  useEffect(() => setRenderLimit(RENDER_BATCH), [video.id])
  const format = video.transcript?.original_format === 'srt' ? 'srt' : 'vtt'
  const transcript = formatTranscript(cues, format)
  const hasTranscript = cues.length > 0
  const copyTranscript = async () => {
    try {
      if (await copyToClipboard(transcript)) toast.success(`Copied all ${cues.length} cues`)
      else throw new Error('copy failed')
    } catch { toast.error('Clipboard access is unavailable. Try downloading the transcript instead.') }
  }
  const downloadTranscript = () => {
    downloadTextFile(`${safeFilename(video.title || 'transcript')}.${format}`, transcript, format === 'srt' ? 'application/x-subrip' : 'text/vtt')
  }
  return (
    <section className="browse-transcript" aria-label="Full transcript">
      <div className="transcript-header">
        <div><span>Full transcript · time-wise</span><Badge tone="muted">{loading ? <Loader2 className="spin" size={12} /> : `${cues.length} cues`}</Badge></div>
        <div className="transcript-actions">
          <Button variant="ghost" size="sm" disabled={!hasTranscript} onClick={() => void copyTranscript()}><Copy size={14} /> Copy</Button>
          <Button variant="ghost" size="sm" disabled={!hasTranscript} onClick={downloadTranscript}><Download size={14} /> Download .{format}</Button>
        </div>
      </div>
      {loading && <div className="transcript-loading"><Loader2 className="spin" size={16} /> Loading full transcript…</div>}
      {!loading && <div className="browse-cue-list">{cues.slice(0, renderLimit).map((cue) => <div className="cue-row" key={cue.id}><time>{formatMs(cue.start_ms)}</time><p>{cue.text}</p></div>)}{cues.length > renderLimit && <div className="transcript-more"><small>Showing {renderLimit} of {cues.length} cues</small><Button variant="secondary" size="sm" onClick={() => setRenderLimit((limit) => limit + RENDER_BATCH)}>Show more</Button></div>}</div>}
    </section>
  )
})
