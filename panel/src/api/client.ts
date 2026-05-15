import { apiBaseUrl } from '../config'
import { clearToken, getToken } from '../auth'
import type { AdminUser, RedemptionCodeResult, RedemptionPlanId } from '../types'

class ApiError extends Error {
  status: number
  code?: string

  constructor(status: number, code?: string, message?: string) {
    super(message ?? code ?? `HTTP ${status}`)
    this.status = status
    this.code = code
  }
}

async function request<T>(path: string, init: RequestInit = {}): Promise<T> {
  const headers = new Headers(init.headers)
  if (!headers.has('Content-Type') && init.body) {
    headers.set('Content-Type', 'application/json')
  }
  const token = getToken()
  if (token) {
    headers.set('Authorization', `Bearer ${token}`)
  }

  const res = await fetch(`${apiBaseUrl}${path}`, { ...init, headers })
  const text = await res.text()
  let data: Record<string, unknown> = {}
  if (text) {
    try {
      data = JSON.parse(text) as Record<string, unknown>
    } catch {
      data = {}
    }
  }

  if (!res.ok) {
    if (res.status === 401) {
      clearToken()
    }
    const code = typeof data.error === 'string' ? data.error : undefined
    throw new ApiError(res.status, code)
  }
  return data as T
}

export async function login(username: string, password: string): Promise<string> {
  const data = await request<{ access_token: string }>('/api/v1/admin/auth/login', {
    method: 'POST',
    body: JSON.stringify({ username, password }),
  })
  return data.access_token
}

export async function fetchUsers(): Promise<AdminUser[]> {
  const data = await request<{ users: AdminUser[] }>('/api/v1/admin/users')
  return data.users
}

export async function createRedemptionCodes(
  planId: RedemptionPlanId,
  count: number,
): Promise<RedemptionCodeResult> {
  return request<RedemptionCodeResult>('/api/v1/admin/redemption-codes', {
    method: 'POST',
    body: JSON.stringify({ plan_id: planId, count }),
  })
}

export { ApiError }
