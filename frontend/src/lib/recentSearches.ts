/**
 * Recent searches live in localStorage — there is no backend table for
 * them, so the browser keeps the history on the user's own machine,
 * consistent with Sutra's local-first design.
 */
import type { SearchResult, Video } from '../api/endpoints'

export interface RecentSearch {
  id: string
  query: string
  scope: 'video' | 'library'
  videoId?: string
  videoTitle?: string
  thumbnailUrl?: string
  durationMs?: number
  sourceType?: 'youtube' | 'upload'
  channel?: string
  at: number
}

const STORAGE_KEY = 'sutra-recent-searches'
const MAX_ENTRIES = 12

export function loadRecentSearches(): RecentSearch[] {
  try {
    const raw = localStorage.getItem(STORAGE_KEY)
    if (!raw) return []
    const parsed: unknown = JSON.parse(raw)
    if (!Array.isArray(parsed)) return []
    return parsed.filter((entry): entry is RecentSearch => Boolean(entry) && typeof (entry as RecentSearch).query === 'string')
  } catch {
    return []
  }
}

function persist(entries: RecentSearch[]) {
  try { localStorage.setItem(STORAGE_KEY, JSON.stringify(entries.slice(0, MAX_ENTRIES))) } catch {
    // Storage may be unavailable (private mode); recent searches are best-effort.
  }
}

/**
 * Record a completed search. For a video-scoped search the focused video
 * is shown on the card; for a library search the card points at the video
 * behind the top result (when its record is still in the library).
 */
export function rememberSearch(query: string, scope: 'video' | 'library', video: Video | null, topResult: SearchResult | null, videos: Video[]) {
  const trimmed = query.trim()
  if (!trimmed) return
  const entryVideo = scope === 'video' && video
    ? video
    : videos.find((candidate) => candidate.id === topResult?.video_id) ?? null
  const entry: RecentSearch = {
    id: `${Date.now()}-${Math.random().toString(36).slice(2, 8)}`,
    query: trimmed,
    scope: scope === 'video' && video ? 'video' : 'library',
    videoId: entryVideo?.id,
    videoTitle: entryVideo?.title ?? topResult?.video_title,
    thumbnailUrl: entryVideo?.thumbnail_url,
    durationMs: entryVideo?.duration_ms,
    sourceType: entryVideo?.source_type,
    channel: entryVideo?.channel,
    at: Date.now(),
  }
  const existing = loadRecentSearches()
  const deduped = existing.filter((item) => !(item.query === entry.query && (item.videoId ?? '') === (entry.videoId ?? '')))
  persist([entry, ...deduped])
}
