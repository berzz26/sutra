import type { Cue } from '../api/endpoints'
import { ApiError } from '../api/client'

export function formatMs(ms: number) {
  return `${Math.floor(ms / 60000)}:${String(Math.floor(ms / 1000) % 60).padStart(2, '0')}`
}

export function formatTranscript(cues: Cue[], format: 'srt' | 'vtt') {
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
  try { copied = document.execCommand('copy') } finally { textarea.remove() }
  return copied
}

export async function copyToClipboard(text: string): Promise<boolean> {
  if (navigator.clipboard?.writeText) {
    try { await navigator.clipboard.writeText(text); return true } catch { return copyTextFallback(text) }
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

const QUERY_STOPWORDS = new Set(['about', 'after', 'again', 'also', 'been', 'being', 'could', 'does', 'from', 'have', 'into', 'more', 'most', 'only', 'other', 'should', 'some', 'than', 'that', 'their', 'there', 'these', 'they', 'this', 'those', 'through', 'what', 'when', 'where', 'which', 'while', 'with', 'would', 'your'])

function normalizeComparisonWord(word: string) {
  return word.toLocaleLowerCase().replace(/[^\p{L}\p{N}']+/gu, '')
}

export function meaningfulQueryTerms(query: string) {
  return query.split(/\s+/).map(normalizeComparisonWord).filter((term, index, terms) => term.length >= 2 && !QUERY_STOPWORDS.has(term) && terms.indexOf(term) === index).slice(0, 8)
}

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

export function buildTranscriptExcerpt(text: string, query: string) {
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

export function escapeRegExp(value: string) {
  return value.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
}
