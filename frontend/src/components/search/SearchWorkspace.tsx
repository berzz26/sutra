import { useRef, type FormEvent } from 'react'
import {
  Check, FileText, Folder, Link2, Search as SearchIcon, Sparkles,
  SlidersHorizontal, SquarePlay, ScanSearch, Upload, Video as VideoIcon,
} from 'lucide-react'
import type { Cue, SubtitleOption, Video, YouTubeMetadata } from '../../api/endpoints'
import { formatMs } from '../../lib/format'
import { CurrentVideoContext } from './CurrentVideoContext'
import { BrowseMetadata, BrowseTranscript } from './BrowseVideo'
import { ProgressiveFluxLoader, type ProgressiveFluxPhase } from '../ProgressiveFluxLoader'
import type { ProcessingState } from '../../api/endpoints'

export type WorkspaceTab = 'youtube' | 'library' | 'transcript' | 'find'

const TABS: Array<{ id: WorkspaceTab; label: string; icon: typeof Folder }> = [
  { id: 'youtube', label: 'YouTube', icon: SquarePlay },
  { id: 'library', label: 'Library', icon: Folder },
  { id: 'transcript', label: 'Transcript', icon: FileText },
  { id: 'find', label: 'Find Video', icon: ScanSearch },
]

const SUGGESTIONS = [
  'How does database replication work?',
  'What is the future of AI?',
  'Explain this concept simply',
]

const PIPELINE_PHASES: ProgressiveFluxPhase[] = [
  { at: 0, label: 'starting up' },
  { at: 15, label: 'fetching transcript' },
  { at: 45, label: 'queued for indexing' },
  { at: 60, label: 'building chunks' },
  { at: 70, label: 'generating embeddings' },
  { at: 95, label: 'saving index' },
  { at: 100, label: 'ready to search' },
]

interface SearchWorkspaceProps {
  tab: WorkspaceTab
  onTabChange: (tab: WorkspaceTab) => void

  currentVideo: Video | null
  onChangeVideo: () => void
  searchScope: 'video' | 'library'
  onScopeChange: (scope: 'video' | 'library') => void
  libraryCount: number
  searchQuery: string
  onSearchQueryChange: (value: string) => void
  searching: boolean
  onSearchSubmit: () => void
  onSuggestionPick: (query: string) => void

  url: string
  onUrlChange: (value: string) => void
  busy: boolean
  youtubePreview: YouTubeMetadata | null
  subtitleOptions: SubtitleOption[]
  translatedSubtitleOptions: SubtitleOption[]
  selectedSubtitle: string
  onSelectSubtitle: (value: string) => void
  onFetchMetadata: () => void
  onIndexVideo: () => void
  pipeline: { progress: number; stage: string; state: ProcessingState | null } | null

  file: File | null
  onFileChange: (file: File | null) => void
  uploadTitle: string
  onUploadTitleChange: (value: string) => void
  uploadURL: string
  onUploadURLChange: (value: string) => void
  onUpload: () => void

  videos: Video[]
  browseQuery: string
  onBrowseQueryChange: (value: string) => void
  browseResults: Video[]
  browseSelected: Video | null
  onSelectBrowse: (video: Video) => void
  browseCues: Cue[]
  browseLoading: boolean
}

export function SearchWorkspace(props: SearchWorkspaceProps) {
  const { tab, onTabChange } = props

  return (
    <section className="workspace" aria-label="Search workspace">
      <div className="workspace-tabs" role="tablist" aria-label="Search source">
        {TABS.map(({ id, label, icon: Icon }) => (
          <button
            key={id}
            type="button"
            role="tab"
            aria-selected={tab === id}
            className={`workspace-tab ${tab === id ? 'active' : ''}`}
            onClick={() => onTabChange(id)}
          >
            <Icon size={16} /> {label}
          </button>
        ))}
      </div>

      <div className="workspace-pane" key={tab}>
        {tab === 'youtube' && <YouTubePane {...props} searchable={Boolean(props.currentVideo)} />}
        {tab === 'library' && <LibraryPane {...props} />}
        {tab === 'transcript' && <TranscriptPane {...props} />}
        {tab === 'find' && <FindPane {...props} />}
      </div>

      {props.pipeline && (props.busy || props.pipeline.stage === 'failed') && (
        <PipelineProgress progress={props.pipeline.progress} stage={props.pipeline.stage} state={props.pipeline.state} />
      )}
    </section>
  )
}

