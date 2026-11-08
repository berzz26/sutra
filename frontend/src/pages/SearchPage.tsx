import { FormEvent, type CSSProperties, useEffect, useRef, useState } from 'react'
import { ArrowUpRight, Check, Clock3, Copy, Download, FileText, FolderOpen, Library, Loader2, Search as SearchIcon, Trash2, Upload, Video, X } from 'lucide-react'
import { toast } from 'sonner'
import { ApiError } from '../api/client'
import { Cue, deleteVideo, fetchAllVideos, getProcessing, getTranscript, ingestYouTube, listSubtitles, ProcessingState, SearchResult, search, SubtitleOption, uploadTranscript, Video as VideoRecord, YouTubeMetadata } from '../api/endpoints'
import { AlertDialog, Badge, Button, Card, Input, Skeleton } from '../components/ui'
import { ProgressiveFluxLoader } from '../components/ProgressiveFluxLoader'

export function SearchPage() {
  const [videos, setVideos] = useState<VideoRecord[]>([])
  const [currentVideo, setCurrentVideo] = useState<VideoRecord | null>(null)
  const [searchScope, setSearchScope] = useState<'video' | 'library'>('video')
  const [loadingVideos, setLoadingVideos] = useState(true)
  const [ingestTab, setIngestTab] = useState<'youtube' | 'library' | 'file' | 'browse'>('youtube')
  const [url, setUrl] = useState('')
  const [language, setLanguage] = useState('en')
  const [title, setTitle] = useState('')
  const [originalURL, setOriginalURL] = useState('')
  const [subtitleOptions, setSubtitleOptions] = useState<SubtitleOption[]>([])
  const [translatedSubtitleOptions, setTranslatedSubtitleOptions] = useState<SubtitleOption[]>([])
  const [selectedSubtitle, setSelectedSubtitle] = useState('')
  const [youtubePreview, setYoutubePreview] = useState<YouTubeMetadata | null>(null)
  const [file, setFile] = useState<File | null>(null)
  const [busy, setBusy] = useState(false)
  const [pipeline, setPipeline] = useState<{ progress: number; stage: string; state: ProcessingState | null } | null>(null)
  const [searchQuery, setSearchQuery] = useState('')
  const [searchResults, setSearchResults] = useState<SearchResult[]>([])
  const [searching, setSearching] = useState(false)
  const [searchDuration, setSearchDuration] = useState<{ client: number; server: number } | null>(null)
  const [searchMessage, setSearchMessage] = useState<string | null>(null)
  const [open, setOpen] = useState<string | null>(null)
  const [cues, setCues] = useState<Cue[]>([])
  const [transcriptLoading, setTranscriptLoading] = useState(false)
  const [browseQuery, setBrowseQuery] = useState('')
  const [browseSelected, setBrowseSelected] = useState<VideoRecord | null>(null)
  const [browseCues, setBrowseCues] = useState<Cue[]>([])
  const [browseLoading, setBrowseLoading] = useState(false)
  const [deleteTarget, setDeleteTarget] = useState<VideoRecord | null>(null)
  const fileInput = useRef<HTMLInputElement>(null)

  const refresh = async () => {
    setLoadingVideos(true)
    try { setVideos(await fetchAllVideos()) } catch (cause) { toast.error(errorText(cause)) } finally { setLoadingVideos(false) }
  }

  useEffect(() => { void refresh(); if (window.location.hash === '#library') setTimeout(() => document.getElementById('library')?.scrollIntoView(), 0) }, [])
  const discoverSubtitles = async () => {
    if (!url.trim()) { toast.error('Enter a YouTube URL first.'); return }
    setBusy(true)
    try {
      const result = await listSubtitles(url)
      setYoutubePreview(result.metadata)
      setSubtitleOptions(result.subtitles)
      setTranslatedSubtitleOptions(result.translated_subtitles)
      const english = result.subtitles.find((option) => /^en(?:[-_]|$)/i.test(option.language))
      setSelectedSubtitle(english ? `${english.source}:${english.language}` : '')
      toast.success(`Found ${result.subtitles.length} subtitle tracks`)
    } catch (cause) { toast.error(errorText(cause)) } finally { setBusy(false) }
  }

  const waitForProcessing = async (videoID: string) => {
    for (let attempt = 0; attempt < 240; attempt += 1) {
      const state = await getProcessing(videoID)
      setPipeline({ progress: state.progress_percent, stage: state.stage, state })
      if (state.status === 'completed') return
      if (state.status === 'failed') throw new Error(state.last_error || 'Transcript indexing failed.')
      await new Promise((resolve) => window.setTimeout(resolve, 750))
    }
    throw new Error('Indexing is taking longer than expected. Check the library for status.')
  }

  const submitYouTube = async (event: FormEvent) => {
    event.preventDefault(); setBusy(true); setPipeline({ progress: 15, stage: 'fetching subtitles', state: null })
    try {
      const subtitle = [...subtitleOptions, ...translatedSubtitleOptions].find((option) => `${option.source}:${option.language}` === selectedSubtitle)
      const result = await ingestYouTube(url, language, subtitle)
      setPipeline({ progress: 45, stage: 'queued for indexing', state: null })
      await waitForProcessing(result.id)
      setCurrentVideo(result); setSearchScope('video'); setSearchResults([]); setSearchDuration(null); setSearchMessage(null); setUrl(''); setLanguage('en'); setYoutubePreview(null); setSubtitleOptions([]); setTranslatedSubtitleOptions([]); setSelectedSubtitle(''); toast.success('Video indexed and ready to search'); await refresh()
    } catch (cause) { toast.error(errorText(cause)); setPipeline((current) => current ? { ...current, stage: 'failed' } : null) } finally { setBusy(false) }
  }

  const submitUpload = async (event: FormEvent) => {
    event.preventDefault()
    if (!file) { toast.error('Choose a .vtt or .srt file first.'); return }
    setBusy(true); setPipeline({ progress: 15, stage: 'uploading transcript', state: null })
    try { const data = new FormData(); data.append('file', file); if (title) data.append('title', title); if (originalURL) data.append('original_url', originalURL); if (language) data.append('language', language); const result = await uploadTranscript(data); setPipeline({ progress: 45, stage: 'queued for indexing', state: null }); await waitForProcessing(result.id); setCurrentVideo(result); setSearchScope('video'); setSearchResults([]); setSearchDuration(null); setSearchMessage(null); setFile(null); setTitle(''); setOriginalURL(''); setLanguage('en'); if (fileInput.current) fileInput.current.value = ''; toast.success('Transcript indexed and ready to search'); await refresh() } catch (cause) { toast.error(errorText(cause)); setPipeline((current) => current ? { ...current, stage: 'failed' } : null) } finally { setBusy(false) }
  }

  // The transcript endpoint is paginated (default limit 50); copy/download
  // need the whole conversation, so fetch every page before showing it.
  const TRANSCRIPT_PAGE_SIZE = 200

  const fetchAllCues = async (video: VideoRecord): Promise<Cue[]> => {
    const expected = video.transcript?.cue_count
    const all: Cue[] = []
    let offset = 0
    for (;;) {
      const page = await getTranscript(video.id, TRANSCRIPT_PAGE_SIZE, offset)
      all.push(...page.cues)
      if (page.cues.length < TRANSCRIPT_PAGE_SIZE) break
      offset += page.cues.length
      if (expected !== undefined && offset >= expected) break
    }
    return all
  }

  const showTranscript = async (video: VideoRecord) => {
    if (open === video.id) { setOpen(null); return }
    setTranscriptLoading(true)
    setOpen(video.id)
    try {
      setCues(await fetchAllCues(video))
    } catch (cause) {
      setOpen(null)
      toast.error(errorText(cause))
    } finally { setTranscriptLoading(false) }
  }

  // Library browser tab: pick a video by title, then inspect its full
  // transcript with its metadata (rendered below the library section).
  const openBrowseVideo = async (video: VideoRecord) => {
    const alreadyLoaded = browseSelected?.id === video.id && browseCues.length >= (video.transcript?.cue_count ?? 0)
    if (alreadyLoaded) { document.getElementById('browse-detail')?.scrollIntoView({ behavior: 'smooth' }); return }
    setBrowseSelected(video)
    setBrowseLoading(true)
    setBrowseCues([])
    try {
      setBrowseCues(await fetchAllCues(video))
      setTimeout(() => document.getElementById('browse-detail')?.scrollIntoView({ behavior: 'smooth' }), 0)
    } catch (cause) { toast.error(errorText(cause)) } finally { setBrowseLoading(false) }
  }

  const confirmDelete = async () => {
    if (!deleteTarget) return
    try { await deleteVideo(deleteTarget.id); setDeleteTarget(null); if (open === deleteTarget.id) { setOpen(null); setCues([]) } if (browseSelected?.id === deleteTarget.id) { setBrowseSelected(null); setBrowseCues([]) } if (currentVideo?.id === deleteTarget.id) { setCurrentVideo(null); setSearchResults([]); setSearchDuration(null); setSearchMessage(null) } toast.success('Video removed from your library'); await refresh() } catch (cause) { toast.error(errorText(cause)) }
  }

  const changeSearchScope = (scope: 'video' | 'library') => {
    setSearchScope(scope)
    setSearchResults([])
    setSearchDuration(null)
    setSearchMessage(null)
  }

  const changeIngestTab = (tab: 'youtube' | 'library' | 'file' | 'browse') => {
    setIngestTab(tab)
    if (tab !== 'browse') changeSearchScope(tab === 'library' ? 'library' : 'video')
  }

  const submitSearch = async (event: FormEvent) => {
    event.preventDefault()
    if (!searchQuery.trim()) { setSearchMessage('Enter a question or topic to search.'); return }
    if (searchScope === 'video' && !currentVideo?.id) { setSearchMessage('Fetch and index a video before searching within it.'); return }
    const started = performance.now(); setSearching(true); setSearchMessage(null); setSearchDuration(null)
    try { const result = await search(searchQuery, 10, searchScope === 'video' ? currentVideo?.id : undefined); setSearchResults(result.results); setSearchDuration({ client: Math.round(performance.now() - started), server: result.duration_ms }); if (result.results.length === 0) setSearchMessage('No indexed transcript matches found. Try a broader phrase.') } catch (cause) { setSearchMessage(errorText(cause)); setSearchResults([]); toast.error(errorText(cause)) } finally { setSearching(false) }
  }


  const browseFilter = browseQuery.trim().toLowerCase()
  const browseResults = browseFilter
    ? videos.filter((video) => (video.title || '').toLowerCase().includes(browseFilter) || (video.channel || '').toLowerCase().includes(browseFilter))
    : videos

  return <>
    <section className="hero-section">
      <div className="hero-copy"><h1>Find the moment<br /><em>that matters.</em></h1><p className="hero-description">Search every indexed conversation, lecture, and idea in your video library using natural language.</p></div>
      {/* <div className="hero-note"><span className="note-line" /><span>Local inference<br />Private by design</span></div> */}
    </section>

    <section className="ingest-section" aria-labelledby="ingest-heading"><div className="section-heading"><div><p className="eyebrow">Build your library</p><h2 id="ingest-heading">Bring in a new source</h2></div><p className="section-description">Sutra turns captions into timestamped, searchable moments.</p></div>
      <Card className={`ingest-card unified-ingest-card ${ingestTab === 'library' ? 'library-active' : ''}`}>
        <div className="ingest-tabs" role="tablist" aria-label="Transcript source">
          <button type="button" role="tab" aria-selected={ingestTab === 'youtube'} className={ingestTab === 'youtube' ? 'active' : ''} onClick={() => changeIngestTab('youtube')}><Video size={16} /> YouTube video</button>
          <button type="button" role="tab" aria-selected={ingestTab === 'library'} className={ingestTab === 'library' ? 'active' : ''} onClick={() => changeIngestTab('library')}><Library size={16} /> Search Library</button>
          <button type="button" role="tab" aria-selected={ingestTab === 'file'} className={ingestTab === 'file' ? 'active' : ''} onClick={() => changeIngestTab('file')}><Upload size={16} /> Transcript file</button>
          <button type="button" role="tab" aria-selected={ingestTab === 'browse'} className={ingestTab === 'browse' ? 'active' : ''} onClick={() => changeIngestTab('browse')}><FileText size={16} /> Find video</button>
        </div>
        {ingestTab === 'library' && <div className="library-tab-pane"><Library size={22} /><div><h3>Search your library</h3><p>Search across every indexed video without fetching a new source.</p></div><Badge tone="muted">{videos.length} {videos.length === 1 ? 'video' : 'videos'} indexed</Badge></div>}
        {ingestTab === 'youtube' ? <div className="ingest-pane"><div className="card-title-row"><div><h3>Fetch a video</h3><p>Paste a YouTube URL to load its title, thumbnail, duration, channel, and all compatible subtitles.</p></div><Badge tone="accent">Local extraction</Badge></div><form onSubmit={submitYouTube} className="stack-form"><div className="url-action-row"><Input required value={url} onChange={(event) => { setUrl(event.target.value); setYoutubePreview(null); setSubtitleOptions([]); setTranslatedSubtitleOptions([]); setSelectedSubtitle('') }} placeholder="youtube.com/watch?v=…" /><Button type="button" variant="secondary" onClick={() => void discoverSubtitles()} disabled={busy}>{busy ? <Loader2 className="spin" size={16} /> : <SearchIcon size={16} />} Fetch video</Button></div>{youtubePreview && <div className="youtube-preview"><img src={youtubePreview.thumbnail_url} alt="" /><div><strong>{youtubePreview.title || 'Untitled video'}</strong><span>{youtubePreview.channel || 'YouTube'}{youtubePreview.duration_ms ? ` · ${formatMs(youtubePreview.duration_ms)}` : ''}</span></div></div>}{youtubePreview && <><label className="field-label" htmlFor="subtitle-language">Subtitle language <span>English selected when available</span></label><select id="subtitle-language" className="ui-input ui-select" value={translatedSubtitleOptions.some((option) => `${option.source}:${option.language}` === selectedSubtitle) ? '' : selectedSubtitle} onChange={(event) => setSelectedSubtitle(event.target.value)}><option value="">English</option>{subtitleOptions.map((option) => <option key={`${option.source}:${option.language}`} value={`${option.source}:${option.language}`}>{option.language}{option.name ? ` — ${option.name}` : ''}</option>)}</select><Button disabled={busy}>{busy ? <><Loader2 className="spin" size={16} /> Indexing video</> : <>Upload & index <ArrowUpRight size={16} /></>}</Button></>}</form></div> : ingestTab === 'file' ? <div className="ingest-pane"><div className="card-title-row"><div><h3>Upload a transcript</h3><p>Bring your own VTT or SRT captions into the same indexing pipeline.</p></div><Badge tone="muted">.VTT · .SRT</Badge></div><form onSubmit={submitUpload} className="stack-form"><button type="button" className={`drop-zone ${file ? 'has-file' : ''}`} onClick={() => fileInput.current?.click()} onDragOver={(event) => event.preventDefault()} onDrop={(event) => { event.preventDefault(); setFile(event.dataTransfer.files.item(0)) }}><input ref={fileInput} type="file" accept=".vtt,.srt" hidden onChange={(event) => setFile(event.target.files?.[0] ?? null)} />{file ? <><Check size={20} /><strong>{file.name}</strong><span>Ready to upload</span></> : <><FileText size={22} /><strong>Drop a transcript here</strong><span>or browse from your computer</span></>}</button><Input value={title} onChange={(event) => setTitle(event.target.value)} placeholder="Title (optional)" /><Input value={originalURL} onChange={(event) => setOriginalURL(event.target.value)} placeholder="Original YouTube URL (optional)" /><Button disabled={busy || !file}>{busy ? <><Loader2 className="spin" size={16} /> Indexing transcript</> : <>Upload & index <ArrowUpRight size={16} /></>}</Button></form></div> : null}
        {ingestTab === 'browse' && <div className="browse-pane">
          <div className="browse-search">
            <SearchIcon size={16} />
            <Input value={browseQuery} onChange={(event) => setBrowseQuery(event.target.value)} placeholder="Search your library by title…" aria-label="Search library by title" />
            <Badge tone="muted">{browseResults.length} {browseResults.length === 1 ? 'match' : 'matches'}</Badge>
          </div>
          <div className="browse-results" aria-label="Matching videos">
            {videos.length === 0 && <div className="browse-empty"><FolderOpen size={20} /><p>No videos in your library yet. Bring one in from the tabs above.</p></div>}
            {videos.length > 0 && browseResults.length === 0 && <div className="browse-empty"><p>No videos match “{browseQuery.trim()}”.</p></div>}
            {browseResults.map((video) => <button type="button" key={video.id} className={`browse-result ${browseSelected?.id === video.id ? 'selected' : ''}`} onClick={() => void openBrowseVideo(video)}><span className="browse-result-copy"><strong title={video.title}>{video.title || 'Untitled video'}</strong><small>{video.source_type === 'youtube' ? 'YouTube' : 'Transcript file'}{video.channel && ` · ${video.channel}`}{video.duration_ms && ` · ${formatMs(video.duration_ms)}`}</small></span><Badge tone={video.status === 'ready' ? 'success' : 'muted'}>{video.status}</Badge></button>)}
          </div>
        </div>}
        {pipeline && (busy || pipeline.stage === 'failed') && <PipelineProgress pipeline={pipeline} />}
      </Card>
    </section>

    {(ingestTab === 'library' || (currentVideo && !busy)) && <div className="search-reveal">
      {ingestTab === 'library' ? <div className="library-search-heading"><div><p className="eyebrow">Search Library</p><h2>Find a moment across your videos.</h2></div><span>{videos.length} indexed {videos.length === 1 ? 'video' : 'videos'}</span></div> : <div className="video-ready-card"><div className="video-ready-copy"><div className="video-ready-status"><Check size={14} /> Ready to search</div><h2 title={currentVideo?.title}>{currentVideo?.title || 'Untitled video'}</h2><div className="video-ready-meta"><span>{currentVideo?.source_type === 'youtube' ? 'YouTube' : 'Transcript file'}</span>{currentVideo?.channel && <span>{currentVideo.channel}</span>}{currentVideo?.duration_ms && <span>{formatMs(currentVideo.duration_ms)}</span>}{currentVideo?.language && <span>{currentVideo.language}</span>}</div></div>{currentVideo?.thumbnail_url ? <img src={currentVideo.thumbnail_url} alt="" /> : <div className="video-ready-fallback"><Video size={22} /></div>}</div>}
      <section className="search-panel" aria-labelledby="search-heading">
        <div className="search-context">{searchScope === 'video' && currentVideo ? <><Video size={16} /><span>Current video</span><strong title={currentVideo.title}>{currentVideo.title || 'Untitled video'}</strong></> : <><Library size={16} /><span>Entire library</span><strong>{videos.length} indexed {videos.length === 1 ? 'video' : 'videos'}</strong></>}</div>
        <div className="search-scope-row">
          <span className="search-scope-label">Search scope</span>
          <div className="search-scope-toggle" role="group" aria-label="Search scope">
            <button type="button" className={searchScope === 'video' ? 'active' : ''} aria-pressed={searchScope === 'video'} disabled={!currentVideo} onClick={() => changeSearchScope('video')}><Video size={14} /> This Video</button>
            <button type="button" className={searchScope === 'library' ? 'active' : ''} aria-pressed={searchScope === 'library'} onClick={() => changeSearchScope('library')}><Library size={14} /> Entire Library</button>
          </div>
        </div>
        <form onSubmit={submitSearch}>
          <label id="search-heading" className="sr-only">Search your transcript library</label>
          <SearchIcon className="search-leading-icon" size={20} />
          <Input aria-label="Search query" value={searchQuery} onChange={(event) => { setSearchQuery(event.target.value); setSearchMessage(null) }} placeholder="Ask anything about your videos…" maxLength={2000} />
          {searchQuery && <Button type="button" variant="ghost" size="icon" className="clear-search" aria-label="Clear search" onClick={() => setSearchQuery('')}>< X size={17} /></Button>}
          <Button type="submit" disabled={searching} size="lg">{searching ? <><Loader2 className="spin" size={17} /> Searching</> : <>Search <ArrowUpRight size={17} /></>}</Button>
        </form>
        {!searchResults.length && !searchDuration && !searchMessage && <div className="search-hint"><span>Try a question</span><button type="button" onClick={() => setSearchQuery('How does database replication work?')}>“How does database replication work?”</button><span className="hint-separator">or</span><button type="button" onClick={() => setSearchQuery('Find the part where they discuss the future of AI')}>“Find the part where they discuss the future of AI”</button></div>}
      </section>

      {(searching || searchDuration || searchMessage || searchResults.length > 0) && <section className="results-section" aria-live="polite">
        <div className="section-heading"><div><p className="eyebrow">Semantic retrieval</p><h2>Relevant moments</h2></div>{searchDuration && <div className="search-metrics"><strong>{searchResults.length}</strong> matches <span className="metric-divider" /> resolved in <strong>{searchDuration.client} ms</strong></div>}</div>
        {searching && <div className="result-loading-list"><Skeleton className="result-loading-card" /><Skeleton className="result-loading-card" /></div>}
        {!searching && searchMessage && <div className="empty-state"><SearchIcon size={20} /><p>{searchMessage}</p></div>}
        {!searching && searchResults.length > 0 && <div className="results-list">{searchResults.map((result, index) => <ResultCard key={result.chunk_id} result={result} video={videos.find((video) => video.id === result.video_id)} query={searchQuery} index={index} />)}</div>}
      </section>}
    </div>}

    <section className="library-section" id="library" aria-labelledby="library-heading"><div className="section-heading"><div><p className="eyebrow">Your collection</p><h2 id="library-heading">Video library</h2></div>{videos.length > 0 && <Badge tone="muted">{videos.length} {videos.length === 1 ? 'source' : 'sources'}</Badge>}</div>{loadingVideos ? <div className="library-list"><Skeleton className="library-skeleton" /><Skeleton className="library-skeleton" /></div> : videos.length === 0 ? <div className="empty-library"><FolderOpen size={22} /><h3>Your library is quiet</h3><p>Add a YouTube video or transcript above to start searching.</p></div> : <div className="library-list">{videos.map((video) => <VideoCard key={video.id} video={video} open={open === video.id} cues={cues} loading={transcriptLoading} onToggle={() => void showTranscript(video)} onDelete={() => setDeleteTarget(video)} />)}</div>}</section>
    {ingestTab === 'browse' && browseSelected && <section className="browse-detail" id="browse-detail" aria-label="Selected video detail"><BrowseMetadata video={browseSelected} /><BrowseTranscript video={browseSelected} cues={browseCues} loading={browseLoading} /></section>}
    <AlertDialog open={deleteTarget !== null} title="Remove this video?" description="This deletes the video, transcript cues, and indexed moments. This cannot be undone." confirmLabel="Delete video" onCancel={() => setDeleteTarget(null)} onConfirm={() => void confirmDelete()} />
  </>
}

