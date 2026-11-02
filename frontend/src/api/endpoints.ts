import type { HealthStatus } from './client'
import { apiClient } from './client'

export function getHealth(): Promise<HealthStatus> {
  return apiClient.get<HealthStatus>('/api/v1/health')
}

export interface Transcript { id: string; video_id: string; extraction_method: string; original_format: string; language?: string; cue_count: number }
export interface Video { id: string; source_type: 'youtube' | 'upload'; source_ref: string; title?: string; original_url?: string; duration_ms?: number; channel?: string; language?: string; status: string; created_at: string; transcript?: Transcript }
export interface Cue { id: string; video_id: string; seq: number; start_ms: number; end_ms: number; text: string }
export interface SubtitleOption { language: string; name?: string; source: 'manual' | 'automatic'; formats: string[] }

export function listVideos(): Promise<{ videos: Video[] }> { return apiClient.get('/api/v1/videos') }
export function listSubtitles(url: string): Promise<{ video_id: string; title: string; subtitles: SubtitleOption[] }> { return apiClient.get(`/api/v1/videos/subtitles?url=${encodeURIComponent(url)}`) }
export function ingestYouTube(url: string, language: string, subtitle?: SubtitleOption): Promise<Video> {
  return apiClient.post('/api/v1/videos', {
    url,
    language: language || undefined,
    subtitle_key: subtitle?.language,
    subtitle_source: subtitle?.source,
  })
}
export function uploadTranscript(data: FormData): Promise<Video> { return apiClient.postForm('/api/v1/transcripts', data) }
export function getTranscript(id: string): Promise<{ cues: Cue[] }> { return apiClient.get(`/api/v1/videos/${id}/transcript`) }
export function deleteVideo(id: string): Promise<void> { return apiClient.delete(`/api/v1/videos/${id}`) }
