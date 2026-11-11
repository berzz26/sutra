import { Check, Library, Video as VideoIcon } from 'lucide-react'
import type { Video } from '../../api/endpoints'
import { formatMs } from '../../lib/format'

interface CurrentVideoContextProps {
  video: Video
  scope: 'video' | 'library'
  libraryCount: number
  onScopeChange: (scope: 'video' | 'library') => void
  onChangeVideo: () => void
}

export function CurrentVideoContext({ video, scope, libraryCount, onScopeChange, onChangeVideo }: CurrentVideoContextProps) {
  return (
    <div>
      <div className="context-header">
        {video.thumbnail_url
          ? <img src={video.thumbnail_url} alt="" />
          : <div className="context-thumb-fallback"><VideoIcon size={22} /></div>}
        <div className="context-copy">
          <span className="context-status"><Check size={13} /> Ready to search</span>
          <h3 title={video.title}>{video.title || 'Untitled video'}</h3>
          <div className="context-meta">
            {video.source_type === 'youtube' ? 'YouTube' : 'Transcript file'}
            {video.channel && ` · ${video.channel}`}
            {video.duration_ms && ` · ${formatMs(video.duration_ms)}`}
          </div>
        </div>
        <button type="button" className="context-change" onClick={onChangeVideo}>Change video</button>
      </div>
      <div className="scope-toggle" role="group" aria-label="Search scope">
        <button type="button" className={scope === 'video' ? 'active' : ''} aria-pressed={scope === 'video'} onClick={() => onScopeChange('video')}>
          <VideoIcon size={14} /> This Video
        </button>
        <button type="button" className={scope === 'library' ? 'active' : ''} aria-pressed={scope === 'library'} onClick={() => onScopeChange('library')}>
          <Library size={14} /> Entire Library · {libraryCount}
        </button>
      </div>
    </div>
  )
}