/* ---------- YouTube: progressive ingestion, then in-video search ---------- */

function YouTubePane({ searchable, ...props }: SearchWorkspaceProps & { searchable: boolean }) {
  return (
    <div>
      {searchable && props.currentVideo && (
        <CurrentVideoContext
          video={props.currentVideo}
          scope={props.searchScope}
          libraryCount={props.libraryCount}
          onScopeChange={props.onScopeChange}
          onChangeVideo={props.onChangeVideo}
        />
      )}
      <form
        className="search-bar"
        onSubmit={(event: FormEvent) => { event.preventDefault(); searchable ? props.onSearchSubmit() : props.onFetchMetadata() }}
      >
        <span className="search-bar-icon">{searchable ? <SearchIcon size={19} /> : <Link2 size={19} />}</span>
        <input
          aria-label={searchable ? 'Search query' : 'YouTube URL'}
          value={searchable ? props.searchQuery : props.url}
          onChange={(event) => searchable ? props.onSearchQueryChange(event.target.value) : props.onUrlChange(event.target.value)}
          placeholder={searchable ? 'Ask anything about this video…' : 'Paste a YouTube URL or ask something about your videos...'}
          maxLength={2000}
          autoComplete="off"
        />
        <button type="submit" className="search-submit" disabled={props.searching}>
          {props.searching ? <LoaderIcon /> : <SearchIcon size={16} />}
          <span className="submit-label">Search</span>
        </button>
      </form>
      {searchable ? (
        <div className="suggest-row">
          <span className="sparkle"><Sparkles size={13} /></span>
          <span>Try searching</span>
          {SUGGESTIONS.map((suggestion) => (
            <button key={suggestion} type="button" className="suggest-chip" onClick={() => props.onSuggestionPick(suggestion)}>
              “{suggestion}”
            </button>
          ))}
        </div>
      ) : (
        <IngestFlow {...props} />
      )}
    </div>
  )
}

function IngestFlow({ ...props }: SearchWorkspaceProps) {
  const preview = props.youtubePreview
  return (
    <>
      {!preview && !props.busy && (
        <div className="suggest-row">
          <span className="sparkle"><Sparkles size={13} /></span>
          <span>Try searching</span>
          {SUGGESTIONS.map((suggestion) => (
            <button key={suggestion} type="button" className="suggest-chip" onClick={() => props.onSuggestionPick(suggestion)}>
              “{suggestion}”
            </button>
          ))}
        </div>
      )}
      {props.busy && !preview && <div className="suggest-row"><LoaderIcon /> <span>Fetching video details…</span></div>}
      {preview && (
        <>
          <div className="video-preview">
            <img src={preview.thumbnail_url} alt="" onError={(event) => { event.currentTarget.style.display = 'none' }} />
            <div className="video-preview-copy">
              <strong>{preview.title || 'Untitled video'}</strong>
              <span>{preview.channel || 'YouTube'}{preview.duration_ms ? ` · ${formatMs(preview.duration_ms)}` : ''}</span>
            </div>
            <span className="preview-status"><Check size={12} /> Subtitles found</span>
          </div>
          <div className="ingest-fields">
            <label className="field-label" htmlFor="subtitle-language">
              Subtitle language <small>English selected when available</small>
            </label>
            <select
              id="subtitle-language"
              className="subtitle-select"
              value={props.translatedSubtitleOptions.some((option) => `${option.source}:${option.language}` === props.selectedSubtitle) ? '' : props.selectedSubtitle}
              onChange={(event) => props.onSelectSubtitle(event.target.value)}
            >
              <option value="">English</option>
              {props.subtitleOptions.map((option) => (
                <option key={`${option.source}:${option.language}`} value={`${option.source}:${option.language}`}>
                  {option.language}{option.name ? ` — ${option.name}` : ''}
                </option>
              ))}
            </select>
            <button type="button" className="index-action" onClick={props.onIndexVideo} disabled={props.busy}>
              {props.busy ? <><LoaderIcon /> Indexing video</> : <>Upload &amp; index <ArrowUpRightIcon /></>}
            </button>
          </div>
        </>
      )}
    </>
  )
}

