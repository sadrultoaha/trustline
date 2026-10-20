import { keepPreviousData, queryOptions, useMutation, useQueryClient } from '@tanstack/react-query'
import { api } from './client'
import type { Owner, TaskRow } from './compliance'

export interface PolicyRow {
  id: string
  template_id?: string
  slug: string
  title: string
  category: string
  summary: string
  controls: string[]
  owner?: Owner
  published_version?: number
  published_at?: string
  open_status: '' | 'draft' | 'in_review' | 'approved' | 'rejected'
  next_review_at?: string
  review_overdue: boolean
  audience: number
  accepted: number
  acceptance_rate: number
  review_cycle_days: number
}
export interface ConsistencyIssue {
  setting: string
  setting_value: string
  policy_value: string
  quote: string
  explanation: string
}
export interface PolicyVersion {
  id: string
  version: number
  status: 'draft' | 'in_review' | 'approved' | 'rejected' | 'published' | 'superseded'
  content_html?: string
  change_summary: string
  created_by: string
  created_at: string
  submitted_at?: string
  approver?: Owner
  decided_by: string
  decided_at?: string
  review_comment: string
  published_at?: string
  published_by: string
  ai_generated: boolean
  consistency?: { issues: ConsistencyIssue[] }
  consistency_checked_at?: string
}
export interface AcceptanceEntry {
  membership_id: string
  name: string
  email: string
  role: string
  accepted_at?: string
}
export interface PolicyDetail extends PolicyRow {
  versions: PolicyVersion[]
  acceptances: AcceptanceEntry[]
}
export interface LibraryItem {
  id: string
  title: string
  category: string
  summary: string
  controls: string[]
  review_cycle_days: number
  words: number
  policy_id?: string
}
export interface PersonRow {
  id: string
  email: string
  name: string
  department: string
  title: string
  manager_email: string
  status: 'active' | 'on_leave' | 'terminated'
  employment_type: string
  hire_date?: string
  termination_date?: string
  terminated_at?: string
  background_check: string
  has_account: boolean
  policies_accepted: boolean
  training_completed: boolean
  device_enrolled: boolean
  device_compliant: boolean
  onboarding_complete: boolean
  open_tasks: number
  access: Record<string, boolean>
}
export interface Device {
  id: string
  serial: string
  hostname: string
  owner_email: string
  owner_name: string
  platform: string
  os_version: string
  disk_encrypted: boolean
  screen_lock_minutes?: number
  antivirus_running: boolean
  antivirus_name: string
  agent_version: string
  last_seen_at: string
  compliant: boolean
  issues: string[]
}
export interface PersonDetail extends PersonRow {
  tasks: TaskRow[]
  devices: Device[]
}
export interface TrainingCourse {
  id: string
  title: string
  description: string
  passing_score: number
  valid_days: number
  modules: { title: string; body: string }[]
  questions: { id: string; question: string; options: string[] }[]
}
export interface TrainingStatus {
  course: TrainingCourse
  completed: boolean
  last_score?: number
  last_passed?: boolean
  completed_at?: string
  expires_at?: string
}
export interface QuizResult {
  score: number
  passed: boolean
  required: number
  feedback: { question_id: string; correct: boolean; correct_answer: number; explanation: string }[]
}
export interface Campaign {
  id: string
  name: string
  status: 'open' | 'completed' | 'cancelled'
  systems: string[]
  due_at: string
  created_by: string
  created_at: string
  completed_at?: string
  evidence_id?: string
  total: number
  decided: number
  revoked: number
  overdue: boolean
}
export interface ReviewItem {
  id: string
  campaign_id: string
  campaign_name: string
  integration_type: string
  account_id: string
  account_label: string
  email: string
  person_id?: string
  person_status: string
  entitlements: string[]
  is_admin: boolean
  reviewer?: Owner
  decision: 'pending' | 'approve' | 'revoke'
  comment: string
  decided_by: string
  decided_at?: string
  revocation_task_id?: string
}
export interface CampaignDetail extends Campaign {
  items: ReviewItem[]
}
export interface PendingPolicy {
  policy_id: string
  version_id: string
  title: string
  version: number
  published_at: string
  accepted_at?: string
}
export interface MyWork {
  policies: PendingPolicy[]
  training?: TrainingStatus
  devices: Device[]
  tasks: TaskRow[]
  review_items: ReviewItem[]
  pending: number
}
export interface DeviceToken {
  exists: boolean
  hint: string
  created_at?: string
  last_used_at?: string
  token?: string
}

type Items<T> = { items: T[] }

export const policiesQuery = () => queryOptions({ queryKey: ['policies'], queryFn: () => api.get<Items<PolicyRow>>('/policies'), refetchInterval: 10_000 })
export const policyLibraryQuery = () => queryOptions({ queryKey: ['policy-library'], queryFn: () => api.get<Items<LibraryItem>>('/policy-library') })
export const policyQuery = (id: string) => queryOptions({ queryKey: ['policy', id], queryFn: () => api.get<PolicyDetail>(`/policies/${id}`) })
export const peopleQuery = (status?: string) =>
  queryOptions({ queryKey: ['people', status ?? ''], queryFn: () => api.get<Items<PersonRow>>(`/people${status ? `?status=${status}` : ''}`), placeholderData: keepPreviousData, refetchInterval: 10_000 })
