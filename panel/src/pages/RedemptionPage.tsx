import { type FormEvent, useState } from 'react'
import { ApiError, createRedemptionCodes } from '../api/client'
import type { RedemptionPlanId } from '../types'

const planOptions: { value: RedemptionPlanId; label: string }[] = [
  { value: 'monthly', label: '月卡' },
  { value: 'half_year', label: '半年卡' },
  { value: 'yearly', label: '年卡' },
  { value: 'lifetime_vip', label: '终身 VIP' },
]

const errorMessages: Record<string, string> = {
  redemption_issue_not_configured: '服务端未配置 REDEMPTION_ISSUE_SECRET',
  invalid_body: '请检查数量与类型',
  db_failed: '创建失败，请稍后重试',
}

export function RedemptionPage() {
  const [count, setCount] = useState(1)
  const [planId, setPlanId] = useState<RedemptionPlanId>('monthly')
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')
  const [codes, setCodes] = useState<string[]>([])

  async function handleSubmit(e: FormEvent) {
    e.preventDefault()
    setError('')
    setCodes([])
    setLoading(true)
    try {
      const result = await createRedemptionCodes(planId, count)
      setCodes(result.codes.map((c) => c.code))
    } catch (err) {
      if (err instanceof ApiError) {
        setError(errorMessages[err.code ?? ''] ?? '创建失败')
      } else {
        setError('网络错误')
      }
    } finally {
      setLoading(false)
    }
  }

  function copyAll() {
    void navigator.clipboard.writeText(codes.join('\n'))
  }

  return (
    <div className="page">
      <header className="page-header">
        <h1>创建兑换码</h1>
        <p>创建后将通过飞书 Webhook 发送通知（服务端已配置时）</p>
      </header>

      <form className="card form-card" onSubmit={handleSubmit}>
        <label>
          数量
          <input
            type="number"
            min={1}
            max={100}
            value={count}
            onChange={(e) => setCount(Number(e.target.value))}
            required
          />
        </label>
        <label>
          类型
          <select value={planId} onChange={(e) => setPlanId(e.target.value as RedemptionPlanId)}>
            {planOptions.map((opt) => (
              <option key={opt.value} value={opt.value}>
                {opt.label}
              </option>
            ))}
          </select>
        </label>
        {error && <div className="alert error">{error}</div>}
        <button type="submit" className="btn-primary" disabled={loading}>
          {loading ? '创建中…' : '创建兑换码'}
        </button>
      </form>

      {codes.length > 0 && (
        <div className="card result-card">
          <div className="result-header">
            <h2>已创建 {codes.length} 个兑换码</h2>
            <button type="button" className="btn-ghost" onClick={copyAll}>
              复制全部
            </button>
          </div>
          <ul className="code-list">
            {codes.map((code) => (
              <li key={code}>
                <code>{code}</code>
              </li>
            ))}
          </ul>
        </div>
      )}
    </div>
  )
}