/* ---------- Library: natural-language search across everything ---------- */

function LibraryPane({ ...props }: SearchWorkspaceProps) {
  return (
    <div>
      <form className="search-bar" onSubmit={(event: FormEvent) => { event.preventDefault(); props.onSearchSubmit() }}>
        <span className="search-bar-icon"><SearchIcon size={19} /></span>
        <input
          aria-label="Search your entire library"
          value={props.searchQuery}
          onChange={(event) => props.onSearchQueryChange(event.target.value)}
          placeholder="Search across your entire video library..."
          maxLength={2000}
          autoComplete="off"
        />
        {/* <span className="search-bar-tools" aria-hidden="true"><SlidersHorizontal size={17} /></span> */}
        <button type="submit" className="search-submit" disabled={props.searching}>
          {props.searching ? <LoaderIcon /> : <SearchIcon size={16} />}
          <span className="submit-label">Search</span>
        </button>
      </form>
      <div className="suggest-row">
        <span className="sparkle"><Sparkles size={13} /></span>
        <span>Try searching</span>
        {SUGGESTIONS.map((suggestion) => (
          <button key={suggestion} type="button" className="suggest-chip" onClick={() => props.onSuggestionPick(suggestion)}>
            “{suggestion}”
          </button>
        ))}
      </div>
    </div>
  )
}

/* ---------- Transcript: bring your own .vtt / .srt ---------- */

function TranscriptPane({ ...props }: SearchWorkspaceProps) {
  const fileInput = useRef<HTMLInputElement>(null)
  const openPicker = () => fileInput.current?.click()
  return (
    <div className="upload-pane">
      <div
        className="search-bar"
        role="button"
        tabIndex={0}
        aria-label="Choose a transcript file"
        onClick={openPicker}
        onKeyDown={(event) => { if (event.key === 'Enter' || event.key === ' ') { event.preventDefault(); openPicker() } }}
        onDragOver={(event) => event.preventDefault()}
        onDrop={(event) => { event.preventDefault(); props.onFileChange(event.dataTransfer.files.item(0)) }}
        style={{ cursor: 'pointer' }}
      >
        <input ref={fileInput} type="file" accept=".vtt,.srt" hidden onChange={(event) => props.onFileChange(event.target.files?.[0] ?? null)} />
        <span className="search-bar-icon">{props.file ? <Check size={19} /> : <FileText size={19} />}</span>
        <input
          readOnly
          tabIndex={-1}
          value={props.file ? props.file.name : ''}
          placeholder="Drop a .vtt or .srt transcript here — or click to browse"
          aria-label="Selected transcript file"
          style={{ cursor: 'pointer' }}
        />
        <span className="search-bar-tools" aria-hidden="true"><Upload size={17} /></span>
      </div>
      {props.file ? (
        <>
          <div className="upload-fields">
            <input className="ui-input" value={props.uploadTitle} onChange={(event) => props.onUploadTitleChange(event.target.value)} placeholder="Title (optional)" />
            <input className="ui-input" value={props.uploadURL} onChange={(event) => props.onUploadURLChange(event.target.value)} placeholder="Original YouTube URL (optional)" />
          </div>
          <button type="button" className="index-action" onClick={props.onUpload} disabled={props.busy}>
            {props.busy ? <><LoaderIcon /> Indexing transcript</> : <>Upload &amp; index <ArrowUpRightIcon /></>}
          </button>
        </>
      ) : (
        <div className="suggest-row">
          <span className="sparkle"><Sparkles size={13} /></span>
          <span>Indexed locally · .VTT and .SRT supported</span>
        </div>
      )}
    </div>
  )
}

