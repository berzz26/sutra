import type { HealthStatus } from './client'
import { apiClient } from './client'

export function getHealth(): Promise<HealthStatus> {
  return apiClient.get<HealthStatus>('/api/v1/health')
}
