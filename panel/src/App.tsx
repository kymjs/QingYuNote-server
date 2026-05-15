import { Navigate, Route, Routes } from 'react-router-dom'
import { Layout } from './components/Layout'
import { isLoggedIn } from './auth'
import { LoginPage } from './pages/LoginPage'
import { RedemptionPage } from './pages/RedemptionPage'
import { UsersPage } from './pages/UsersPage'

function RequireAuth({ children }: { children: React.ReactNode }) {
  if (!isLoggedIn()) {
    return <Navigate to="/login" replace />
  }
  return <>{children}</>
}

export default function App() {
  return (
    <Routes>
      <Route path="/login" element={<LoginPage />} />
      <Route
        path="/"
        element={
          <RequireAuth>
            <Layout />
          </RequireAuth>
        }
      >
        <Route index element={<Navigate to="/users" replace />} />
        <Route path="users" element={<UsersPage />} />
        <Route path="redemption" element={<RedemptionPage />} />
      </Route>
      <Route path="*" element={<Navigate to="/users" replace />} />
    </Routes>
  )
}
