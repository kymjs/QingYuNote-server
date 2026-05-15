import { NavLink, Outlet, useNavigate } from 'react-router-dom'
import { clearToken } from '../auth'

export function Layout() {
  const navigate = useNavigate()

  function handleLogout() {
    clearToken()
    navigate('/login', { replace: true })
  }

  return (
    <div className="app-shell">
      <aside className="sidebar">
        <div className="brand">
          <span className="brand-mark">羽</span>
          <div>
            <strong>轻羽云</strong>
            <span>管理后台</span>
          </div>
        </div>
        <nav className="nav">
          <NavLink to="/users" className={({ isActive }) => (isActive ? 'active' : '')}>
            用户列表
          </NavLink>
          <NavLink to="/redemption" className={({ isActive }) => (isActive ? 'active' : '')}>
            创建兑换码
          </NavLink>
        </nav>
        <button type="button" className="btn-ghost logout" onClick={handleLogout}>
          退出登录
        </button>
      </aside>
      <main className="main">
        <Outlet />
      </main>
    </div>
  )
}