export const personQuery = (id: string) => queryOptions({ queryKey: ['person', id], queryFn: () => api.get<PersonDetail>(`/people/${id}`), refetchInterval: 5_000 })
export const devicesQuery = () => queryOptions({ queryKey: ['devices'], queryFn: () => api.get<Items<Device>>('/devices'), refetchInterval: 10_000 })
export const deviceTokenQuery = () => queryOptions({ queryKey: ['device-token'], queryFn: () => api.get<DeviceToken>('/device-token') })
export const trainingQuery = () => queryOptions({ queryKey: ['training'], queryFn: () => api.get<TrainingStatus>('/training') })
export const myWorkQuery = () => queryOptions({ queryKey: ['my-work'], queryFn: () => api.get<MyWork>('/me/work'), refetchInterval: 10_000 })
export const campaignsQuery = () => queryOptions({ queryKey: ['campaigns'], queryFn: () => api.get<Items<Campaign>>('/access-reviews'), refetchInterval: 10_000 })
export const campaignQuery = (id: string) => queryOptions({ queryKey: ['campaign', id], queryFn: () => api.get<CampaignDetail>(`/access-reviews/${id}`), refetchInterval: 10_000 })

function useInv() {
  const qc = useQueryClient()
  return (...keys: string[]) => keys.forEach((k) => void qc.invalidateQueries({ queryKey: [k] }))
}

export function useCreatePolicy() {
  const inv = useInv()
  return useMutation({ mutationFn: (input: { template_id?: string; title?: string }) => api.post<PolicyDetail>('/policies', input), onSuccess: () => inv('policies', 'policy-library') })
}
export function usePolicyAction(policyId: string) {
  const inv = useInv()
  const done = () => inv('policy', 'policies', 'my-work', 'controls', 'activity')
  return {
    newVersion: useMutation({ mutationFn: () => api.post<PolicyVersion>(`/policies/${policyId}/versions`), onSuccess: done }),
    markReviewed: useMutation({ mutationFn: () => api.post(`/policies/${policyId}/reviewed`), onSuccess: done }),
    setOwner: useMutation({ mutationFn: (owner: string) => api.patch(`/policies/${policyId}`, { owner_membership_id: owner }), onSuccess: done }),
    setControls: useMutation({ mutationFn: (codes: string[]) => api.patch(`/policies/${policyId}`, { control_codes: codes }), onSuccess: () => { done(); } }),
  }
}
export function useSaveDraft() {
  const inv = useInv()
  return useMutation({
    mutationFn: (i: { versionId: string; content_html: string; change_summary: string }) =>
      api.put(`/policy-versions/${i.versionId}`, { content_html: i.content_html, change_summary: i.change_summary }),
    onSuccess: () => inv('policy', 'policies'),
  })
}
export function useVersionAction() {
  const inv = useInv()
  const done = () => inv('policy', 'policies', 'my-work', 'controls', 'notifications', 'activity')
  return {
    submit: useMutation({ mutationFn: (i: { id: string; approver: string }) => api.post(`/policy-versions/${i.id}/submit`, { approver_membership_id: i.approver }), onSuccess: done }),
    decide: useMutation({ mutationFn: (i: { id: string; approve: boolean; comment: string }) => api.post(`/policy-versions/${i.id}/decide`, { approve: i.approve, comment: i.comment }), onSuccess: done }),
    publish: useMutation({ mutationFn: (id: string) => api.post(`/policy-versions/${id}/publish`), onSuccess: done }),
    aiDraft: useMutation({ mutationFn: (i: { id: string; instructions: string }) => api.post<PolicyVersion>(`/policy-versions/${i.id}/ai-draft`, { instructions: i.instructions }), onSuccess: done }),
    consistency: useMutation({ mutationFn: (id: string) => api.post<{ issues: ConsistencyIssue[] }>(`/policy-versions/${id}/consistency`), onSuccess: done }),
    acknowledge: useMutation({ mutationFn: (id: string) => api.post(`/policy-versions/${id}/acknowledge`), onSuccess: done }),
  }
}
export function useAttestBackground(personId: string) {
  const inv = useInv()
  return useMutation({ mutationFn: (status: string) => api.post(`/people/${personId}/background-check`, { status }), onSuccess: () => inv('person', 'people') })
}
export function useCompleteTask() {
  const inv = useInv()
  return useMutation({ mutationFn: (i: { id: string; note: string }) => api.post(`/tasks/${i.id}/complete`, { note: i.note }), onSuccess: () => inv('person', 'people', 'tasks') })
}
export function useRotateDeviceToken() {
  const inv = useInv()
  return useMutation({ mutationFn: () => api.post<DeviceToken>('/device-token'), onSuccess: () => inv('device-token') })
}
export function useDeleteDevice() {
  const inv = useInv()
  return useMutation({ mutationFn: (id: string) => api.delete(`/devices/${id}`), onSuccess: () => inv('devices', 'people') })
}
export function useSubmitQuiz() {
  const inv = useInv()
  return useMutation({ mutationFn: (answers: Record<string, number>) => api.post<QuizResult>('/training/submit', { answers }), onSuccess: () => inv('training', 'my-work') })
}
export function useCreateCampaign() {
  const inv = useInv()
  return useMutation({ mutationFn: (i: { name: string; systems: string[]; due_days: number }) => api.post<CampaignDetail>('/access-reviews', i), onSuccess: () => inv('campaigns', 'my-work') })
}
export function useDecideItem() {
  const inv = useInv()
  return useMutation({
    mutationFn: (i: { id: string; decision: 'approve' | 'revoke'; comment?: string }) => api.post<ReviewItem>(`/access-review-items/${i.id}/decision`, { decision: i.decision, comment: i.comment ?? '' }),
    onSuccess: () => inv('campaign', 'campaigns', 'my-work', 'tasks'),
  })
}
export function useCompleteCampaign() {
  const inv = useInv()
  return useMutation({ mutationFn: (id: string) => api.post<CampaignDetail>(`/access-reviews/${id}/complete`), onSuccess: () => inv('campaign', 'campaigns', 'controls') })
}
