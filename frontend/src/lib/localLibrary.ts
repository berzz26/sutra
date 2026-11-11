/**
 * Browser-local video library.
 *
 * The database is shared, but every browser keeps its own list of video IDs
 * in localStorage. Only IDs are stored here — never transcripts, metadata,
 * or any other large data. All localStorage access for the library lives in
 * this module so components never touch storage directly.
 *
 * Note: this is browser-level organization, not user accounts or real
 * authorization. Clearing browser storage removes the local references but
 * leaves the indexed database content untouched.
 */

const STORAGE_KEY = 'sutra.videoLibrary.v1'

type Listener = (ids: string[]) => void

const listeners = new Set<Listener>()

function readIDs(): string[] {
  try {
    const raw = localStorage.getItem(STORAGE_KEY)
    if (!raw) return []
    const parsed: unknown = JSON.parse(raw)
    if (!Array.isArray(parsed)) return []
    // Only string entries survive; anything malformed is dropped so a
    // corrupted key can never break the app.
    return Array.from(new Set(parsed.filter((id): id is string => typeof id === 'string' && id.length > 0)))
  } catch {
    return []
  }
}

function writeIDs(ids: string[]) {
  try { localStorage.setItem(STORAGE_KEY, JSON.stringify(ids)) } catch {
    // Storage can be unavailable (private mode, quota); the in-memory state
    // still works for the current page load.
  }
  for (const listener of listeners) listener(ids)
}

function normalizeID(id: string) {
  return typeof id === 'string' ? id.trim() : ''
}

/** All locally saved video IDs (order: newest first). */
export function getLocalLibraryIDs(): string[] {
  return readIDs()
}

/** Adds a video ID (deduplicated, newest first) and returns the new list. */
export function addVideoToLibrary(id: string): string[] {
  const trimmed = normalizeID(id)
  if (!trimmed) return readIDs()
  const existing = readIDs()
  if (existing.includes(trimmed)) return existing
  const next = [trimmed, ...existing]
  writeIDs(next)
  return next
}

/** Removes a video ID (the database record is untouched) and returns the new list. */
export function removeVideoFromLibrary(id: string): string[] {
  const next = readIDs().filter((existing) => existing !== normalizeID(id))
  writeIDs(next)
  return next
}

/** Whether a video ID is saved in this browser's library. */
export function hasVideoInLibrary(id: string): boolean {
  return readIDs().includes(normalizeID(id))
}

/** Clears every locally saved reference. The database is unaffected. */
export function clearLocalLibrary(): void {
  writeIDs([])
}

/**
 * Subscribes to library changes (from this tab or another tab/storage event)
 * so component state can stay in sync with localStorage.
 */
export function subscribeLocalLibrary(listener: Listener): () => void {
  listeners.add(listener)
  const onStorage = (event: StorageEvent) => {
    if (event.key === STORAGE_KEY) listener(readIDs())
  }
  window.addEventListener('storage', onStorage)
  return () => {
    listeners.delete(listener)
    window.removeEventListener('storage', onStorage)
  }
}
