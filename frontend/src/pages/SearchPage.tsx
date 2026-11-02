import { FormEvent, type CSSProperties, useEffect, useRef, useState } from 'react'
import { ArrowUpRight, Check, Clock3, FileText, FolderOpen, Loader2, Search as SearchIcon, Trash2, Upload, Video, X } from 'lucide-react'
import { toast } from 'sonner'
import { ApiError } from '../api/client'
import { Cue, deleteVideo, getTranscript, ingestYouTube, listSubtitles, listVideos, SearchResult, search, SubtitleOption, uploadTranscript, Video as VideoRecord } from '../api/endpoints'
import { AlertDialog, Badge, Button, Card, Input, Separator, Skeleton } from '../components/ui'

export function SearchPage() {
  const [videos, setVideos] = useState<VideoRecord[]>([])
  const [loadingVideos, setLoadingVideos] = useState(true)
  const [url, setUrl] = useState('')
  const [language, setLanguage] = useState('')
  const [title, setTitle] = useState('')
  const [originalURL, setOriginalURL] = useState('')
  const [subtitleOptions, setSubtitleOptions] = useState<SubtitleOption[]>([])
  const [selectedSubtitle, setSelectedSubtitle] = useState('')
  const [file, setFile] = useState<File | null>(null)
  const [busy, setBusy] = useState(false)
  const [searchQuery, setSearchQuery] = useState('')
  const [searchResults, setSearchResults] = useState<SearchResult[]>([])
  const [searching, setSearching] = useState(false)
  const [searchDuration, setSearchDuration] = useState<{ client: number; server: number } | null>(null)
  const [searchMessage, setSearchMessage] = useState<string | null>(null)
  const [open, setOpen] = useState<string | null>(null)
  const [cues, setCues] = useState<Cue[]>([])
  const [deleteTarget, setDeleteTarget] = useState<VideoRecord | null>(null)
  const fileInput = useRef<HTMLInputElement>(null)

  const refresh = async () => {
    setLoadingVideos(true)
    try { setVideos((await listVideos()).videos) } catch (cause) { toast.error(errorText(cause)) } finally { setLoadingVideos(false) }
  }

  useEffect(() => { void refresh(); if (window.location.hash === '#library') setTimeout(() => document.getElementById('library')?.scrollIntoView(), 0) }, [])
  const discoverSubtitles = async () => {
    if (!url.trim()) { toast.error('Enter a YouTube URL first.'); return }
    setBusy(true)
    try { const result = await listSubtitles(url); setSubtitleOptions(result.subtitles); setSelectedSubtitle(''); toast.success(`Found ${result.subtitles.length} manual subtitle tracks`) } catch (cause) { toast.error(errorText(cause)) } finally { setBusy(false) }
  }

  const submitYouTube = async (event: FormEvent) => {
    event.preventDefault(); setBusy(true)
    try { const subtitle = subtitleOptions.find((option) => `${option.source}:${option.language}` === selectedSubtitle); await ingestYouTube(url, language, subtitle); setUrl(''); setLanguage(''); setSubtitleOptions([]); setSelectedSubtitle(''); toast.success('YouTube transcript queued for indexing'); await refresh() } catch (cause) { toast.error(errorText(cause)) } finally { setBusy(false) }
  }

  const submitUpload = async (event: FormEvent) => {
    event.preventDefault()
    if (!file) { toast.error('Choose a .vtt or .srt file first.'); return }
    setBusy(true)
    try { const data = new FormData(); data.append('file', file); if (title) data.append('title', title); if (originalURL) data.append('original_url', originalURL); if (language) data.append('language', language); await uploadTranscript(data); setFile(null); setTitle(''); setOriginalURL(''); setLanguage(''); if (fileInput.current) fileInput.current.value = ''; toast.success('Transcript uploaded and queued for indexing'); await refresh() } catch (cause) { toast.error(errorText(cause)) } finally { setBusy(false) }
  }

  const showTranscript = async (video: VideoRecord) => {
    if (open === video.id) { setOpen(null); return }
    try { setCues((await getTranscript(video.id)).cues); setOpen(video.id) } catch (cause) { toast.error(errorText(cause)) }
  }

  const confirmDelete = async () => {
    if (!deleteTarget) return
    try { await deleteVideo(deleteTarget.id); setDeleteTarget(null); if (open === deleteTarget.id) setOpen(null); toast.success('Video removed from your library'); await refresh() } catch (cause) { toast.error(errorText(cause)) }
  }

  const submitSearch = async (event: FormEvent) => {
    event.preventDefault()
    if (!searchQuery.trim()) { setSearchMessage('Enter a question or topic to search.'); return }
    const started = performance.now(); setSearching(true); setSearchMessage(null); setSearchDuration(null)
    try { const result = await search(searchQuery); setSearchResults(result.results); setSearchDuration({ client: Math.round(performance.now() - started), server: result.duration_ms }); if (result.results.length === 0) setSearchMessage('No indexed transcript matches found. Try a broader phrase.') } catch (cause) { setSearchMessage(errorText(cause)); setSearchResults([]); toast.error(errorText(cause)) } finally { setSearching(false) }
  }

  return <>
    <section className="hero-section">
      <div className="hero-copy"><p className="eyebrow">Your archive, understood</p><h1>Find the moment<br /><em>that matters.</em></h1><p className="hero-description">Search every indexed conversation, lecture, and idea in your video library using natural language.</p></div>
      <div className="hero-note"><span className="note-line" /><span>Local inference<br />Private by design</span></div>
    </section>

    <section className="search-panel" aria-labelledby="search-heading">
      <form onSubmit={submitSearch}>
        <label id="search-heading" className="sr-only">Search your transcript library</label>
        <SearchIcon className="search-leading-icon" size={20} />
        <Input aria-label="Search query" value={searchQuery} onChange={(event) => { setSearchQuery(event.target.value); setSearchMessage(null) }} placeholder="Ask anything about your videos…" maxLength={2000} />
        {searchQuery && <Button type="button" variant="ghost" size="icon" className="clear-search" aria-label="Clear search" onClick={() => setSearchQuery('')}><X size={17} /></Button>}
        <span className="shortcut">⌘ K</span><Button type="submit" disabled={searching} size="lg">{searching ? <><Loader2 className="spin" size={17} /> Searching</> : <>Search <ArrowUpRight size={17} /></>}</Button>
      </form>
      {!searchResults.length && !searchDuration && !searchMessage && <div className="search-hint"><span>Try a question</span><button type="button" onClick={() => setSearchQuery('How does database replication work?')}>“How does database replication work?”</button><span className="hint-separator">or</span><button type="button" onClick={() => setSearchQuery('Find the part where they discuss the future of AI')}>“Find the part where they discuss the future of AI”</button></div>}
    </section>

    {(searchDuration || searchMessage || searchResults.length > 0) && <section className="results-section" aria-live="polite">
      <div className="section-heading"><div><p className="eyebrow">Semantic retrieval</p><h2>{searchResults.length ? 'Relevant moments' : 'Search complete'}</h2></div>{searchDuration && <div className="search-metrics"><strong>{searchResults.length}</strong> matches <span className="metric-divider" /> resolved in <strong>{searchDuration.client} ms</strong><small></small></div>}</div>
      {searchMessage && <div className="empty-state"><SearchIcon size={20} /><p>{searchMessage}</p></div>}
      <div className="results-list">{searchResults.map((result, index) => <ResultCard key={result.chunk_id} result={result} query={searchQuery} index={index} />)}</div>
    </section>}

    <Separator />
    <section className="ingest-section" aria-labelledby="ingest-heading"><div className="section-heading"><div><p className="eyebrow">Build your library</p><h2 id="ingest-heading">Bring in a new source</h2></div><p className="section-description">Sutra turns captions into timestamped, searchable moments. Everything stays on this machine.</p></div>
      <div className="ingest-grid">
        <Card className="ingest-card youtube-card"><div className="card-icon youtube-icon"><Video size={19} /></div><div className="card-title-row"><div><h3>YouTube video</h3><p>Discover subtitles directly from a video URL.</p></div><Badge tone="accent">Recommended</Badge></div><form onSubmit={submitYouTube} className="stack-form"><Input required value={url} onChange={(event) => { setUrl(event.target.value); setSubtitleOptions([]); setSelectedSubtitle('') }} placeholder="youtube.com/watch?v=…" /><Button type="button" variant="secondary" onClick={discoverSubtitles} disabled={busy}>{busy ? <Loader2 className="spin" size={16} /> : <SearchIcon size={16} />} Find subtitles</Button>{subtitleOptions.length > 0 && <select required className="ui-input ui-select" value={selectedSubtitle} onChange={(event) => setSelectedSubtitle(event.target.value)}><option value="">Select a manual subtitle track</option>{subtitleOptions.map((option) => <option key={`${option.source}:${option.language}`} value={`${option.source}:${option.language}`}>{option.language}{option.name ? ` — ${option.name}` : ''} ({option.formats.join(', ')})</option>)}</select>}<Input value={language} onChange={(event) => setLanguage(event.target.value)} placeholder="Language preference (optional)" /><Button disabled={busy || subtitleOptions.length === 0}>{busy ? <><Loader2 className="spin" size={16} /> Adding to library</> : <>Fetch transcript <ArrowUpRight size={16} /></>}</Button></form></Card>
        <Card className="ingest-card upload-card"><div className="card-icon upload-icon"><Upload size={19} /></div><div className="card-title-row"><div><h3>Transcript file</h3><p>Upload captions you already have.</p></div><Badge tone="muted">.VTT · .SRT</Badge></div><form onSubmit={submitUpload} className="stack-form"><button type="button" className={`drop-zone ${file ? 'has-file' : ''}`} onClick={() => fileInput.current?.click()} onDragOver={(event) => event.preventDefault()} onDrop={(event) => { event.preventDefault(); setFile(event.dataTransfer.files.item(0)) }}><input ref={fileInput} type="file" accept=".vtt,.srt" hidden onChange={(event) => setFile(event.target.files?.[0] ?? null)} />{file ? <><Check size={20} /><strong>{file.name}</strong><span>Ready to upload</span></> : <><FileText size={22} /><strong>Drop a transcript here</strong><span>or browse from your computer</span></>}</button><Input value={title} onChange={(event) => setTitle(event.target.value)} placeholder="Title (optional)" /><Input value={originalURL} onChange={(event) => setOriginalURL(event.target.value)} placeholder="Original YouTube URL (optional)" /><Button disabled={busy || !file}>{busy ? <><Loader2 className="spin" size={16} /> Uploading</> : <>Upload transcript <ArrowUpRight size={16} /></>}</Button></form></Card>
      </div>
    </section>

    <section className="library-section" id="library" aria-labelledby="library-heading"><div className="section-heading"><div><p className="eyebrow">Your collection</p><h2 id="library-heading">Video library</h2></div>{videos.length > 0 && <Badge tone="muted">{videos.length} {videos.length === 1 ? 'source' : 'sources'}</Badge>}</div>{loadingVideos ? <div className="library-list"><Skeleton className="library-skeleton" /><Skeleton className="library-skeleton" /></div> : videos.length === 0 ? <div className="empty-library"><FolderOpen size={22} /><h3>Your library is quiet</h3><p>Add a YouTube video or transcript above to start searching.</p></div> : <div className="library-list">{videos.map((video) => <VideoCard key={video.id} video={video} open={open === video.id} cues={cues} onToggle={() => void showTranscript(video)} onDelete={() => setDeleteTarget(video)} />)}</div>}</section>
    <AlertDialog open={deleteTarget !== null} title="Remove this video?" description="This deletes the video, transcript cues, and indexed moments. This cannot be undone." confirmLabel="Delete video" onCancel={() => setDeleteTarget(null)} onConfirm={() => void confirmDelete()} />
  </>
}