function ResultCard({ result, video, query, index }: { result: SearchResult; video?: VideoRecord; query: string; index: number }) {
  const link = result.timestamp_url || result.original_url || video?.original_url || '#'
  const excerpt = buildTranscriptExcerpt(result.text, query)
  return <article className="result-card" style={{ '--delay': `${index * 55}ms` } as CSSProperties}><div className="result-media"><Thumbnail src={video?.thumbnail_url} /><Badge tone="accent"><Clock3 size={12} /> {formatMs(result.start_ms)} – {formatMs(result.end_ms)}</Badge></div><div className="result-content"><div className="result-meta"><span>{video?.source_type === 'youtube' ? 'YouTube' : 'Transcript file'}</span><span>{video?.channel || 'Video source'}</span>{video?.language && <span>{video.language}</span>}</div><h3>{result.video_title || video?.title || 'Untitled video'}</h3><p className="result-excerpt"><Highlight text={excerpt} query={query} /></p><div className="result-footer"><span><Clock3 size={13} /> Relevant moment</span><a href={link} target="_blank" rel="noreferrer">Open moment <ArrowUpRight size={14} /></a></div></div></article>
}

function Thumbnail({ src }: { src?: string }) {
  const [failed, setFailed] = useState(!src)
  if (failed || !src) return <div className="thumbnail-fallback"><Video size={25} /><span>Video moment</span></div>
  return <img src={src} alt="" loading="lazy" onError={() => setFailed(true)} />
}

