import type { ApiResponse } from './types'

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const headers = new Headers(init?.headers)
  if (!(init?.body instanceof FormData) && !headers.has('Content-Type')) headers.set('Content-Type', 'application/json')
  const res = await fetch(path, {
    ...init,
    headers
  })
  const body = (await res.json()) as ApiResponse<T>
  if (!res.ok || !body.success) {
    throw new Error(body.error || `Request failed: ${res.status}`)
  }
  return body.data as T
}

export const api = {
  get: <T>(path: string) => request<T>(path),
  post: <T>(path: string, data: unknown) => request<T>(path, { method: 'POST', body: JSON.stringify(data) }),
  put: <T>(path: string, data: unknown) => request<T>(path, { method: 'PUT', body: JSON.stringify(data) }),
  upload: <T>(path: string, data: FormData) => request<T>(path, { method: 'POST', body: data }),
  delete: <T>(path: string) => request<T>(path, { method: 'DELETE' })
}
