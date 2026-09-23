import { useCallback, useEffect, useRef, useState } from 'react'
import ReactMarkdown from 'react-markdown'
import remarkGfm from 'remark-gfm'
import {
  api,
  uploadMaterial,
  type MaterialDetail,
  type MaterialSummary,
  type Me,
} from '../api'
import { useToast } from '../components/Toast'
import type { Theme } from '../App'

interface Props {
  me: Me
  theme: Theme
  onToggleTheme: () => void
  onLogout: () => void
}

export default function MaterialsPage({ me, theme, onToggleTheme, onLogout }: Props) {
  const [items, setItems] = useState<MaterialSummary[]>([])
  const [query, setQuery] = useState('')
  const [view, setView] = useState<'list' | 'grid'>('list')
  const [detail, setDetail] = useState<MaterialDetail | null>(null)
  const [paletteOpen, setPaletteOpen] = useState(false)
  const [paletteQuery, setPaletteQuery] = useState('')
  const [progress, setProgress] = useState<number | null>(null)
  const fileInput = useRef<HTMLInputElement>(null)
  const toast = useToast()

  const refresh = useCallback(
    async (q: string) => {
      try {
        setItems(await api.listMaterials(q))
      } catch (err) {
        if (err instanceof Error && err.message !== '未登录') {
          toast(err.message, true)
        }
      }
    },
    [toast],
  )

  useEffect(() => {
    refresh('')
  }, [refresh])

  // ⌘/Ctrl+K 唤起命令面板（spec「界面体验」）。
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 'k') {
        e.preventDefault()
        setPaletteOpen((v) => !v)
        setPaletteQuery('')
      }
      if (e.key === 'Escape') {
        setPaletteOpen(false)
        setDetail(null)
      }
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [])

  async function openDetail(id: number) {
    try {
      setDetail(await api.materialDetail(id))
    } catch (err) {
      // 403/404 按服务端语义提示，不改变语义（spec R9）。
      const message = err instanceof Error ? err.message : '加载失败'
      if (message !== '未登录') toast(message, true)
    }
  }

  async function onPickFile(file: File | undefined) {
    if (!file) return
    setProgress(0)
    try {
      const result = await uploadMaterial(file, '', setProgress)
      toast(`已上传「${result.title}」`)
      await refresh(query)
    } catch (err) {
      if (err instanceof Error && err.message !== '未登录') toast(err.message, true)
    } finally {
      setProgress(null)
      if (fileInput.current) fileInput.current.value = ''
    }
  }

  const paletteItems = items.filter((m) =>
    m.title.toLowerCase().includes(paletteQuery.toLowerCase()),
  )

  return (
    <>
      <header className="topbar">
        <span className="logo">CampusClaw</span>
        <span className="who">
          {me.username} · {me.role === 'teacher' ? '教师' : '学生'} · {me.class_name}
        </span>
        <span className="spacer" />
        <button onClick={() => setPaletteOpen(true)}>⌘K 搜索</button>
        <button onClick={onToggleTheme}>{theme === 'light' ? '深色' : '浅色'}</button>
        <button onClick={onLogout}>登出</button>
      </header>

      <main className="container">
        <div className="toolbar">
          <input
            className="search"
            type="text"
            placeholder="搜索本班材料标题或正文…"
            value={query}
            onChange={(e) => {
              setQuery(e.target.value)
              refresh(e.target.value)
            }}
          />
          <button onClick={() => setView(view === 'list' ? 'grid' : 'list')}>
            {view === 'list' ? '网格视图' : '列表视图'}
          </button>
          {/* 上传入口依据 /api/me 的角色渲染；隐藏不构成权限，服务端仍会拒绝学生（spec R2/R9） */}
          {me.role === 'teacher' && (
            <>
              <button className="primary" onClick={() => fileInput.current?.click()}>
                上传材料
              </button>
              <input
                ref={fileInput}
                type="file"
                accept=".txt,.md"
                hidden
                onChange={(e) => onPickFile(e.target.files?.[0])}
              />
            </>
          )}
        </div>

        {progress !== null && (
          <div className="progress-track">
            <div className="progress-bar" style={{ width: `${progress}%` }} />
          </div>
        )}

        {items.length === 0 ? (
          <div className="empty">本班暂无材料</div>
        ) : (
          <div className={view === 'list' ? 'material-list' : 'material-grid'}>
            {items.map((m) => (
              <div key={m.id} className="material-item" onClick={() => openDetail(m.id)}>
                <div className="title">{m.title}</div>
                <div className="meta">
                  {m.class_name} · {new Date(m.created_at).toLocaleString()}
                </div>
              </div>
            ))}
          </div>
        )}
      </main>

      {detail && (
        <div className="modal-mask" onClick={() => setDetail(null)}>
          <div className="modal" onClick={(e) => e.stopPropagation()}>
            <div className="modal-head">
              <h2>{detail.title}</h2>
              <a href={`/api/materials/${detail.id}/file`} download>
                下载
              </a>
              <button onClick={() => setDetail(null)}>关闭</button>
            </div>
            <div className="meta" style={{ color: 'var(--text-dim)', marginBottom: 12 }}>
              {detail.class_name} · {new Date(detail.created_at).toLocaleString()}
            </div>
            {/* react-markdown 默认不渲染原始 HTML，材料中的脚本不会被执行（spec R9 渲染安全） */}
            <article className="markdown-body">
              <ReactMarkdown remarkPlugins={[remarkGfm]}>{detail.body}</ReactMarkdown>
            </article>
          </div>
        </div>
      )}

      {paletteOpen && (
        <div className="palette" onClick={() => setPaletteOpen(false)}>
          <div className="palette-box" onClick={(e) => e.stopPropagation()}>
            <input
              autoFocus
              type="text"
              placeholder="搜索本班材料并回车打开…（Esc 关闭）"
              value={paletteQuery}
              onChange={(e) => setPaletteQuery(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === 'Enter' && paletteItems[0]) {
                  setPaletteOpen(false)
                  openDetail(paletteItems[0].id)
                }
              }}
            />
            {paletteItems.slice(0, 8).map((m, i) => (
              <div
                key={m.id}
                className={i === 0 ? 'palette-item active' : 'palette-item'}
                onClick={() => {
                  setPaletteOpen(false)
                  openDetail(m.id)
                }}
              >
                <span>{m.title}</span>
                <span className="hint">{m.class_name}</span>
              </div>
            ))}
            {paletteItems.length === 0 && <div className="palette-item hint">无匹配材料</div>}
          </div>
        </div>
      )}
    </>
  )
}