function VideoCard({ video, open, cues, loading, onToggle, onDelete }: { video: VideoRecord; open: boolean; cues: Cue[]; loading: boolean; onToggle: () => void; onDelete: () => void }) {
  // Full transcripts can exceed 15k cues; render them progressively while
  // copy/download always encode the complete set.
  const RENDER_BATCH = 400
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
  return <Card className="video-card"><div className="video-card-main"><div className="video-thumb"><Video size={20} /><span>{video.source_type === 'youtube' ? 'YT' : 'FILE'}</span></div><div className="video-info"><div className="video-heading"><h3 title={video.title}>{video.title || 'Untitled video'}</h3><Badge tone={video.status === 'ready' || video.status === 'succeeded' ? 'success' : 'muted'}>{video.status}</Badge></div><div className="video-meta"><span>{video.source_type === 'youtube' ? 'YouTube' : 'Uploaded transcript'}</span><span>{video.language || 'Language unknown'}</span><span>{video.transcript?.cue_count ?? 0} cues</span></div></div><div className="video-actions"><Button variant="secondary" size="sm" onClick={onToggle}>{open ? 'Hide transcript' : 'View transcript'}</Button><Button variant="ghost" size="icon" aria-label={`Delete ${video.title || 'video'}`} onClick={onDelete}><Trash2 size={16} /></Button></div></div>{open && <div className="transcript-viewer"><div className="transcript-header"><div><span>Transcript cues</span><Badge tone="muted">{loading ? <Loader2 className="spin" size={12} /> : `${cues.length} cues`}</Badge></div><div className="transcript-actions"><Button variant="ghost" size="sm" disabled={!hasTranscript} onClick={() => void copyTranscript()}><Copy size={14} /> Copy</Button><Button variant="ghost" size="sm" disabled={!hasTranscript} onClick={downloadTranscript}><Download size={14} /> Download .{format}</Button></div></div>{loading && <div className="transcript-loading"><Loader2 className="spin" size={16} /> Loading full transcript…</div>}{!loading && <>{cues.slice(0, renderLimit).map((cue) => <div className="cue-row" key={cue.id}><time>{formatMs(cue.start_ms)}</time><p>{cue.text}</p></div>)}{cues.length > renderLimit && <div className="transcript-more"><small>Showing {renderLimit} of {cues.length} cues</small><Button variant="secondary" size="sm" onClick={() => setRenderLimit((limit) => limit + RENDER_BATCH)}>Show more</Button></div>}</>}</div>}</Card>
}

