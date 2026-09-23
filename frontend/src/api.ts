// API 封装：同源 /api；任何 401 都会清空本地身份并回登录页（spec R1）。
// 前端不存储角色（无 localStorage），身份一律以 GET /api/me 为准。

export interface Me {
  id: number
  username: string
  role: 'teacher' | 'student'
  class_id: number
  class_name: string
}

export interface MaterialSummary {
  id: number
  title: string
  class_name: string
  created_at: string
}

export interface MaterialDetail extends MaterialSummary {
  body: string
}

let onUnauthorized: () => void = () => {}
export function setUnauthorizedHandler(fn: () => void) {
  onUnauthorized = fn
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(path, { credentials: 'same-origin', ...init })
  if (res.status === 401) {
    onUnauthorized()
    throw new Error('未登录')
  }
  const data = await res.json().catch(() => ({}))
  if (!res.ok) {
    const message = (data as { error?: string }).error ?? `请求失败（${res.status}）`
    const err = new Error(message) as Error & { status?: number }
    err.status = res.status
    throw err
  }
  return data as T
}

export const api = {
  login: (username: string, password: string) =>
    request<{ ok: boolean }>('/api/login', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ username, password }),
    }),
  logout: () => request<{ ok: boolean }>('/api/logout', { method: 'POST' }),
  me: () => request<Me>('/api/me'),
  listMaterials: (q: string) =>
    request<MaterialSummary[]>(`/api/materials${q ? `?q=${encodeURIComponent(q)}` : ''}`),
  materialDetail: (id: number) => request<MaterialDetail>(`/api/materials/${id}`),
}

// 上传走 XHR 以获得进度事件（spec「界面体验」上传进度）。
export function uploadMaterial(
  file: File,
  title: string,
  onProgress: (percent: number) => void,
): Promise<{ id: number; title: string }> {
  return new Promise((resolve, reject) => {
    const form = new FormData()
    form.append('file', file)
    if (title) form.append('title', title)
    const xhr = new XMLHttpRequest()
    xhr.open('POST', '/api/materials')
    xhr.withCredentials = true
    xhr.upload.onprogress = (e) => {
      if (e.lengthComputable) onProgress(Math.round((e.loaded / e.total) * 100))
    }
    xhr.onload = () => {
      if (xhr.status === 401) {
        onUnauthorized()
        reject(new Error('未登录'))
        return
      }
      let data: { id?: number; title?: string; error?: string } = {}
      try {
        data = JSON.parse(xhr.responseText)
      } catch {
        /* 忽略非 JSON 响应 */
      }
      if (xhr.status >= 200 && xhr.status < 300) {
        resolve({ id: data.id ?? 0, title: data.title ?? file.name })
      } else {
        const err = new Error(data.error ?? `上传失败（${xhr.status}）`) as Error & { status?: number }
        err.status = xhr.status
        reject(err)
      }
    }
    xhr.onerror = () => reject(new Error('网络错误'))
    xhr.send(form)
  })
}
