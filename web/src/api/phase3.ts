import { keepPreviousData, queryOptions, useMutation, useQueryClient } from '@tanstack/react-query'
import { ApiError, api } from './client'
import type { Owner } from './compliance'

export interface RiskControl {
  id: string
  code: string
  name: string
  status: string
}
export interface Risk {
  id: string
  title: string
  description: string
  category: string
  likelihood: number
  impact: number
  score: number
  residual_likelihood?: number
  residual_impact?: number
  residual_score?: number
  level: 'low' | 'medium' | 'high' | 'critical'
  treatment: 'mitigate' | 'accept' | 'transfer' | 'avoid'
  treatment_plan: string
  status: 'suggested' | 'open' | 'in_treatment' | 'accepted' | 'closed' | 'dismissed'
  owner?: Owner
  source: 'manual' | 'ai' | 'test'
  test_id?: string
  ai_rationale: string
  review_cycle_days: number
  next_review_at?: string
  last_reviewed_at?: string
  review_overdue: boolean
  controls: RiskControl[]
  created_at: string
}
export interface Heatmap {
  inherent: number[][]
  residual: number[][]
}
export interface RiskAssessmentRow {
  id: string
  kind: string
  summary: string
  suggested: number
  model: string
  created_by: string
  created_at: string
}
export interface RiskInput {
  title?: string
  description?: string
  category?: string
  likelihood?: number
  impact?: number
  residual_likelihood?: number
  residual_impact?: number
  treatment?: string
  treatment_plan?: string
  status?: string
  owner_membership_id?: string
  review_cycle_days?: number
  control_ids?: string[]
}
export interface Vendor {
  id: string
  name: string
  domain: string
  description: string
  category: string
  status: 'discovered' | 'active' | 'offboarded' | 'dismissed'
  tier: 'critical' | 'high' | 'medium' | 'low' | 'unassessed'
  data_access: 'none' | 'internal' | 'confidential' | 'restricted'
  handles_pii: boolean
  owner?: Owner
  source: string
  discovery_ref: string
  discovery_users: number
  discovery_note: string
  recommended_tier: string
  risk_summary: string
  review_cycle_days: number
  next_review_at?: string
  last_reviewed_at?: string
  discovered_at?: string
  created_at: string
  review_overdue: boolean
  has_security_report: boolean
  has_dpa: boolean
  documents: number
  frameworks: string[]
}
export interface Citation {
  page: number
  section: string
  quote: string
}
export interface VendorAssessmentResult {
  document_type: string
  auditor: string
  opinion: string
  opinion_citation?: Citation | null
  period_start: string
  period_end: string
  summary: string
  controls_tested: { id: string; description: string; result: 'no_exceptions' | 'exceptions'; citation: Citation }[]
  exceptions: { control: string; description: string; management_response: string; severity: string; citation: Citation }[]
  cuecs: { description: string; citation: Citation }[]
  subservice_organizations: { description: string; citation: Citation }[]
  risk_summary: string
  recommended_tier: string
  recommended_tier_reason: string
}
export interface VendorDocument {
  id: string
  kind: string
  title: string
  filename: string
  content_type: string
  size_bytes: number
  sha256: string
  expires_at?: string
  uploaded_by: string
  created_at: string
}
export interface VendorQuestionnaire {
  id: string
  template_id: string
  recipient_email: string
  status: string
  templates: QuestionnaireTemplate[]
  answers: Record<string, string>
  sent_at: string
  submitted_at?: string
  expires_at: string
  link?: string
}
export interface QuestionnaireQuestion {
  id: string
  section: string
  type: 'yes_no' | 'text' | 'choice'
  options?: string[]
  question: string
}
export interface QuestionnaireTemplate {
  id: string
  title: string
  description: string
  frameworks: string[]
  questions: QuestionnaireQuestion[]
}
export interface QuestionnaireSummary {
  id: string
  title: string
  description: string
  frameworks: string[]
  question_count: number
  generated: boolean
}
export interface VendorAssessmentRow {
  id: string
  document_id?: string
  questionnaire_id?: string
  status: 'pending' | 'done' | 'error' | 'skipped'
  result?: VendorAssessmentResult
  model: string
  error: string
  created_at: string
  completed_at?: string
}
export interface VendorDetail extends Vendor {
  documents_list: VendorDocument[]
  questionnaires: VendorQuestionnaire[]
  assessments: VendorAssessmentRow[]
  questionnaire_templates: QuestionnaireSummary[]
  questionnaires_default: boolean
}
export interface PortalView {
  org_name: string
  vendor_name: string
  status: string
  expires_at: string
  templates: QuestionnaireTemplate[]
  answers: Record<string, string>
}
export interface VendorInput {
  name?: string
  domain?: string
  description?: string
  category?: string
  tier?: string
  data_access?: string
  handles_pii?: boolean
  owner_membership_id?: string
  status?: string
  frameworks?: string[]
  questionnaires?: string[]
}

type Items<T> = { items: T[] }

export const risksQuery = (status?: string) =>
  queryOptions({ queryKey: ['risks', status ?? ''], queryFn: () => api.get<Items<Risk>>(`/risks${status ? `?status=${status}` : ''}`), placeholderData: keepPreviousData, refetchInterval: 10_000 })