function BrowseMetadata({ video }: { video: VideoRecord }) {
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
  return <section className="browse-metadata" aria-label="Video metadata">
    <div className="browse-metadata-body">
      <div className="browse-metadata-title"><h3 title={video.title}>{video.title || 'Untitled video'}</h3><Badge tone={video.status === 'ready' || video.status === 'succeeded' ? 'success' : 'muted'}>{video.status}</Badge></div>
      <dl className="browse-meta-grid">{items.map(([label, value]) => <div key={label}><dt>{label}</dt><dd title={value}>{value}</dd></div>)}</dl>
      {link && <a className="browse-original-link" href={link} target="_blank" rel="noreferrer">Open original <ArrowUpRight size={13} /></a>}
    </div>
  </section>
}

function BrowseTranscript({ video, cues, loading }: { video: VideoRecord; cues: Cue[]; loading: boolean }) {
  const RENDER_BATCH = 400
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
  return <section className="browse-transcript" aria-label="Full transcript">
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
}

function PipelineProgress({ pipeline }: { pipeline: { progress: number; stage: string; state: ProcessingState | null } }) {
  const failed = pipeline.stage === 'failed'
  const phases = failed ? [{ at: 0, label: 'indexing failed' }] : [{ at: 0, label: 'starting up' }, { at: 15, label: 'fetching transcript' }, { at: 45, label: 'queued for indexing' }, { at: 60, label: 'building chunks' }, { at: 70, label: 'generating embeddings' }, { at: 95, label: 'saving index' }, { at: 100, label: 'ready to search' }]
  return <div className={`pipeline-progress ${failed ? 'failed' : ''}`} aria-live="polite"><ProgressiveFluxLoader value={pipeline.progress} phases={phases} /><div className="pipeline-counts">{pipeline.state && <small>{pipeline.state.cue_count} cues · {pipeline.state.embedding_count} embeddings generated</small>}<span>{pipeline.progress}%</span></div></div>
}