/* ---------- Find video: locate a video, inspect its transcript ---------- */

function FindPane({ ...props }: SearchWorkspaceProps) {
  return (
    <div className="find-pane">
      <form className="search-bar" onSubmit={(event: FormEvent) => event.preventDefault()}>
        <span className="search-bar-icon"><SearchIcon size={19} /></span>
        <input
          aria-label="Search library by title"
          value={props.browseQuery}
          onChange={(event) => props.onBrowseQueryChange(event.target.value)}
          placeholder="Find a video by title or channel…"
          autoComplete="off"
        />
        <button type="submit" className="search-submit" aria-label="Filter library">
          <SearchIcon size={16} />
          <span className="submit-label">Search</span>
        </button>
      </form>
      <div className="find-list" aria-label="Matching videos">
        {props.videos.length === 0 && <div className="find-empty">No videos in your library yet. Bring one in from the YouTube or Transcript tab.</div>}
        {props.videos.length > 0 && props.browseResults.length === 0 && <div className="find-empty">No videos match “{props.browseQuery.trim()}”.</div>}
        {props.browseResults.map((video) => (
          <button
            type="button"
            key={video.id}
            className={`find-item ${props.browseSelected?.id === video.id ? 'selected' : ''}`}
            onClick={() => props.onSelectBrowse(video)}
          >
            {video.thumbnail_url
              ? <img src={video.thumbnail_url} alt="" loading="lazy" onError={(event) => { event.currentTarget.style.display = 'none' }} />
              : <span className="find-item-fallback"><VideoIcon size={16} /></span>}
            <span className="find-item-copy">
              <strong title={video.title}>{video.title || 'Untitled video'}</strong>
              <small>
                {video.source_type === 'youtube' ? 'YouTube' : 'Transcript file'}
                {video.channel && ` · ${video.channel}`}
                {video.duration_ms && ` · ${formatMs(video.duration_ms)}`}
              </small>
            </span>
            {video.status !== 'ready' && <small className="find-status">{video.status}</small>}
          </button>
        ))}
      </div>
      {props.browseSelected && <BrowseMetadata video={props.browseSelected} />}
      {props.browseSelected && <BrowseTranscript video={props.browseSelected} cues={props.browseCues} loading={props.browseLoading} />}
    </div>
  )
}

/* ---------- Shared bits ---------- */

function PipelineProgress({ progress, stage, state }: { progress: number; stage: string; state: ProcessingState | null }) {
  const failed = stage === 'failed'
  return (
    <div className={`pipeline-progress ${failed ? 'failed' : ''}`} aria-live="polite">
      <ProgressiveFluxLoader value={progress} phases={failed ? [{ at: 0, label: 'indexing failed' }] : PIPELINE_PHASES} />
      <div className="pipeline-counts">
        {state && <small>{state.cue_count} cues · {state.embedding_count} embeddings generated</small>}
        <span>{progress}%</span>
      </div>
    </div>
  )
}

function LoaderIcon() {
  return (
    <svg className="spin" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.4" strokeLinecap="round" aria-hidden="true">
      <path d="M21 12a9 9 0 1 1-6.2-8.56" />
    </svg>
  )
}

function ArrowUpRightIcon() {
  return (
    <svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
      <path d="M7 17 17 7" /><path d="M8 7h9v9" />
    </svg>
  )
}