export const riskQuery = (id: string) => queryOptions({ queryKey: ['risk', id], queryFn: () => api.get<Risk>(`/risks/${id}`) })
export const heatmapQuery = () => queryOptions({ queryKey: ['risks', 'heatmap'], queryFn: () => api.get<Heatmap>('/risks/heatmap'), refetchInterval: 10_000 })
export const riskAssessmentsQuery = () => queryOptions({ queryKey: ['risk-assessments'], queryFn: () => api.get<Items<RiskAssessmentRow>>('/risk-assessments') })
export const vendorsQuery = (status?: string, framework?: string) => {
  const qs = new URLSearchParams()
  if (status) qs.set('status', status)
  if (framework) qs.set('framework', framework)
  const q = qs.toString()
  return queryOptions({ queryKey: ['vendors', status ?? '', framework ?? ''], queryFn: () => api.get<Items<Vendor>>(`/vendors${q ? `?${q}` : ''}`), placeholderData: keepPreviousData, refetchInterval: 10_000 })
}
export const questionnaireLibraryQuery = () => queryOptions({ queryKey: ['questionnaire-library'], queryFn: () => api.get<Items<QuestionnaireSummary>>('/questionnaire-library') })
export const vendorQuery = (id: string) =>
  queryOptions({
    queryKey: ['vendor', id],
    queryFn: () => api.get<VendorDetail>(`/vendors/${id}`),
    refetchInterval: (q) => (q.state.data?.assessments.some((a) => a.status === 'pending') ? 2_000 : 15_000),
  })
export const portalQuery = (token: string) => queryOptions({ queryKey: ['portal', token], queryFn: () => api.get<PortalView>(`/vendor-portal/${token}`), retry: false })

function useInv() {
  const qc = useQueryClient()
  return (...keys: string[]) => keys.forEach((k) => void qc.invalidateQueries({ queryKey: [k] }))
}

export function useCreateRisk() {
  const inv = useInv()
  return useMutation({ mutationFn: (i: RiskInput) => api.post<Risk>('/risks', i), onSuccess: () => inv('risks', 'control') })
}
export function useUpdateRisk(id: string) {
  const inv = useInv()
  return useMutation({ mutationFn: (i: RiskInput) => api.patch<Risk>(`/risks/${id}`, i), onSuccess: () => inv('risk', 'risks', 'control', 'activity') })
}
export function useRiskAction() {
  const inv = useInv()
  const done = () => inv('risk', 'risks', 'control', 'risk-assessments', 'activity')
  return {
    accept: useMutation({ mutationFn: (id: string) => api.post<Risk>(`/risks/${id}/accept`), onSuccess: done }),
    dismiss: useMutation({ mutationFn: (id: string) => api.post(`/risks/${id}/dismiss`), onSuccess: done }),
    reviewed: useMutation({ mutationFn: (id: string) => api.post<Risk>(`/risks/${id}/reviewed`), onSuccess: done }),
    aiAssess: useMutation({ mutationFn: () => api.post<{ items: Risk[]; summary: string }>('/risks/ai-assessment'), onSuccess: done }),
  }
}
export function useCreateVendor() {
  const inv = useInv()
  return useMutation({ mutationFn: (i: VendorInput) => api.post<VendorDetail>('/vendors', i), onSuccess: () => inv('vendors') })
}
export function useVendorAction(id: string) {
  const inv = useInv()
  const done = () => inv('vendor', 'vendors', 'activity')
  return {
    update: useMutation({ mutationFn: (i: VendorInput) => api.patch<VendorDetail>(`/vendors/${id}`, i), onSuccess: done }),
    triage: useMutation({ mutationFn: (add: boolean) => api.post<VendorDetail>(`/vendors/${id}/triage`, { add }), onSuccess: done }),
    restore: useMutation({ mutationFn: () => api.post<VendorDetail>(`/vendors/${id}/restore`, {}), onSuccess: done }),
    applyTier: useMutation({ mutationFn: () => api.post<VendorDetail>(`/vendors/${id}/apply-tier`), onSuccess: done }),
    reviewed: useMutation({ mutationFn: () => api.post<VendorDetail>(`/vendors/${id}/reviewed`), onSuccess: done }),
    sendQuestionnaire: useMutation({ mutationFn: (email: string) => api.post<VendorQuestionnaire>(`/vendors/${id}/questionnaires`, { recipient_email: email }), onSuccess: done }),
    assess: useMutation({ mutationFn: (docId: string) => api.post(`/vendors/${id}/documents/${docId}/assess`), onSuccess: done }),
    upload: useMutation({
      mutationFn: async (i: { file: File; kind: string; title: string; expires_at?: string }) => {
        const fd = new FormData()
        fd.set('file', i.file)
        fd.set('kind', i.kind)
        fd.set('title', i.title)
        if (i.expires_at) fd.set('expires_at', i.expires_at)
        const res = await fetch(`/api/v1/vendors/${id}/documents`, { method: 'POST', body: fd, credentials: 'same-origin' })
        const data = await res.json()
        if (!res.ok) throw new ApiError(res.status, data?.error?.code ?? 'error', data?.error?.message ?? 'Upload failed', data?.error?.fields ?? {})
        return data as VendorDetail
      },
      onSuccess: done,
    }),
  }
}
export function usePortalSave(token: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (i: { answers: Record<string, string>; submit: boolean }) => api.put<PortalView>(`/vendor-portal/${token}`, i),
    onSuccess: (v) => qc.setQueryData(['portal', token], v),
  })
}