const QUERY_STOPWORDS = new Set(['about', 'after', 'again', 'also', 'been', 'being', 'could', 'does', 'from', 'have', 'into', 'more', 'most', 'only', 'other', 'should', 'some', 'than', 'that', 'their', 'there', 'these', 'they', 'this', 'those', 'through', 'what', 'when', 'where', 'which', 'while', 'with', 'would', 'your'])

function normalizeTranscriptText(text: string) {
  const withoutMarkers = text.replace(/(?:^|\s)(?:>>\s*)+/g, ' ')
  const normalized = withoutMarkers.replace(/\s+/g, ' ').replace(/\s+([,.;!?])/g, '$1').trim()
  return collapseRepeatedPhrases(normalized)
}

function collapseRepeatedPhrases(text: string) {
  const words = text.split(' ')
  const output: string[] = []
  let index = 0
  while (index < words.length) {
    let collapsed = false
    const maximumPhraseLength = Math.min(12, Math.floor((words.length - index) / 2))
    for (let phraseLength = maximumPhraseLength; phraseLength >= 3; phraseLength -= 1) {
      const first = words.slice(index, index + phraseLength).map(normalizeComparisonWord)
      const second = words.slice(index + phraseLength, index + phraseLength * 2).map(normalizeComparisonWord)
      if (first.every(Boolean) && first.join(' ') === second.join(' ')) {
        output.push(...words.slice(index, index + phraseLength))
        index += phraseLength * 2
        collapsed = true
        break
      }
    }
    if (!collapsed) {
      output.push(words[index])
      index += 1
    }
  }
  return output.join(' ')
}

