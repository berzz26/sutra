import { useEffect, useDeferredValue, useMemo, useState } from 'react'
import { useSearchParams } from 'react-router-dom'
import { Search as SearchIcon } from 'lucide-react'
import { toast } from 'sonner'
import { getProcessing, getTranscript, getVideo, ingestYouTube, listSubtitles, search, uploadTranscript } from '../api/endpoints'
import type { Cue, ProcessingState, SearchResult, SubtitleOption, Video as VideoRecord, YouTubeMetadata } from '../api/endpoints'
import { AlertDialog, Skeleton } from '../components/ui'
import { ApiError } from '../api/client'
import { HeroSection } from '../components/HeroSection'
import { SearchWorkspace, type WorkspaceTab } from '../components/search/SearchWorkspace'
import { ResultCard } from '../components/search/ResultCard'
import { RecentSearches } from '../components/search/RecentSearches'
import { LibrarySection } from '../components/search/LibrarySection'
import { errorText } from '../lib/format'
import { getLocalLibraryIDs, addVideoToLibrary, removeVideoFromLibrary, subscribeLocalLibrary } from '../lib/localLibrary'
import { loadRecentSearches, rememberSearch, type RecentSearch } from '../lib/recentSearches'

const TRANSCRIPT_PAGE_SIZE = 200

export function SearchPage() {
  // videos is metadata for the videos saved in this browser's local library
  // only — never the shared global listing.
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
  const [removeTarget, setRemoveTarget] = useState<VideoRecord | null>(null)

  const [recentSearches, setRecentSearches] = useState<RecentSearch[]>(() => loadRecentSearches())
  const [searchParams] = useSearchParams()

  const loadLibrary = async () => {
    setLoadingVideos(true)
    try {
      const ids = getLocalLibraryIDs()
      if (ids.length === 0) {
        setVideos([])
        return
      }
      // Metadata is fetched per ID from the existing single-video endpoint;
      // the global listing is administrative and never used here.
      const settled = await Promise.allSettled(ids.map((id) => getVideo(id)))
      const loaded: VideoRecord[] = []
      const staleIDs: string[] = []
      settled.forEach((outcome, index) => {
        if (outcome.status === 'fulfilled') loaded.push(outcome.value)
        else if (isNotFound(outcome.reason)) staleIDs.push(ids[index])
      })
      // References to videos deleted from the database are dead weight; they
      // can never resolve again, so drop them quietly.
      if (staleIDs.length > 0) {
        for (const id of staleIDs) removeVideoFromLibrary(id)
        toast.info(`${staleIDs.length} unavailable ${staleIDs.length === 1 ? 'video was' : 'videos were'} removed from this browser's library.`)
      }
      loaded.sort((a, b) => (b.created_at > a.created_at ? 1 : -1))
      setVideos(loaded)
    } catch (cause) {
      toast.error(errorText(cause))
    } finally {
      setLoadingVideos(false)
    }
  }

  // Keep component state synchronized with localStorage (including changes
  // made in other tabs of the same browser).
  useEffect(() => subscribeLocalLibrary(() => void loadLibrary()), [])

  useEffect(() => { void loadLibrary() }, [])

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
      // The API can omit these arrays entirely (e.g. videos with no
      // translated tracks); coerce so downstream `.some` calls never see
      // null/undefined.
      setSubtitleOptions(Array.isArray(result.subtitles) ? result.subtitles : [])
      setTranslatedSubtitleOptions(Array.isArray(result.translated_subtitles) ? result.translated_subtitles : [])
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

  // Called only after ingestion completes successfully: the ID is the actual
  // database ID returned by the backend, and it is saved to this browser's
  // local library. Failed ingestions never reach this point.
  const adoptIndexedVideo = (result: VideoRecord) => {
    addVideoToLibrary(result.id)
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
      adoptIndexedVideo(result)
      toast.success('Video indexed and added to your library')
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
      adoptIndexedVideo(result)
      toast.success('Transcript indexed and added to your library')
    } catch (cause) { toast.error(errorText(cause)); setPipeline((current) => current ? { ...current, stage: 'failed' } : null) } finally { setBusy(false) }
  }

  /* ---------- Search ---------- */

  const executeSearch = async (query: string, scope: 'video' | 'library', videoId?: string) => {
    if (!query.trim()) { setSearchMessage('Enter a question or topic to search.'); return }
    if (scope === 'video' && !videoId) { setSearchMessage('Fetch and index a video before searching within it.'); return }
    // Entire Library means "this browser's saved videos" — the request is
    // always sent with an explicit ID list, never as a global search.
    const libraryIDs = getLocalLibraryIDs()
    if (scope === 'library' && libraryIDs.length === 0) {
      setSearchResults([]); setSearchDuration(null)
      setSearchMessage('Your library is empty here. Add a video first and it will be searchable.')
      return
    }
    const started = performance.now()
    setSearching(true); setSearchMessage(null); setSearchDuration(null)
    try {
      const result = await search(query, 10, scope === 'video' ? videoId : undefined, scope === 'library' ? libraryIDs : undefined)
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
    const stillSaved = entry.videoId ? videos.some((video) => video.id === entry.videoId) : false
    if (entry.scope === 'video' && stillSaved && entry.videoId) {
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

  /* ---------- Library management (browser-local) ---------- */

  // Removing a video only drops its ID from this browser's localStorage —
  // the shared database record and other browsers are untouched.
  const confirmRemove = () => {
    if (!removeTarget) return
    const id = removeTarget.id
    removeVideoFromLibrary(id)
    setRemoveTarget(null)
    if (currentVideo?.id === id) { setCurrentVideo(null); setSearchResults([]); setSearchDuration(null); setSearchMessage(null) }
    if (browseSelected?.id === id) { setBrowseSelected(null); setBrowseCues([]) }
    if (open === id) { setOpen(null); setCues([]) }
    toast.success('Removed from this browser\'s library. The shared database record is untouched.')
  }

  /* ---------- Transcripts ---------- */

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
        onDelete={setRemoveTarget}
      />
    )}

    <AlertDialog
      open={removeTarget !== null}
      title="Remove from your library?"
      description="This removes the video from this browser's library only. The shared database record and other browsers are unaffected."
      confirmLabel="Remove from library"
      onCancel={() => setRemoveTarget(null)}
      onConfirm={confirmRemove}
    />
  </>
}

function isNotFound(cause: unknown) {
  return cause instanceof ApiError && cause.status === 404
}
