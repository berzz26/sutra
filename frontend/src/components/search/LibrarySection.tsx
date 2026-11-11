import { useState } from 'react'
import { Copy, Download, FolderOpen, Loader2, Trash2, Video as VideoIcon } from 'lucide-react'
import { toast } from 'sonner'
import type { Cue, Video } from '../../api/endpoints'
import { Badge, Button, Skeleton } from '../ui'
import { copyToClipboard, downloadTextFile, formatMs, formatTranscript, safeFilename } from '../../lib/format'

interface LibrarySectionProps {
  videos: Video[]
  loading: boolean
  openId: string | null
  cues: Cue[]
  cuesLoading: boolean
  onToggle: (video: Video) => void
  onDelete: (video: Video) => void
}

export function LibrarySection({ videos, loading, openId, cues, cuesLoading, onToggle, onDelete }: LibrarySectionProps) {
  return (
    <section className="library-section" id="library" aria-labelledby="library-heading">
      <div className="section-heading">
        <h2 id="library-heading"><FolderOpen size={17} /> Video library</h2>
        <span className="spacer" />
        {videos.length > 0 && <Badge tone="muted">{videos.length} {videos.length === 1 ? 'source' : 'sources'}</Badge>}
      </div>
      {loading ? (
        <div className="library-list"><Skeleton className="library-skeleton" /><Skeleton className="library-skeleton" /></div>
      ) : videos.length === 0 ? (
        <div className="empty-library"><FolderOpen size={22} /><h3>Your library is quiet here</h3><p>Videos you add from this browser appear here. Add a YouTube video or transcript above to start searching.</p></div>
      ) : (
        <div className="library-list">
          {videos.map((video) => (
            <VideoCard
              key={video.id}
              video={video}
              open={openId === video.id}
              cues={openId === video.id ? cues : []}
              loading={cuesLoading}
              onToggle={() => onToggle(video)}
              onDelete={() => onDelete(video)}
            />
          ))}
        </div>
      )}
    </section>
  )
}

const RENDER_BATCH = 400

// Full transcripts can exceed 15k cues; render them progressively while
// copy/download always encode the complete set.
function VideoCard({ video, open, cues, loading, onToggle, onDelete }: { video: Video; open: boolean; cues: Cue[]; loading: boolean; onToggle: () => void; onDelete: () => void }) {
  const [renderLimit, setRenderLimit] = useState(RENDER_BATCH)
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
    <div className="video-card">
      <div className="video-card-main">
        {video.thumbnail_url
          ? <img className="video-thumb-img" src={video.thumbnail_url} alt="" loading="lazy" onError={(event) => { event.currentTarget.style.display = 'none' }} />
          : <div className="video-thumb"><VideoIcon size={18} /></div>}
        <div className="video-info">
          <div className="video-heading">
            <h3 title={video.title}>{video.title || 'Untitled video'}</h3>
            <Badge tone={video.status === 'ready' || video.status === 'succeeded' ? 'success' : 'muted'}>{video.status}</Badge>
          </div>
          <div className="video-meta">
            <span>{video.source_type === 'youtube' ? 'YouTube' : 'Uploaded transcript'}</span>
            {video.channel && <span>{video.channel}</span>}
            {video.duration_ms && <span>{formatMs(video.duration_ms)}</span>}
            <span>{video.transcript?.cue_count ?? 0} cues</span>
          </div>
        </div>
        <div className="video-actions">
          <Button variant="secondary" size="sm" onClick={onToggle}>{open ? 'Hide transcript' : 'View transcript'}</Button>
          <Button variant="ghost" size="icon" aria-label={`Remove ${video.title || 'video'} from this browser's library`} onClick={onDelete}><Trash2 size={16} /></Button>
        </div>
      </div>
      {open && (
        <div className="transcript-viewer">
          <div className="transcript-header">
            <div><span>Transcript cues</span><Badge tone="muted">{loading ? <Loader2 className="spin" size={12} /> : `${cues.length} cues`}</Badge></div>
            <div className="transcript-actions">
              <Button variant="ghost" size="sm" disabled={!hasTranscript} onClick={() => void copyTranscript()}><Copy size={14} /> Copy</Button>
              <Button variant="ghost" size="sm" disabled={!hasTranscript} onClick={downloadTranscript}><Download size={14} /> Download .{format}</Button>
            </div>
          </div>
          {loading && <div className="transcript-loading"><Loader2 className="spin" size={16} /> Loading full transcript…</div>}
          {!loading && (
            <>
              {cues.slice(0, renderLimit).map((cue) => <div className="cue-row" key={cue.id}><time>{formatMs(cue.start_ms)}</time><p>{cue.text}</p></div>)}
              {cues.length > renderLimit && (
                <div className="transcript-more">
                  <small>Showing {renderLimit} of {cues.length} cues</small>
                  <Button variant="secondary" size="sm" onClick={() => setRenderLimit((limit) => limit + RENDER_BATCH)}>Show more</Button>
                </div>
              )}
            </>
          )}
        </div>
      )}
    </div>
  )
}