function normalizeComparisonWord(word: string) {
  return word.toLocaleLowerCase().replace(/[^\p{L}\p{N}']+/gu, '')
}

function meaningfulQueryTerms(query: string) {
  return query.split(/\s+/).map(normalizeComparisonWord).filter((term, index, terms) => term.length >= 2 && !QUERY_STOPWORDS.has(term) && terms.indexOf(term) === index).slice(0, 8)
}

function buildTranscriptExcerpt(text: string, query: string) {
  const normalized = normalizeTranscriptText(text)
  const maximumCharacters = 320
  if (normalized.length <= maximumCharacters) return normalized

  const terms = meaningfulQueryTerms(query)
  const lowered = normalized.toLocaleLowerCase()
  const matchIndex = terms.map((term) => lowered.indexOf(term)).filter((index) => index >= 0).sort((a, b) => a - b)[0] ?? 0
  let start = sentenceStart(normalized, matchIndex)
  let end = sentenceEnd(normalized, matchIndex)

  while (start > 0) {
    const previousStart = sentenceStart(normalized, start - 1)
    if (end - previousStart > maximumCharacters) break
    start = previousStart
  }
  while (end < normalized.length) {
    const nextEnd = sentenceEnd(normalized, end)
    if (nextEnd - start > maximumCharacters) break
    end = nextEnd
  }

  if (end - start > maximumCharacters) {
    start = Math.max(0, matchIndex - Math.floor(maximumCharacters * 0.35))
    end = Math.min(normalized.length, start + maximumCharacters)
    while (start > 0 && normalized[start] !== ' ') start += 1
    while (end < normalized.length && normalized[end] !== ' ') end -= 1
  }

  const excerpt = normalized.slice(start, end).trim()
  return `${start > 0 ? '… ' : ''}${excerpt}${end < normalized.length ? ' …' : ''}`
}

function sentenceStart(text: string, index: number) {
  const boundary = Math.max(text.lastIndexOf('.', index - 1), text.lastIndexOf('!', index - 1), text.lastIndexOf('?', index - 1))
  return boundary < 0 ? 0 : boundary + 1
}

function sentenceEnd(text: string, index: number) {
  const boundaries = [text.indexOf('.', index), text.indexOf('!', index), text.indexOf('?', index)].filter((boundary) => boundary >= 0)
  return boundaries.length ? Math.min(...boundaries) + 1 : text.length
}

function Highlight({ text, query }: { text: string; query: string }) {
  const terms = meaningfulQueryTerms(query)
  if (!terms.length) return <>{text}</>
  const pattern = new RegExp(`\\b(${terms.map(escapeRegExp).join('|')})\\b`, 'gi')
  return <>{text.split(pattern).map((part, index) => terms.some((term) => part.toLocaleLowerCase() === term) ? <mark key={index}>{part}</mark> : part)}</>
}

function escapeRegExp(value: string) { return value.replace(/[.*+?^${}()|[\]\\]/g, '\\$&') }
function formatMs(ms: number) { return `${Math.floor(ms / 60000)}:${String(Math.floor(ms / 1000) % 60).padStart(2, '0')}` }
function formatTranscript(cues: Cue[], format: 'srt' | 'vtt') {
  const body = cues.map((cue, index) => {
    const start = formatSubtitleTimestamp(cue.start_ms, format)
    const end = formatSubtitleTimestamp(cue.end_ms, format)
    return format === 'srt' ? `${index + 1}\n${start} --> ${end}\n${cue.text}` : `${start} --> ${end}\n${cue.text}`
  }).join('\n\n')
  return format === 'vtt' ? `WEBVTT\n\n${body}\n` : `${body}\n`
}
function formatSubtitleTimestamp(ms: number, format: 'srt' | 'vtt') {
  const safeMs = Math.max(0, Math.round(ms))
  const hours = Math.floor(safeMs / 3_600_000)
  const minutes = Math.floor((safeMs % 3_600_000) / 60_000)
  const seconds = Math.floor((safeMs % 60_000) / 1_000)
  const millis = safeMs % 1_000
  const separator = format === 'srt' ? ',' : '.'
  return `${String(hours).padStart(2, '0')}:${String(minutes).padStart(2, '0')}:${String(seconds).padStart(2, '0')}${separator}${String(millis).padStart(3, '0')}`
}
function safeFilename(value: string) { return value.replace(/[^a-z0-9]+/gi, '-').replace(/^-|-$/g, '').toLowerCase() || 'transcript' }
function copyTextFallback(text: string) {
  const textarea = document.createElement('textarea')
  textarea.value = text
  textarea.setAttribute('readonly', '')
  textarea.style.position = 'fixed'
  textarea.style.opacity = '0'
  document.body.appendChild(textarea)
  textarea.select()
  textarea.setSelectionRange(0, textarea.value.length)
  let copied = false
  try { copied = document.execCommand('copy') } finally { textarea.remove() }
  return copied
}
async function copyToClipboard(text: string): Promise<boolean> {
  if (navigator.clipboard?.writeText) {
    try { await navigator.clipboard.writeText(text); return true } catch { return copyTextFallback(text) }
  }
  return copyTextFallback(text)
}

function downloadTextFile(filename: string, content: string, mime: string) {
  const blob = new Blob([content], { type: mime })
  const href = URL.createObjectURL(blob)
  const anchor = document.createElement('a')
  anchor.href = href
  anchor.download = filename
  anchor.click()
  URL.revokeObjectURL(href)
}

function errorText(cause: unknown) { return cause instanceof ApiError ? cause.message : 'Something went wrong. Please try again.' }
