/**
 * Result object of a readiness/liveness probe.
 */
export interface HealthStatus {
  status: string
}

/**
 * Fetch-based HTTP client. All backend communication goes through this
 * module so request handling (base URL, errors, envelope) is in one place.
 */
export class ApiError extends Error {
  readonly status: number

  constructor(status: number, message: string) {
    super(message)
    this.name = 'ApiError'
    this.status = status
  }
}

export class ApiClient {
  private readonly baseUrl: string

  constructor(baseUrl?: string) {
    this.baseUrl = (baseUrl ?? import.meta.env.VITE_API_BASE_URL ?? '').replace(/\/$/, '')
  }

  get<T>(path: string): Promise<T> {
    return this.request<T>(path, { method: 'GET' })
  }

  post<T>(path: string, body?: unknown): Promise<T> {
    return this.request<T>(path, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: body === undefined ? undefined : JSON.stringify(body),
    })
  }

  postForm<T>(path: string, body: FormData): Promise<T> {
    return this.request<T>(path, { method: 'POST', body })
  }

  delete<T>(path: string): Promise<T> {
    return this.request<T>(path, { method: 'DELETE' })
  }

  private async request<T>(path: string, init: RequestInit): Promise<T> {
    let response: Response
    try {
      response = await fetch(`${this.baseUrl}${path}`, init)
    } catch (cause) {
      throw new ApiError(0, `Network request failed: ${String(cause)}`)
    }

    if (!response.ok) {
      const detail = await this.readError(response)
      throw new ApiError(response.status, detail)
    }

    if (response.status === 204) {
      return undefined as T
    }
    return (await response.json()) as T
  }

  private async readError(response: Response): Promise<string> {
    try {
      const body = (await response.json()) as { error?: string; code?: string }
      if (body && typeof body.error === 'string') {
        return body.code ? `${body.code}: ${body.error}` : body.error
      }
    } catch {
      // fall through to generic message
    }
    return response.statusText || `Request failed with status ${response.status}`
  }
}

export const apiClient = new ApiClient()
