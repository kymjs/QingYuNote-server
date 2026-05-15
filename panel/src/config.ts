/** 管理后台登录凭据（构建时注入，见 .env.example） */
export const adminCredentials = {
  username: import.meta.env.VITE_ADMIN_USERNAME ?? 'admin',
  password: import.meta.env.VITE_ADMIN_PASSWORD ?? 'change-me',
}

/** Note API 根地址；开发环境留空走 Vite 代理 */
export const apiBaseUrl = (import.meta.env.VITE_API_BASE_URL ?? '').replace(/\/$/, '')