function ResultCard({ result, query, index }: { result: SearchResult; query: string; index: number }) {
  return <article className="result-card" style={{ '--delay': `${index * 55}ms` } as CSSProperties}><div className="result-index">{String(index + 1).padStart(2, '0')}</div><div className="result-content"><div className="result-topline"><Badge tone="accent"><Clock3 size={12} /> {formatMs(result.start_ms)} – {formatMs(result.end_ms)}</Badge><span className="result-score">{Math.round(result.score * 100)}% relevance</span></div><h3>{result.video_title || 'Untitled video'}</h3><p className="result-excerpt"><Highlight text={result.text} query={query} /></p><div className="result-footer"><span><Video size={13} /> </span><a href={result.timestamp_url || result.original_url || '#'} target="_blank" rel="noreferrer">Open moment <ArrowUpRight size={14} /></a></div></div></article>
}

function VideoCard({ video, open, cues, onToggle, onDelete }: { video: VideoRecord; open: boolean; cues: Cue[]; onToggle: () => void; onDelete: () => void }) {
  return <Card className="video-card"><div className="video-card-main"><div className="video-thumb"><Video size={20} /><span>{video.source_type === 'youtube' ? 'YT' : 'FILE'}</span></div><div className="video-info"><div className="video-heading"><h3 title={video.title}>{video.title || 'Untitled video'}</h3><Badge tone={video.status === 'ready' || video.status === 'succeeded' ? 'success' : 'muted'}>{video.status}</Badge></div><div className="video-meta"><span>{video.source_type === 'youtube' ? 'YouTube' : 'Uploaded transcript'}</span><span>{video.language || 'Language unknown'}</span><span>{video.transcript?.cue_count ?? 0} cues</span></div></div><div className="video-actions"><Button variant="secondary" size="sm" onClick={onToggle}>{open ? 'Hide transcript' : 'View transcript'}</Button><Button variant="ghost" size="icon" aria-label={`Delete ${video.title || 'video'}`} onClick={onDelete}><Trash2 size={16} /></Button></div></div>{open && <div className="transcript-viewer"><div className="transcript-header"><span>Transcript cues</span><Badge tone="muted">{cues.length} cues</Badge></div>{cues.map((cue) => <div className="cue-row" key={cue.id}><time>{formatMs(cue.start_ms)}</time><p>{cue.text}</p></div>)}</div>}</Card>
}

function Highlight({ text, query }: { text: string; query: string }) {
  const terms = query.trim().split(/\s+/).filter((term) => term.length > 2).slice(0, 6)
  if (!terms.length) return <>{text}</>
  const pattern = new RegExp(`(${terms.map(escapeRegExp).join('|')})`, 'gi')
  return <>{text.split(pattern).map((part, index) => terms.some((term) => part.toLowerCase() === term.toLowerCase()) ? <mark key={index}>{part}</mark> : part)}</>
}

function escapeRegExp(value: string) { return value.replace(/[.*+?^${}()|[\]\\]/g, '\\$&') }
function formatMs(ms: number) { return `${Math.floor(ms / 60000)}:${String(Math.floor(ms / 1000) % 60).padStart(2, '0')}` }
function errorText(cause: unknown) { return cause instanceof ApiError ? cause.message : 'Something went wrong. Please try again.' }
