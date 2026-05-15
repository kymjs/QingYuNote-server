import { useEffect, useState } from 'react'
import { ApiError, fetchUsers } from '../api/client'
import type { AdminUser } from '../types'

function formatQingyuStatus(user: AdminUser): string {
  if (user.qingyu_is_lifetime) return '终身 VIP'
  if (user.qingyu_active) return user.qingyu_expires_at ? `至 ${user.qingyu_expires_at}` : '已开通'
  return '未开通'
}

function formatMoney(yuan: number): string {
  return `¥${yuan.toFixed(2)}`
}

export function UsersPage() {
  const [users, setUsers] = useState<AdminUser[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  useEffect(() => {
    let cancelled = false
    ;(async () => {
      try {
        const data = await fetchUsers()
        if (!cancelled) setUsers(data)
      } catch (err) {
        if (!cancelled) {
          setError(err instanceof ApiError ? '加载失败，请重新登录' : '网络错误')
        }
      } finally {
        if (!cancelled) setLoading(false)
      }
    })()
    return () => {
      cancelled = true
    }
  }, [])

  return (
    <div className="page">
      <header className="page-header">
        <h1>注册用户</h1>
        <p>共 {users.length} 位用户</p>
      </header>

      {error && <div className="alert error">{error}</div>}

      {loading ? (
        <p className="muted">加载中…</p>
      ) : (
        <div className="table-wrap">
          <table className="data-table">
            <thead>
              <tr>
                <th>ID</th>
                <th>账号</th>
                <th>手机号</th>
                <th>昵称</th>
                <th>头像</th>
                <th>轻羽云</th>
                <th>过期时间</th>
                <th>累计充值</th>
              </tr>
            </thead>
            <tbody>
              {users.map((user) => (
                <tr key={user.id}>
                  <td>{user.id}</td>
                  <td>{user.account}</td>
                  <td>{user.phone ?? '—'}</td>
                  <td>{user.nickname ?? '—'}</td>
                  <td>
                    {user.avatar_url ? (
                      <img src={user.avatar_url} alt="" className="avatar" />
                    ) : (
                      '—'
                    )}
                  </td>
                  <td>
                    <span className={user.qingyu_active ? 'tag ok' : 'tag'}>
                      {user.qingyu_active || user.qingyu_is_lifetime ? '已开通' : '未开通'}
                    </span>
                  </td>
                  <td>{formatQingyuStatus(user)}</td>
                  <td>{formatMoney(user.total_recharge_yuan)}</td>
                </tr>
              ))}
            </tbody>
          </table>
          {users.length === 0 && <p className="empty">暂无注册用户</p>}
        </div>
      )}
    </div>
  )
}
