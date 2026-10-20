export type Role = 'admin' | 'editor' | 'auditor' | 'employee'

export const ROLES: { value: Role; label: string; description: string }[] = [
  { value: 'admin', label: 'Admin', description: 'Full access, including members and billing' },
  { value: 'editor', label: 'Editor', description: 'Manage controls, policies, risks and vendors' },
  { value: 'auditor', label: 'Auditor', description: 'Read-only access to everything, can request evidence' },
  { value: 'employee', label: 'Employee', description: 'Accept policies, complete training and tasks' },
]

export interface OrgSettings {
  access_review_days: number
  password_min_length: number
  policy_acceptance_threshold: number
  ai_enabled: boolean
  timezone: string
  slack_webhook_url: string
  email_notifications: boolean
}

export interface Org {
  id: string
  name: string
  slug: string
  settings: OrgSettings
  created_at: string
  updated_at: string
}

export interface OrgWithRole extends Org {
  role: Role
}

export interface User {
  id: string
  email: string
  name: string
  avatar_url?: string
  has_password: boolean
  created_at: string
  last_login_at?: string | null
}

export interface Me {
  user: User
  org?: Org
  role?: Role
  permissions: string[]
  orgs: OrgWithRole[]
}

export interface Member {
  id: string
  org_id: string
  user_id: string
  role: Role
  email: string
  name: string
  avatar_url?: string
  last_login_at?: string | null
  created_at: string
}

export interface Invite {
  id: string
  org_id: string
  email: string
  role: Role
  invited_by?: string
  expires_at: string
  created_at: string
}

export interface InvitePreview {
  email: string
  role: Role
  org_name: string
  expired: boolean
}

export interface ActivityEntry {
  id: string
  org_id: string
  actor_type: 'user' | 'system' | 'api_key' | 'agent'
  actor_id?: string
  actor_label: string
  action: string
  target_type: string
  target_id?: string
  target_label?: string
  metadata: Record<string, unknown>
  ip?: string
  created_at: string
}

export interface Page<T> {
  items: T[]
  next_cursor?: string
}
