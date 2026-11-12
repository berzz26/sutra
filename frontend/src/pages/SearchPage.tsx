import { useEffect, useDeferredValue, useMemo, useState, useCallback, useRef } from 'react'
import { useSearchParams } from 'react-router-dom'
import { Search as SearchIcon } from 'lucide-react'
import { toast } from 'sonner'
import {
  getProcessing,
  getTranscript,
  getVideo,
  ingestYouTube,
  listSubtitles,
  search,
  uploadTranscript,
} from '../api/endpoints'
import type {
  Cue,
  ProcessingState,
  SearchResult,
  SubtitleOption,
  Video as VideoRecord,
  YouTubeMetadata,
} from '../api/endpoints'
import { AlertDialog, Skeleton } from '../components/ui'
import { ApiError } from '../api/client'
import { HeroSection } from '../components/HeroSection'
import { SearchWorkspace, type WorkspaceTab } from '../components/search/SearchWorkspace'
import { ResultCard } from '../components/search/ResultCard'
import { RecentSearches } from '../components/search/RecentSearches'
import { LibrarySection } from '../components/search/LibrarySection'
import { errorText } from '../lib/format'
import {
  getLocalLibraryIDs,
  addVideoToLibrary,
  removeVideoFromLibrary,
  subscribeLocalLibrary,
} from '../lib/localLibrary'
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
  const [pipeline, setPipeline] = useState<{
    progress: number
    stage: string
    state: ProcessingState | null
  } | null>(null)

  // FIX: Decouple live input from submitted query.
  // - searchQuery   = what's in the text box (updates on every keystroke)
  // - submittedQuery = the query that was actually searched (drives ResultCard highlights)
  // This stops highlights re-rendering on every keystroke.
  const [searchQuery, setSearchQuery] = useState('')
  const [submittedQuery, setSubmittedQuery] = useState('')
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

  // Keep a stable ref to videos for use inside callbacks without re-creating them
  const videosRef = useRef(videos)
  useEffect(() => { videosRef.current = videos }, [videos])

  // FIX: Stable loadLibrary with useCallback so it doesn't re-create on every render
  const loadLibrary = useCallback(async () => {
    setLoadingVideos(true)
    try {
      const ids = getLocalLibraryIDs()
      if (ids.length === 0) {
        setVideos([])
        return
      }
      const settled = await Promise.allSettled(ids.map((id) => getVideo(id)))
      const loaded: VideoRecord[] = []
      const staleIDs: string[] = []
      settled.forEach((outcome, index) => {
        if (outcome.status === 'fulfilled') loaded.push(outcome.value)
        else if (isNotFound(outcome.reason)) staleIDs.push(ids[index])
      })
      if (staleIDs.length > 0) {
        for (const id of staleIDs) removeVideoFromLibrary(id)
        toast.info(
          `${staleIDs.length} unavailable ${staleIDs.length === 1 ? 'video was' : 'videos were'} removed from this browser's library.`,
        )
      }
      loaded.sort((a, b) => (b.created_at > a.created_at ? 1 : -1))
      setVideos(loaded)
    } catch (cause) {
      toast.error(errorText(cause))
    } finally {
      setLoadingVideos(false)
    }
  }, [])

  useEffect(() => subscribeLocalLibrary(() => void loadLibrary()), [loadLibrary])
  useEffect(() => { void loadLibrary() }, [loadLibrary])

  useEffect(() => {
    const requested = searchParams.get('tab')
    if (requested === 'library') setTab('library')
  }, [searchParams])

  useEffect(() => {
    if (window.location.hash === '#library')
      setTimeout(() => document.getElementById('library')?.scrollIntoView(), 0)
  }, [])

  /* ---------- Ingestion ---------- */

  const clearIngestState = useCallback(() => {
    setUrl('')
    setYoutubePreview(null)
    setSubtitleOptions([])
    setTranslatedSubtitleOptions([])
    setSelectedSubtitle('')
    setFile(null)
    setUploadTitle('')
    setUploadURL('')
  }, [])

  const discoverSubtitles = useCallback(async () => {
    if (!url.trim()) { toast.error('Enter a YouTube URL first.'); return }
    setBusy(true)
    try {
      const result = await listSubtitles(url)
      setYoutubePreview(result.metadata)
      setSubtitleOptions(Array.isArray(result.subtitles) ? result.subtitles : [])
      setTranslatedSubtitleOptions(
        Array.isArray(result.translated_subtitles) ? result.translated_subtitles : [],
      )
      const english = result.subtitles.find((option) => /^en(?:[-_]|$)/i.test(option.language))
      setSelectedSubtitle(english ? `${english.source}:${english.language}` : '')
      toast.success(`Found ${result.subtitles.length} subtitle tracks`)
    } catch (cause) {
      toast.error(errorText(cause))
    } finally {
      setBusy(false)
    }
  }, [url])

  const waitForProcessing = useCallback(async (videoID: string) => {
    for (let attempt = 0; attempt < 240; attempt += 1) {
      const state = await getProcessing(videoID)
      setPipeline({ progress: state.progress_percent, stage: state.stage, state })
      if (state.status === 'completed') return
      if (state.status === 'failed') throw new Error(state.last_error || 'Transcript indexing failed.')
      await new Promise((resolve) => window.setTimeout(resolve, 750))
    }
    throw new Error('Indexing is taking longer than expected. Check the library for status.')
  }, [])

  const adoptIndexedVideo = useCallback(
    (result: VideoRecord) => {
      addVideoToLibrary(result.id)
      setCurrentVideo(result)
      setSearchScope('video')
      setSearchResults([])
      setSearchDuration(null)
      setSearchMessage(null)
      setSubmittedQuery('')
      clearIngestState()
      setPipeline(null)
    },
    [clearIngestState],
  )

  const submitYouTubeIndex = useCallback(async () => {
    setBusy(true)
    setPipeline({ progress: 15, stage: 'fetching subtitles', state: null })
    try {
      const subtitle = [...subtitleOptions, ...translatedSubtitleOptions].find(
        (option) => `${option.source}:${option.language}` === selectedSubtitle,
      )
      const result = await ingestYouTube(url, language, subtitle)
      setPipeline({ progress: 45, stage: 'queued for indexing', state: null })
      await waitForProcessing(result.id)
      adoptIndexedVideo(result)
      toast.success('Video indexed and added to your library')
    } catch (cause) {
      toast.error(errorText(cause))
      setPipeline((current) => (current ? { ...current, stage: 'failed' } : null))
    } finally {
      setBusy(false)
    }
  }, [subtitleOptions, translatedSubtitleOptions, selectedSubtitle, url, language, waitForProcessing, adoptIndexedVideo])

  const submitUpload = useCallback(async () => {
    if (!file) { toast.error('Choose a .vtt or .srt file first.'); return }
    setBusy(true)
    setPipeline({ progress: 15, stage: 'uploading transcript', state: null })
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
    } catch (cause) {
      toast.error(errorText(cause))
      setPipeline((current) => (current ? { ...current, stage: 'failed' } : null))
    } finally {
      setBusy(false)
    }
  }, [file, uploadTitle, uploadURL, language, waitForProcessing, adoptIndexedVideo])

  /* ---------- Search ---------- */

  // FIX: Use a ref to track in-flight search so a new one cancels the stale one's state updates.
  const searchAbortRef = useRef<symbol | null>(null)

  const executeSearch = useCallback(
    async (query: string, scope: 'video' | 'library', videoId?: string) => {
      if (!query.trim()) {
        setSearchMessage('Enter a question or topic to search.')
        return
      }
      if (scope === 'video' && !videoId) {
        setSearchMessage('Fetch and index a video before searching within it.')
        return
      }
      const libraryIDs = getLocalLibraryIDs()
      if (scope === 'library' && libraryIDs.length === 0) {
        setSearchResults([])
        setSearchDuration(null)
        setSearchMessage('Your library is empty here. Add a video first and it will be searchable.')
        return
      }

      // FIX: Tag this search run; if a new search starts before this one finishes,
      // the old one's setState calls are ignored — prevents "second search doesn't work"
      // caused by stale state from overlapping async calls.
      const token = Symbol()
      searchAbortRef.current = token

      const started = performance.now()

      // FIX: Clear previous results immediately so the UI feels responsive on re-search
      setSearchResults([])
      setSearchDuration(null)
      setSearchMessage(null)
      setSearching(true)

      // FIX: Set submittedQuery HERE (not on input change) so highlights only
      // appear after a real search, not on every keystroke.
      setSubmittedQuery(query)

      try {
        const result = await search(
          query,
          10,
          scope === 'video' ? videoId : undefined,
          scope === 'library' ? libraryIDs : undefined,
        )

        // If a newer search was started, discard this stale result
        if (searchAbortRef.current !== token) return

        setSearchResults(result.results)
        setSearchDuration({ client: Math.round(performance.now() - started), server: result.duration_ms })
        if (result.results.length === 0) {
          setSearchMessage('No indexed transcript matches found. Try a broader phrase.')
        }

        const currentVideos = videosRef.current
        rememberSearch(
          query,
          scope,
          currentVideos.find((v) => v.id === videoId) ?? null,
          result.results[0] ?? null,
          currentVideos,
        )
        setRecentSearches(loadRecentSearches())
      } catch (cause) {
        if (searchAbortRef.current !== token) return
        setSearchMessage(errorText(cause))
        setSearchResults([])
        toast.error(errorText(cause))
      } finally {
        if (searchAbortRef.current === token) setSearching(false)
      }
    },
    [],
  )

  const onSuggestionPick = useCallback(
    (query: string) => {
      setSearchQuery(query)
      if (tab === 'youtube' && !currentVideo) {
        setTab('library')
        void executeSearch(query, 'library')
      } else {
        void executeSearch(query, searchScope, currentVideo?.id)
      }
    },
    [tab, currentVideo, searchScope, executeSearch],
  )

  const runRecentSearch = useCallback(
    (entry: RecentSearch) => {
      setSearchQuery(entry.query)
      const stillSaved = entry.videoId
        ? videosRef.current.some((v) => v.id === entry.videoId)
        : false
      if (entry.scope === 'video' && stillSaved && entry.videoId) {
        const video = videosRef.current.find((v) => v.id === entry.videoId) ?? null
        setCurrentVideo(video)
        setSearchScope('video')
        setTab('youtube')
        void executeSearch(entry.query, 'video', entry.videoId)
      } else {
        setSearchScope('library')
        setTab('library')
        void executeSearch(entry.query, 'library')
      }
    },
    [executeSearch],
  )

  const changeSearchScope = useCallback((scope: 'video' | 'library') => {
    setSearchScope(scope)
    setSearchResults([])
    setSearchDuration(null)
    setSearchMessage(null)
  }, [])

  const changeTab = useCallback(
    (next: WorkspaceTab) => {
      setTab(next)
      if (next === 'library') setSearchScope('library')
      else setSearchScope(currentVideo ? searchScope : 'video')
    },
    [currentVideo, searchScope],
  )

  const changeVideo = useCallback(() => {
    setCurrentVideo(null)
    setSearchResults([])
    setSearchDuration(null)
    setSearchMessage(null)
    setSearchQuery('')
    setSubmittedQuery('')
    clearIngestState()
  }, [clearIngestState])

  /* ---------- Library management ---------- */

  const confirmRemove = useCallback(() => {
    if (!removeTarget) return
    const id = removeTarget.id
    removeVideoFromLibrary(id)
    setRemoveTarget(null)
    if (currentVideo?.id === id) {
      setCurrentVideo(null)
      setSearchResults([])
      setSearchDuration(null)
      setSearchMessage(null)
    }
    if (browseSelected?.id === id) {
      setBrowseSelected(null)
      setBrowseCues([])
    }
    if (open === id) {
      setOpen(null)
      setCues([])
    }
    toast.success("Removed from this browser's library. The shared database record is untouched.")
  }, [removeTarget, currentVideo, browseSelected, open])

  /* ---------- Transcripts ---------- */

  const fetchAllCues = useCallback(async (video: VideoRecord): Promise<Cue[]> => {
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
  }, [])

  const showTranscript = useCallback(
    async (video: VideoRecord) => {
      if (open === video.id) { setOpen(null); return }
      setTranscriptLoading(true)
      setOpen(video.id)
      try {
        setCues(await fetchAllCues(video))
      } catch (cause) {
        setOpen(null)
        toast.error(errorText(cause))
      } finally {
        setTranscriptLoading(false)
      }
    },
    [open, fetchAllCues],
  )

  const openBrowseVideo = useCallback(
    async (video: VideoRecord) => {
      if (
        browseSelected?.id === video.id &&
        browseCues.length >= (video.transcript?.cue_count ?? 0)
      )
        return
      setBrowseSelected(video)
      setBrowseLoading(true)
      setBrowseCues([])
      try {
        setBrowseCues(await fetchAllCues(video))
      } catch (cause) {
        toast.error(errorText(cause))
      } finally {
        setBrowseLoading(false)
      }
    },
    [browseSelected, browseCues, fetchAllCues],
  )

  const browseFilter = useDeferredValue(browseQuery.trim().toLowerCase())
  const browseResults = useMemo(
    () =>
      browseFilter
        ? videos.filter(
            (v) =>
              (v.title || '').toLowerCase().includes(browseFilter) ||
              (v.channel || '').toLowerCase().includes(browseFilter),
          )
        : videos,
    [videos, browseFilter],
  )

  // FIX: Stable handler refs for primitive setters passed to children,
  // so child components don't re-render when only an unrelated state changes.
  const handleSearchQueryChange = useCallback((value: string) => {
    setSearchQuery(value)
    setSearchMessage(null)
  }, [])

  const handleSearchSubmit = useCallback(() => {
    // Read from state via closure — always current because this is recreated
    // only when searchQuery/searchScope/currentVideo change.
    void executeSearch(searchQuery, searchScope, currentVideo?.id)
  }, [executeSearch, searchQuery, searchScope, currentVideo])

  const showResults =
    searching || Boolean(searchDuration) || Boolean(searchMessage) || searchResults.length > 0

  return (
    <>
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
        onSearchQueryChange={handleSearchQueryChange}
        searching={searching}
        onSearchSubmit={handleSearchSubmit}
        onSuggestionPick={onSuggestionPick}
        url={url}
        onUrlChange={setUrl}
        busy={busy}
        youtubePreview={youtubePreview}
        subtitleOptions={subtitleOptions}
        translatedSubtitleOptions={translatedSubtitleOptions}
        selectedSubtitle={selectedSubtitle}
        onSelectSubtitle={setSelectedSubtitle}
        onFetchMetadata={discoverSubtitles}
        onIndexVideo={submitYouTubeIndex}
        pipeline={pipeline}
        file={file}
        onFileChange={setFile}
        uploadTitle={uploadTitle}
        onUploadTitleChange={setUploadTitle}
        uploadURL={uploadURL}
        onUploadURLChange={setUploadURL}
        onUpload={submitUpload}
        videos={videos}
        browseQuery={browseQuery}
        onBrowseQueryChange={setBrowseQuery}
        browseResults={browseResults}
        browseSelected={browseSelected}
        onSelectBrowse={openBrowseVideo}
        browseCues={browseCues}
        browseLoading={browseLoading}
      />

      {showResults && (
        <section className="results-section" aria-live="polite">
          <div className="section-heading">
            <h2>
              <SearchIcon size={17} /> Relevant moments
            </h2>
            <span className="spacer" />
            {searchDuration && (
              <span className="search-metrics">
                <strong>{searchResults.length}</strong> matches{' '}
                <span className="metric-divider" /> resolved in{' '}
                <strong>{searchDuration.client} ms</strong>
              </span>
            )}
          </div>
          {searching && (
            <div className="result-loading-list">
              <Skeleton className="result-loading-card" />
              <Skeleton className="result-loading-card" />
            </div>
          )}
          {!searching && searchMessage && (
            <div className="empty-state">
              <SearchIcon size={20} />
              <p>{searchMessage}</p>
            </div>
          )}
          {!searching && searchResults.length > 0 && (
            <div style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
              {searchResults.map((result, index) => (
                <ResultCard
                  key={result.chunk_id}
                  result={result}
                  // FIX: Pass submittedQuery (the locked-in search term), NOT
                  // searchQuery (the live input). ResultCard highlights based on
                  // this prop — previously it re-highlighted on every keystroke.
                  query={submittedQuery}
                  video={videos.find((v) => v.id === result.video_id)}
                  index={index}
                />
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
          onToggle={showTranscript}
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
  )
}

function isNotFound(cause: unknown) {
  return cause instanceof ApiError && cause.status === 404
}