import { useEffect, useDeferredValue, useMemo, useState } from 'react'
import { useSearchParams } from 'react-router-dom'
import { Search as SearchIcon } from 'lucide-react'
import { toast } from 'sonner'
import { deleteVideo, fetchAllVideos, getProcessing, getTranscript, ingestYouTube, listSubtitles, search, uploadTranscript } from '../api/endpoints'
import type { Cue, ProcessingState, SearchResult, SubtitleOption, Video as VideoRecord, YouTubeMetadata } from '../api/endpoints'
import { Skeleton } from '../components/ui'
import { AlertDialog } from '../components/ui'
import { HeroSection } from '../components/HeroSection'
import { SearchWorkspace, type WorkspaceTab } from '../components/search/SearchWorkspace'
import { ResultCard } from '../components/search/ResultCard'
import { RecentSearches } from '../components/search/RecentSearches'
import { LibrarySection } from '../components/search/LibrarySection'
import { errorText } from '../lib/format'
import { loadRecentSearches, rememberSearch, type RecentSearch } from '../lib/recentSearches'

const TRANSCRIPT_PAGE_SIZE = 200

export function SearchPage() {
  const [videos, setVideos] = useState<VideoRecord[]>([])
  const [currentVideo, setCurrentVideo] = useState<VideoRecord | null>(null)
  const [searchScope, setSearchScope] = useState<'video' | 'library'>('video')
  const [loadingVideos, setLoadingVideos] = useState(true)

  const [tab, setTab] = useState<WorkspaceTab>('youtube')
  const [url, setUrl] = useState('')
  const [language] = useState('en')
  const [uploadTitle, setUploadTitle] = useState('')
  const [uploadURL, setUploadURL] = useState('')
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

  const [recentSearches, setRecentSearches] = useState<RecentSearch[]>(() => loadRecentSearches())
  const [searchParams] = useSearchParams()

  const refresh = async () => {
    setLoadingVideos(true)
    try { setVideos(await fetchAllVideos()) } catch (cause) { toast.error(errorText(cause)) } finally { setLoadingVideos(false) }
  }

  useEffect(() => { void refresh() }, [])

  // Navbar "Library" links to /?tab=library; keep the workspace tab in sync.
  useEffect(() => {
    const requested = searchParams.get('tab')
    if (requested === 'library') setTab('library')
  }, [searchParams])

  useEffect(() => {
    if (window.location.hash === '#library') setTimeout(() => document.getElementById('library')?.scrollIntoView(), 0)
  }, [])

  /* ---------- Ingestion (unchanged API flow) ---------- */

  const clearIngestState = () => {
    setUrl(''); setYoutubePreview(null); setSubtitleOptions([]); setTranslatedSubtitleOptions([]); setSelectedSubtitle(''); setFile(null); setUploadTitle(''); setUploadURL('')
  }

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

  const selectIndexedVideo = (result: VideoRecord) => {
    setCurrentVideo(result); setSearchScope('video'); setSearchResults([]); setSearchDuration(null); setSearchMessage(null)
    clearIngestState(); setPipeline(null)
  }

  const submitYouTubeIndex = async () => {
    setBusy(true); setPipeline({ progress: 15, stage: 'fetching subtitles', state: null })
    try {
      const subtitle = [...subtitleOptions, ...translatedSubtitleOptions].find((option) => `${option.source}:${option.language}` === selectedSubtitle)
      const result = await ingestYouTube(url, language, subtitle)
      setPipeline({ progress: 45, stage: 'queued for indexing', state: null })
      await waitForProcessing(result.id)
      selectIndexedVideo(result)
      toast.success('Video indexed and ready to search')
      await refresh()
    } catch (cause) { toast.error(errorText(cause)); setPipeline((current) => current ? { ...current, stage: 'failed' } : null) } finally { setBusy(false) }
  }

  const submitUpload = async () => {
    if (!file) { toast.error('Choose a .vtt or .srt file first.'); return }
    setBusy(true); setPipeline({ progress: 15, stage: 'uploading transcript', state: null })
    try {
      const data = new FormData()
      data.append('file', file)
      if (uploadTitle) data.append('title', uploadTitle)
      if (uploadURL) data.append('original_url', uploadURL)
      data.append('language', language)
      const result = await uploadTranscript(data)
      setPipeline({ progress: 45, stage: 'queued for indexing', state: null })
      await waitForProcessing(result.id)
      selectIndexedVideo(result)
      toast.success('Transcript indexed and ready to search')
      await refresh()
    } catch (cause) { toast.error(errorText(cause)); setPipeline((current) => current ? { ...current, stage: 'failed' } : null) } finally { setBusy(false) }
  }

  /* ---------- Search ---------- */

  const executeSearch = async (query: string, scope: 'video' | 'library', videoId?: string) => {
    if (!query.trim()) { setSearchMessage('Enter a question or topic to search.'); return }
    if (scope === 'video' && !videoId) { setSearchMessage('Fetch and index a video before searching within it.'); return }
    const started = performance.now()
    setSearching(true); setSearchMessage(null); setSearchDuration(null)
    try {
      const result = await search(query, 10, scope === 'video' ? videoId : undefined)
      setSearchResults(result.results)
      setSearchDuration({ client: Math.round(performance.now() - started), server: result.duration_ms })
      if (result.results.length === 0) setSearchMessage('No indexed transcript matches found. Try a broader phrase.')
      rememberSearch(query, scope, videos.find((video) => video.id === videoId) ?? null, result.results[0] ?? null, videos)
      setRecentSearches(loadRecentSearches())
    } catch (cause) {
      setSearchMessage(errorText(cause)); setSearchResults([]); toast.error(errorText(cause))
    } finally { setSearching(false) }
  }

  const onSuggestionPick = (query: string) => {
    setSearchQuery(query)
    if (tab === 'youtube' && !currentVideo) {
      setTab('library')
      void executeSearch(query, 'library')
    } else {
      void executeSearch(query, searchScope, currentVideo?.id)
    }
  }

  const runRecentSearch = (entry: RecentSearch) => {
    setSearchQuery(entry.query)
    const stillAvailable = entry.videoId ? videos.some((video) => video.id === entry.videoId) : false
    if (entry.scope === 'video' && stillAvailable && entry.videoId) {
      const video = videos.find((candidate) => candidate.id === entry.videoId) ?? null
      setCurrentVideo(video); setSearchScope('video'); setTab('youtube')
      void executeSearch(entry.query, 'video', entry.videoId)
    } else {
      setSearchScope('library'); setTab('library')
      void executeSearch(entry.query, 'library')
    }
  }

  const changeSearchScope = (scope: 'video' | 'library') => {
    setSearchScope(scope)
    setSearchResults([]); setSearchDuration(null); setSearchMessage(null)
  }

  const changeTab = (next: WorkspaceTab) => {
    setTab(next)
    if (next === 'library') setSearchScope('library')
    else setSearchScope(currentVideo ? searchScope : 'video')
  }

  const changeVideo = () => {
    setCurrentVideo(null)
    setSearchResults([]); setSearchDuration(null); setSearchMessage(null); setSearchQuery('')
    clearIngestState()
  }

  /* ---------- Transcripts & library ---------- */

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
    setTranscriptLoading(true); setOpen(video.id)
    try { setCues(await fetchAllCues(video)) } catch (cause) { setOpen(null); toast.error(errorText(cause)) } finally { setTranscriptLoading(false) }
  }

  const openBrowseVideo = async (video: VideoRecord) => {
    if (browseSelected?.id === video.id && browseCues.length >= (video.transcript?.cue_count ?? 0)) return
    setBrowseSelected(video); setBrowseLoading(true); setBrowseCues([])
    try { setBrowseCues(await fetchAllCues(video)) } catch (cause) { toast.error(errorText(cause)) } finally { setBrowseLoading(false) }
  }

  const confirmDelete = async () => {
    if (!deleteTarget) return
    try {
      await deleteVideo(deleteTarget.id)
      setDeleteTarget(null)
      if (open === deleteTarget.id) { setOpen(null); setCues([]) }
      if (browseSelected?.id === deleteTarget.id) { setBrowseSelected(null); setBrowseCues([]) }
      if (currentVideo?.id === deleteTarget.id) { setCurrentVideo(null); setSearchResults([]); setSearchDuration(null); setSearchMessage(null) }
      toast.success('Video removed from your library')
      await refresh()
    } catch (cause) { toast.error(errorText(cause)) }
  }

  // Defer the filter so typing stays instant.
  const browseFilter = useDeferredValue(browseQuery).trim().toLowerCase()
  const browseResults = useMemo(() => browseFilter
    ? videos.filter((video) => (video.title || '').toLowerCase().includes(browseFilter) || (video.channel || '').toLowerCase().includes(browseFilter))
    : videos, [videos, browseFilter])

  const showResults = searching || Boolean(searchDuration) || Boolean(searchMessage) || searchResults.length > 0

  return <>
    <HeroSection />

    <SearchWorkspace
      tab={tab}
      onTabChange={changeTab}
      currentVideo={currentVideo}
      onChangeVideo={changeVideo}
      searchScope={searchScope}
      onScopeChange={changeSearchScope}
      libraryCount={videos.length}
      searchQuery={searchQuery}
      onSearchQueryChange={(value) => { setSearchQuery(value); setSearchMessage(null) }}
      searching={searching}
      onSearchSubmit={() => void executeSearch(searchQuery, searchScope, currentVideo?.id)}
      onSuggestionPick={onSuggestionPick}
      url={url}
      onUrlChange={setUrl}
      busy={busy}
      youtubePreview={youtubePreview}
      subtitleOptions={subtitleOptions}
      translatedSubtitleOptions={translatedSubtitleOptions}
      selectedSubtitle={selectedSubtitle}
      onSelectSubtitle={setSelectedSubtitle}
      onFetchMetadata={() => void discoverSubtitles()}
      onIndexVideo={() => void submitYouTubeIndex()}
      pipeline={pipeline}
      file={file}
      onFileChange={setFile}
      uploadTitle={uploadTitle}
      onUploadTitleChange={setUploadTitle}
      uploadURL={uploadURL}
      onUploadURLChange={setUploadURL}
      onUpload={() => void submitUpload()}
      videos={videos}
      browseQuery={browseQuery}
      onBrowseQueryChange={setBrowseQuery}
      browseResults={browseResults}
      browseSelected={browseSelected}
      onSelectBrowse={(video) => void openBrowseVideo(video)}
      browseCues={browseCues}
      browseLoading={browseLoading}
    />

    {showResults && (
      <section className="results-section" aria-live="polite">
        <div className="section-heading">
          <h2><SearchIcon size={17} /> Relevant moments</h2>
          <span className="spacer" />
          {searchDuration && (
            <span className="search-metrics"><strong>{searchResults.length}</strong> matches <span className="metric-divider" /> resolved in <strong>{searchDuration.client} ms</strong></span>
          )}
        </div>
        {searching && <div className="result-loading-list"><Skeleton className="result-loading-card" /><Skeleton className="result-loading-card" /></div>}
        {!searching && searchMessage && <div className="empty-state"><SearchIcon size={20} /><p>{searchMessage}</p></div>}
        {!searching && searchResults.length > 0 && (
          <div style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
            {searchResults.map((result, index) => (
              <ResultCard key={result.chunk_id} result={result} video={videos.find((video) => video.id === result.video_id)} query={searchQuery} index={index} />
            ))}
          </div>
        )}
      </section>
    )}

    <RecentSearches items={recentSearches} onRun={runRecentSearch} />

    {tab !== 'find' && (
      <LibrarySection
        videos={videos}
        loading={loadingVideos}
        openId={open}
        cues={cues}
        cuesLoading={transcriptLoading}
        onToggle={(video) => void showTranscript(video)}
        onDelete={setDeleteTarget}
      />
    )}

    <AlertDialog
      open={deleteTarget !== null}
      title="Remove this video?"
      description="This deletes the video, transcript cues, and indexed moments. This cannot be undone."
      confirmLabel="Delete video"
      onCancel={() => setDeleteTarget(null)}
      onConfirm={() => void confirmDelete()}
    />
  </>
}
