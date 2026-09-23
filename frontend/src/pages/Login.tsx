import { useState, type FormEvent } from 'react'
import { api, type Me } from '../api'
import { useToast } from '../components/Toast'

export default function LoginPage({ onSuccess }: { onSuccess: (me: Me) => void }) {
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)
  const toast = useToast()

  async function submit(e: FormEvent) {
    e.preventDefault()
    setBusy(true)
    setError('')
    try {
      const me = await api.login(username, password)
      toast(`欢迎，${me.username}（${me.class_name}）`)
      onSuccess(me)
    } catch (err) {
      // 服务端对账号不存在/口令错误/限流返回同形文案，前端原样展示（spec R1）。
      const message = err instanceof Error ? err.message : '登录失败'
      setError(message)
      toast(message, true)
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="login-card">
      <h1>CampusClaw</h1>
      <p className="sub">面向中小学的教研智能体 · 迭代 1 知识库底座</p>
      <form onSubmit={submit}>
        <label htmlFor="username">账号</label>
        <input
          id="username"
          type="text"
          autoComplete="username"
          value={username}
          onChange={(e) => setUsername(e.target.value)}
          required
        />
        <label htmlFor="password">密码</label>
        <input
          id="password"
          type="password"
          autoComplete="current-password"
          value={password}
          onChange={(e) => setPassword(e.target.value)}
          required
        />
        {error && <div className="form-error">{error}</div>}
        <button className="primary" type="submit" disabled={busy}>
          {busy ? '登录中…' : '登录'}
        </button>
      </form>
    </div>
  )
}
