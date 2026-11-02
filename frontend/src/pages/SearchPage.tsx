import { FormEvent, useEffect, useState } from 'react'
import { ApiError } from '../api/client'
import { Cue, deleteVideo, getTranscript, ingestYouTube, listSubtitles, listVideos, SubtitleOption, uploadTranscript, Video } from '../api/endpoints'

export function SearchPage() {
  const [videos, setVideos] = useState<Video[]>([])
  const [url, setUrl] = useState(''); const [language, setLanguage] = useState(''); const [title, setTitle] = useState(''); const [originalURL, setOriginalURL] = useState('')
  const [subtitleOptions, setSubtitleOptions] = useState<SubtitleOption[]>([]); const [selectedSubtitle, setSelectedSubtitle] = useState('')
  const [file, setFile] = useState<File | null>(null); const [message, setMessage] = useState<string | null>(null); const [busy, setBusy] = useState(false)
  const [open, setOpen] = useState<string | null>(null); const [cues, setCues] = useState<Cue[]>([])
  const refresh = () => listVideos().then((result) => setVideos(result.videos))
  useEffect(() => { refresh().catch(() => undefined) }, [])
  const errorText = (cause: unknown) => cause instanceof ApiError ? cause.message : String(cause)
  const discoverSubtitles = async () => { if (!url.trim()) { setMessage('Enter a YouTube URL first.'); return }; setBusy(true); setMessage(null); try { const result = await listSubtitles(url); setSubtitleOptions(result.subtitles); setSelectedSubtitle(''); setMessage(`Found ${result.subtitles.length} manual subtitle tracks.`) } catch (cause) { setMessage(errorText(cause)) } finally { setBusy(false) } }
  const submitYouTube = async (event: FormEvent) => { event.preventDefault(); setBusy(true); setMessage(null); try { const subtitle = subtitleOptions.find((option) => `${option.source}:${option.language}` === selectedSubtitle); await ingestYouTube(url, language, subtitle); setUrl(''); setSubtitleOptions([]); setSelectedSubtitle(''); setMessage('YouTube transcript ingested.'); await refresh() } catch (cause) { setMessage(errorText(cause)) } finally { setBusy(false) } }
  const submitUpload = async (event: FormEvent) => { event.preventDefault(); if (!file) { setMessage('Choose a .vtt or .srt file.'); return }; setBusy(true); setMessage(null); try { const data = new FormData(); data.append('file', file); if (title) data.append('title', title); if (originalURL) data.append('original_url', originalURL); if (language) data.append('language', language); await uploadTranscript(data); setFile(null); setTitle(''); setOriginalURL(''); setMessage('Transcript uploaded.'); await refresh() } catch (cause) { setMessage(errorText(cause)) } finally { setBusy(false) } }
  const showTranscript = async (video: Video) => { if (open === video.id) { setOpen(null); return }; try { const result = await getTranscript(video.id); setCues(result.cues); setOpen(video.id) } catch (cause) { setMessage(errorText(cause)) } }
  const remove = async (id: string) => { if (!window.confirm('Delete this video and transcript?')) return; try { await deleteVideo(id); await refresh() } catch (cause) { setMessage(errorText(cause)) } }
  return <section>
    <h1>Transcript ingestion</h1><p>Index videos that already have subtitles, or upload a transcript file.</p>
    <div className="forms">
      <form onSubmit={submitYouTube}><h2>YouTube URL</h2><input required value={url} onChange={(e) => { setUrl(e.target.value); setSubtitleOptions([]); setSelectedSubtitle('') }} placeholder="https://youtube.com/watch?v=…" /><button type="button" onClick={discoverSubtitles} disabled={busy}>List manual subtitles</button>{subtitleOptions.length > 0 && <select required value={selectedSubtitle} onChange={(e) => setSelectedSubtitle(e.target.value)}><option value="">Select a manual subtitle track</option>{subtitleOptions.map((option) => <option key={`${option.source}:${option.language}`} value={`${option.source}:${option.language}`}>{option.language}{option.name ? ` — ${option.name}` : ''} ({option.formats.join(', ')})</option>)}</select>}<input value={language} onChange={(e) => setLanguage(e.target.value)} placeholder="Language preference (optional)" /><button disabled={busy}>Fetch selected transcript</button></form>
      <form onSubmit={submitUpload}><h2>Upload transcript</h2><input type="file" accept=".vtt,.srt" onChange={(e) => setFile(e.target.files?.[0] ?? null)} /><input value={title} onChange={(e) => setTitle(e.target.value)} placeholder="Title (optional)" /><input value={originalURL} onChange={(e) => setOriginalURL(e.target.value)} placeholder="Original YouTube URL (optional)" /><button disabled={busy}>Upload transcript</button></form>
    </div>
    {message && <p className="message">{message}</p>}<h2>Video library</h2>
    {videos.length === 0 ? <p>No videos ingested yet.</p> : <div className="video-list">{videos.map((video) => <article key={video.id}><div><strong>{video.title || 'Untitled video'}</strong><span>{video.source_type} · {video.language || 'language unknown'} · {video.status} · {video.transcript?.cue_count ?? 0} cues</span></div><div><button onClick={() => showTranscript(video)}>{open === video.id ? 'Hide' : 'View'} transcript</button> <button onClick={() => remove(video.id)}>Delete</button></div>{open === video.id && <div className="cues">{cues.map((cue) => <p key={cue.id}><small>{formatMs(cue.start_ms)}</small>{cue.text}</p>)}</div>}</article>)}</div>}
  </section>
}
function formatMs(ms: number) { return `${Math.floor(ms / 60000)}:${String(Math.floor(ms / 1000) % 60).padStart(2, '0')}` }
