export interface AdminUser {
  id: number
  account: string
  phone: string | null
  nickname: string | null
  avatar_url: string | null
  qingyu_active: boolean
  qingyu_expires_at?: string
  qingyu_is_lifetime: boolean
  total_recharge_yuan: number
}

export type RedemptionPlanId = 'monthly' | 'half_year' | 'yearly' | 'lifetime_vip'

export interface RedemptionCodeResult {
  plan_id: RedemptionPlanId
  codes: { code: string }[]
}
