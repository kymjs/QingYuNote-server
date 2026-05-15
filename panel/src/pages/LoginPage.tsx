import { type FormEvent, useState } from 'react'
import { Navigate, useNavigate } from 'react-router-dom'
import { ApiError, login } from '../api/client'
import { isLoggedIn, setToken } from '../auth'
import { adminCredentials } from '../config'

const errorMessages: Record<string, string> = {
  invalid_credentials: '用户名或密码错误',
  admin_not_configured: '服务端未配置管理后台凭据',
  ip_banned: '该 IP 因连续登录失败已被永久封禁',
}

export function LoginPage() {
  const navigate = useNavigate()
  const [username, setUsername] = useState(adminCredentials.username)
  const [password, setPassword] = useState(adminCredentials.password)
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)

  if (isLoggedIn()) {
    return <Navigate to="/users" replace />
  }

  async function handleSubmit(e: FormEvent) {
    e.preventDefault()
    setError('')
    setLoading(true)
    try {
      const token = await login(username.trim(), password)
      setToken(token)
      navigate('/users', { replace: true })
    } catch (err) {
      if (err instanceof ApiError) {
        setError(errorMessages[err.code ?? ''] ?? '登录失败，请稍后重试')
      } else {
        setError('网络错误，请检查 API 地址')
      }
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="login-page">
      <form className="login-card" onSubmit={handleSubmit}>
        <div className="login-header">
          <span className="brand-mark lg">羽</span>
          <h1>轻羽云管理后台</h1>
          <p>请使用项目配置的账号登录</p>
        </div>
        {error && <div className="alert error">{error}</div>}
        <label>
          账号
          <input
            type="text"
            value={username}
            onChange={(e) => setUsername(e.target.value)}
            autoComplete="username"
            required
          />
        </label>
        <label>
          密码
          <input
            type="password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            autoComplete="current-password"
            required
          />
        </label>
        <button type="submit" className="btn-primary" disabled={loading}>
          {loading ? '登录中…' : '登录'}
        </button>
      </form>
    </div>
  )
}
