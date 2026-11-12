import type { Cue } from '../api/endpoints'
import { ApiError } from '../api/client'

export function formatMs(ms: number) {
  return `${Math.floor(ms / 60000)}:${String(Math.floor(ms / 1000) % 60).padStart(2, '0')}`
}

export function formatTranscript(cues: Cue[], format: 'srt' | 'vtt') {
  const body = cues
    .map((cue, index) => {
      const start = formatSubtitleTimestamp(cue.start_ms, format)
      const end = formatSubtitleTimestamp(cue.end_ms, format)
      return format === 'srt'
        ? `${index + 1}\n${start} --> ${end}\n${cue.text}`
        : `${start} --> ${end}\n${cue.text}`
    })
    .join('\n\n')
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

export function safeFilename(value: string) {
  return value.replace(/[^a-z0-9]+/gi, '-').replace(/^-|-$/g, '').toLowerCase() || 'transcript'
}

export function copyTextFallback(text: string) {
  const textarea = document.createElement('textarea')
  textarea.value = text
  textarea.setAttribute('readonly', '')
  textarea.style.position = 'fixed'
  textarea.style.opacity = '0'
  document.body.appendChild(textarea)
  textarea.select()
  textarea.setSelectionRange(0, textarea.value.length)
  let copied = false
  try {
    copied = document.execCommand('copy')
  } finally {
    textarea.remove()
  }
  return copied
}

export async function copyToClipboard(text: string): Promise<boolean> {
  if (navigator.clipboard?.writeText) {
    try {
      await navigator.clipboard.writeText(text)
      return true
    } catch {
      return copyTextFallback(text)
    }
  }
  return copyTextFallback(text)
}

export function downloadTextFile(filename: string, content: string, mime: string) {
  const blob = new Blob([content], { type: mime })
  const href = URL.createObjectURL(blob)
  const anchor = document.createElement('a')
  anchor.href = href
  anchor.download = filename
  anchor.click()
  URL.revokeObjectURL(href)
}

export function errorText(cause: unknown) {
  return cause instanceof ApiError ? cause.message : 'Something went wrong. Please try again.'
}

/* ---- Transcript excerpt & query highlighting ---- */

const QUERY_STOPWORDS = new Set([
  'about', 'after', 'again', 'also', 'been', 'being', 'could', 'does', 'from',
  'have', 'into', 'more', 'most', 'only', 'other', 'should', 'some', 'than',
  'that', 'their', 'there', 'these', 'they', 'this', 'those', 'through',
  'what', 'when', 'where', 'which', 'while', 'with', 'would', 'your',
])

// FIX: compile once, reuse everywhere — was being created inline per call
const WORD_NORMALIZE_RE = /[^\p{L}\p{N}']+/gu

function normalizeComparisonWord(word: string) {
  // Reset lastIndex since the regex has the 'g' flag and is shared
  WORD_NORMALIZE_RE.lastIndex = 0
  return word.toLocaleLowerCase().replace(WORD_NORMALIZE_RE, '')
}

// FIX: was O(n²) — terms.indexOf is a linear scan for every term.
// Use a Set for O(1) dedup.
export function meaningfulQueryTerms(query: string): string[] {
  const seen = new Set<string>()
  const result: string[] = []
  for (const raw of query.split(/\s+/)) {
    const term = normalizeComparisonWord(raw)
    if (term.length >= 2 && !QUERY_STOPWORDS.has(term) && !seen.has(term)) {
      seen.add(term)
      result.push(term)
      if (result.length === 8) break
    }
  }
  return result
}

// FIX: pre-normalize words once into a parallel array so collapseRepeatedPhrases
// never calls normalizeComparisonWord inside its inner loop.
// Before: for 500-word text, inner loop ran normalizeComparisonWord ~6000×,
// each firing the unicode regex. Now it runs exactly 500× total, up front.
function normalizeTranscriptText(text: string): string {
  const withoutMarkers = text.replace(/(?:^|\s)(?:>>\s*)+/g, ' ')
  const normalized = withoutMarkers
    .replace(/\s+/g, ' ')
    .replace(/\s+([,.;!?])/g, '$1')
    .trim()
  return collapseRepeatedPhrases(normalized)
}

function collapseRepeatedPhrases(text: string): string {
  const words = text.split(' ')
  const n = words.length

  // Normalize all words once up front — O(n) instead of O(n × phraseLength × 2)
  const normed = words.map(normalizeComparisonWord)

  const output: string[] = []
  let i = 0

  while (i < n) {
    let collapsed = false
    const maxLen = Math.min(12, Math.floor((n - i) / 2))

    for (let len = maxLen; len >= 3; len--) {
      // Compare pre-normalized slices directly — no .map() inside the loop
      let match = true
      for (let k = 0; k < len; k++) {
        // Both slots must be non-empty strings
        if (!normed[i + k] || normed[i + k] !== normed[i + len + k]) {
          match = false
          break
        }
      }
      if (match) {
        for (let k = 0; k < len; k++) output.push(words[i + k])
        i += len * 2
        collapsed = true
        break
      }
    }

    if (!collapsed) {
      output.push(words[i])
      i++
    }
  }

  return output.join(' ')
}

// FIX: Cache normalized transcript text by input string.
// normalizeTranscriptText is the most expensive function here — O(n²) in the
// worst case due to collapseRepeatedPhrases. With 10 result cards each holding
// potentially large transcript chunks, this was running 10× per render.
// Now it runs once per unique text string, ever, for the lifetime of the page.
// WeakMap won't work on strings, so we use a plain Map with a cap to avoid
// unbounded memory growth.
const normalizeCache = new Map<string, string>()
const NORMALIZE_CACHE_MAX = 200

function normalizeTranscriptTextCached(text: string): string {
  const cached = normalizeCache.get(text)
  if (cached !== undefined) return cached
  const result = normalizeTranscriptText(text)
  if (normalizeCache.size >= NORMALIZE_CACHE_MAX) {
    // Evict the oldest entry (Map preserves insertion order)
    normalizeCache.delete(normalizeCache.keys().next().value!)
  }
  normalizeCache.set(text, result)
  return result
}

export function buildTranscriptExcerpt(text: string, query: string): string {
  // FIX: use cached normalization — this is now a near-instant Map lookup
  // on repeat calls with the same text (which is every re-render)
  const normalized = normalizeTranscriptTextCached(text)
  const MAX = 320

  if (normalized.length <= MAX) return normalized

  const terms = meaningfulQueryTerms(query)
  const lowered = normalized.toLocaleLowerCase()

  // Find the earliest match position across all terms
  let matchIndex = -1
  for (const term of terms) {
    const idx = lowered.indexOf(term)
    if (idx >= 0 && (matchIndex < 0 || idx < matchIndex)) matchIndex = idx
  }
  if (matchIndex < 0) matchIndex = 0

  let start = sentenceStart(normalized, matchIndex)
  let end = sentenceEnd(normalized, matchIndex)

  while (start > 0) {
    const prev = sentenceStart(normalized, start - 1)
    if (end - prev > MAX) break
    start = prev
  }
  while (end < normalized.length) {
    const next = sentenceEnd(normalized, end)
    if (next - start > MAX) break
    end = next
  }

  if (end - start > MAX) {
    start = Math.max(0, matchIndex - Math.floor(MAX * 0.35))
    end = Math.min(normalized.length, start + MAX)
    while (start > 0 && normalized[start] !== ' ') start++
    while (end < normalized.length && normalized[end] !== ' ') end--
  }

  const excerpt = normalized.slice(start, end).trim()
  return `${start > 0 ? '… ' : ''}${excerpt}${end < normalized.length ? ' …' : ''}`
}

// FIX: combine the three lastIndexOf calls into one pass
function sentenceStart(text: string, index: number): number {
  let boundary = -1
  for (let i = index - 1; i >= 0; i--) {
    const c = text[i]
    if (c === '.' || c === '!' || c === '?') { boundary = i; break }
  }
  return boundary < 0 ? 0 : boundary + 1
}

// FIX: combine the three indexOf calls into one pass
function sentenceEnd(text: string, index: number): number {
  for (let i = index; i < text.length; i++) {
    const c = text[i]
    if (c === '.' || c === '!' || c === '?') return i + 1
  }
  return text.length
}

export function escapeRegExp(value: string) {
  return value.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
}