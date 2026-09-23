import { useCallback, useEffect, useState } from 'react'
import { api, setUnauthorizedHandler, type Me } from './api'
import LoginPage from './pages/Login'
import MaterialsPage from './pages/Materials'
import { ToastProvider } from './components/Toast'

export type Theme = 'light' | 'dark'

export default function App() {
  // 身份只来自 /api/me；本地不持久化角色（spec R9 刷新后身份恢复）。
  const [me, setMe] = useState<Me | null>(null)
  const [loading, setLoading] = useState(true)
  const [theme, setTheme] = useState<Theme>(
    () => (localStorage.getItem('theme') as Theme) || 'light',
  )

  useEffect(() => {
    document.documentElement.dataset.theme = theme
    localStorage.setItem('theme', theme)
  }, [theme])

  const handleUnauthorized = useCallback(() => setMe(null), [])
  useEffect(() => {
    setUnauthorizedHandler(handleUnauthorized)
  }, [handleUnauthorized])

  useEffect(() => {
    api
      .me()
      .then(setMe)
      .catch(() => setMe(null))
      .finally(() => setLoading(false))
  }, [])

  if (loading) return null

  return (
    <ToastProvider>
      {me ? (
        <MaterialsPage
          me={me}
          theme={theme}
          onToggleTheme={() => setTheme((t) => (t === 'light' ? 'dark' : 'light'))}
          onLogout={async () => {
            await api.logout().catch(() => {})
            setMe(null)
          }}
        />
      ) : (
        <LoginPage onSuccess={setMe} />
      )}
    </ToastProvider>
  )
}
