import { keepPreviousData, queryOptions, useMutation, useQueryClient } from '@tanstack/react-query'
import { ApiError, api } from './client'

export type ControlStatus = 'passing' | 'failing' | 'needs_evidence'
export type TestStatus = 'pending' | 'pass' | 'fail' | 'error' | 'na'
export type Severity = 'low' | 'medium' | 'high' | 'critical'

export interface Readiness {
  requirements_total: number
  requirements_met: number
  controls_total: number
  controls_passing: number
  controls_failing: number
  percent: number
}
export interface FrameworkSummary {
  id: string
  name: string
  version: string
  publisher: string
  description: string
  adopted: boolean
  adopted_at?: string
  readiness: Readiness
}
export interface RequirementControl {
  id?: string
  code: string
  name: string
  status: ControlStatus | 'not_in_scope'
}
export interface RequirementView {
  code: string
  title: string
  description: string
  met: boolean
  controls: RequirementControl[]
}
export interface SectionView {
  name: string
  met: number
  total: number
  requirements: RequirementView[]
}
export interface FrameworkDetail extends FrameworkSummary {
  sections: SectionView[]
}
export interface Owner {
  membership_id: string
  name: string
  email: string
}
export interface ControlRow {
  id: string
  code: string
  name: string
  domain: string
  description: string
  evidence_mode: 'automated' | 'manual' | 'hybrid'
  status: ControlStatus
  status_reason: string
  status_changed_at: string
  owner?: Owner
  frameworks: string[]
  tests_total: number
  tests_failing: number
  evidence_valid: number
}
export interface Remediation {
  summary: string
  steps: string[]
  iac_hint?: string
}
export interface TestRow {
  id: string
  catalog_id?: string
  control_id: string
  control_code: string
  control_name: string
  integration_type: string
  name: string
  description: string
  severity: Severity
  rule: Rule
  remediation: Remediation
  custom: boolean
  enabled: boolean
  last_status: TestStatus
  last_run_at?: string
  last_run_id?: string
  last_summary: string
}
export interface Failure {
  id: string
  reason: string
}
export interface TestRun {
  id: string
  test_id: string
  snapshot_id?: string
  status: 'pass' | 'fail' | 'error'
  evaluated: number
  failing: Failure[]
  summary: string
  error: string
  trigger: string
  created_at: string
}
export interface TestDetail extends TestRow {
  runs: TestRun[]
}
export interface RunDetail extends TestRun {
  test_name: string
  resource: string
  resources: Record<string, unknown>[]
  snapshot_taken_at?: string
  snapshot_sha256?: string
}
export interface Evidence {
  id: string
  control_id: string
  title: string
  description: string
  filename: string
  content_type: string
  size_bytes: number
  sha256: string
  collected_at: string
  expires_at?: string
  expired: boolean
  uploaded_by: string
  ai_status: 'pending' | 'done' | 'skipped' | 'error'
  ai_verdict?: 'pass' | 'fail'
  ai_confidence?: number
  ai_reasoning: string
  ai_missing: string[]
  ai_model: string
  override_verdict?: 'pass' | 'fail'
  override_reason: string
  override_by: string
  override_at?: string
  verdict: 'pass' | 'fail' | 'pending'
  created_at: string
}
export interface TaskRow {
  id: string
  kind: string
  title: string
  description: string
  status: 'open' | 'done' | 'dismissed'
  priority: Severity
  control_id?: string
  control_code: string
  test_id?: string
  assignee?: Owner
  steps: string[]
  terraform: string
  ai_status: 'none' | 'pending' | 'done' | 'skipped' | 'error'
  ai_model: string
  ai_notes: string
  resolution: string
  due_at?: string
  completed_at?: string
  created_at: string
  overdue: boolean
  person_id?: string
  person_name: string
  item_key: string
  meta: Record<string, unknown>
}
export interface MappedRequirement {
  framework_id: string
  framework_name: string
  adopted: boolean
  code: string
  title: string
}
export interface ControlDetail extends ControlRow {
  evidence_hint: string
  evidence_ttl_days: number
  requirements: MappedRequirement[]
  tests: TestRow[]
  evidence: Evidence[]
  open_tasks: TaskRow[]
  policies: LinkedPolicy[]
  risks: { id: string; title: string; score: number; status: string; level: string }[]
}
export interface LinkedPolicy {
  id: string
  slug: string
  title: string
  published_version?: number
  open_status: string
  acceptance_rate: number
}
export interface CredentialField {
  key: string
  label: string
  placeholder?: string
  secret: boolean
  required: boolean
  help?: string
}
export interface ScenarioInfo {
  key: string
  label: string
  description: string
  per_person: boolean
}
export interface SchemaField {
  path: string
  type: string
  label: string
}
export interface ResourceType {
  type: string
  label: string
  fields: SchemaField[]
}
export interface IntegrationType {
  type: string
  name: string
  category: string
  description: string
  resources: ResourceType[]
  credentials: CredentialField[]
  live_ready: boolean
  scenarios: ScenarioInfo[]
  test_count: number
  connected_integration_id?: string
}
export interface Integration {
  id: string
  type: string
  name: string
  mode: 'sandbox' | 'live'
  credentials_hint: Record<string, string>
  enabled: boolean
  status: 'pending' | 'syncing' | 'connected' | 'error'
  last_sync_at?: string
  last_error: string
  resource_count: number
  created_at: string
  tests_total: number
  tests_failing: number
  active_scenarios: string[]
}
export interface Snapshot {
  id: string
  connector: string
  taken_at: string
  resource_count: number
  sha256: string
  resources: Record<string, Record<string, unknown>[]>
}
export interface Notification {
  id: string
  kind: string
  title: string
  body: string
  link: string
  read_at?: string
  created_at: string
}
export interface Dashboard {
  controls: { total: number; passing: number; failing: number; needs_evidence: number }
  tests: { total: number; passing: number; failing: number }
  open_tasks: number
  overdue_tasks: number
  integrations: number
  frameworks: FrameworkSummary[]
  failing_controls: ControlRow[]
}

// Rule format (mirrors internal/testengine).
export interface Condition {
  field: string
  op: string
  value?: unknown
  match?: 'any'
}
export type Expr = { and: Expr[] } | { or: Expr[] } | { not: Expr } | Condition
export interface Rule {
  resource: string
  quantifier: 'all' | 'any' | 'none' | 'count'
  min?: number
  max?: number
  where?: Expr
  assert: Expr
  require_resources?: boolean
}
export interface RuleResult {
  status: 'pass' | 'fail'
  evaluated: number
  matched: number
  failing: Failure[]
  summary: string
}
export interface TestSchema {
  connectors: { type: string; name: string; resources: ResourceType[] }[]
  operators: { op: string; label: string }[]
  quantifiers: string[]
}

type Items<T> = { items: T[] }

const qs = (params: Record<string, string | undefined>) => {
  const s = new URLSearchParams()
  for (const [k, v] of Object.entries(params)) if (v) s.set(k, v)
  const out = s.toString()
  return out ? `?${out}` : ''
}

export const dashboardQuery = () => queryOptions({ queryKey: ['dashboard'], queryFn: () => api.get<Dashboard>('/dashboard'), refetchInterval: 10_000 })
export const frameworksQuery = () => queryOptions({ queryKey: ['frameworks'], queryFn: () => api.get<Items<FrameworkSummary>>('/frameworks'), refetchInterval: 10_000 })
export const frameworkQuery = (id: string) => queryOptions({ queryKey: ['frameworks', id], queryFn: () => api.get<FrameworkDetail>(`/frameworks/${id}`) })
export const controlsQuery = (f: { framework?: string; status?: string; q?: string }) =>
  queryOptions({ queryKey: ['controls', f], queryFn: () => api.get<Items<ControlRow>>(`/controls${qs(f)}`), placeholderData: keepPreviousData, refetchInterval: 10_000 })
export const controlQuery = (id: string) => queryOptions({ queryKey: ['control', id], queryFn: () => api.get<ControlDetail>(`/controls/${id}`), refetchInterval: 5_000 })
export const integrationTypesQuery = () => queryOptions({ queryKey: ['integration-types'], queryFn: () => api.get<Items<IntegrationType>>('/integration-types') })
export const integrationsQuery = () => queryOptions({ queryKey: ['integrations'], queryFn: () => api.get<Items<Integration>>('/integrations'), refetchInterval: 5_000 })
export const integrationQuery = (id: string) => queryOptions({ queryKey: ['integration', id], queryFn: () => api.get<Integration>(`/integrations/${id}`), refetchInterval: 3_000 })
export const snapshotQuery = (id: string) => queryOptions({ queryKey: ['integration', id, 'snapshot'], queryFn: () => api.get<Snapshot>(`/integrations/${id}/snapshot`), retry: false, refetchInterval: 10_000 })
export const testsQuery = (f: { status?: string; integration?: string; control?: string; q?: string; include_disabled?: string }) =>
  queryOptions({ queryKey: ['tests', f], queryFn: () => api.get<Items<TestRow>>(`/tests${qs(f)}`), placeholderData: keepPreviousData, refetchInterval: 10_000 })
export const testQuery = (id: string) => queryOptions({ queryKey: ['test', id], queryFn: () => api.get<TestDetail>(`/tests/${id}`), refetchInterval: 5_000 })
export const runQuery = (id: string) => queryOptions({ queryKey: ['run', id], queryFn: () => api.get<RunDetail>(`/test-runs/${id}`) })
export const testSchemaQuery = () => queryOptions({ queryKey: ['test-schema'], queryFn: () => api.get<TestSchema>('/test-schema'), staleTime: Infinity })
export const tasksQuery = (f: { status?: string; mine?: string; control?: string }) =>
  queryOptions({ queryKey: ['tasks', f], queryFn: () => api.get<Items<TaskRow>>(`/tasks${qs(f)}`), placeholderData: keepPreviousData, refetchInterval: 10_000 })
export const taskQuery = (id: string) =>
  queryOptions({
    queryKey: ['task', id],
    queryFn: () => api.get<TaskRow>(`/tasks/${id}`),
    refetchInterval: (q) => (q.state.data?.ai_status === 'pending' ? 2_000 : 15_000),
  })
export const notificationsQuery = () =>
  queryOptions({ queryKey: ['notifications'], queryFn: () => api.get<{ items: Notification[]; unread: number }>('/notifications'), refetchInterval: 10_000 })

function useInvalidate() {
  const qc = useQueryClient()
  return (...keys: string[]) => keys.forEach((k) => void qc.invalidateQueries({ queryKey: [k] }))
}

export function useAdoptFramework() {
  const inv = useInvalidate()
  return useMutation({ mutationFn: (id: string) => api.post<FrameworkDetail>(`/frameworks/${id}/adopt`), onSuccess: () => inv('frameworks', 'controls', 'dashboard', 'activity') })
}
export function useRemoveFramework() {
  const inv = useInvalidate()
  return useMutation({ mutationFn: (id: string) => api.delete(`/frameworks/${id}`), onSuccess: () => inv('frameworks', 'controls', 'dashboard', 'activity') })
}
export function useSetControlOwner(id: string) {
  const inv = useInvalidate()
  return useMutation({
    mutationFn: (owner: string) => api.patch<ControlDetail>(`/controls/${id}`, { owner_membership_id: owner }),
    onSuccess: () => inv('control', 'controls', 'activity'),
  })
}
export function useUploadEvidence(controlId: string) {
  const inv = useInvalidate()
  return useMutation({
    mutationFn: async (input: { file: File; title: string; description: string; expires_at?: string }) => {
      const fd = new FormData()
      fd.set('file', input.file)
      fd.set('title', input.title)
      fd.set('description', input.description)
      if (input.expires_at) fd.set('expires_at', input.expires_at)
      const res = await fetch(`/api/v1/controls/${controlId}/evidence`, { method: 'POST', body: fd, credentials: 'same-origin' })
      const data = await res.json()
      if (!res.ok) {
        throw new ApiError(res.status, data?.error?.code ?? 'error', data?.error?.message ?? 'Upload failed', data?.error?.fields ?? {})
      }
      return data as Evidence
    },
    onSuccess: () => inv('control', 'controls', 'dashboard', 'activity'),
  })
}
export function useReviewEvidence() {
  const inv = useInvalidate()
  return useMutation({
    mutationFn: (input: { id: string; verdict: 'pass' | 'fail'; reason: string }) => api.post<Evidence>(`/evidence/${input.id}/review`, { verdict: input.verdict, reason: input.reason }),
    onSuccess: () => inv('control', 'controls', 'dashboard', 'frameworks', 'activity'),
  })
}
export function useDeleteEvidence() {
  const inv = useInvalidate()
  return useMutation({ mutationFn: (id: string) => api.delete(`/evidence/${id}`), onSuccess: () => inv('control', 'controls', 'dashboard', 'activity') })
}
export function useConnectIntegration() {
  const inv = useInvalidate()
  return useMutation({
    mutationFn: (input: { type: string; name?: string; mode: string; credentials: Record<string, string> }) => api.post<Integration>('/integrations', input),
    onSuccess: () => inv('integrations', 'integration-types', 'tests', 'dashboard', 'activity'),
  })
}
export function useDisconnectIntegration() {
  const inv = useInvalidate()
  return useMutation({ mutationFn: (id: string) => api.delete(`/integrations/${id}`), onSuccess: () => inv('integrations', 'integration-types', 'tests', 'controls', 'dashboard', 'activity') })
}
export function useSyncIntegration() {
  const inv = useInvalidate()
  return useMutation({ mutationFn: (id: string) => api.post(`/integrations/${id}/sync`), onSuccess: () => inv('integration', 'integrations') })
}
export function useSetScenario(id: string) {
  const inv = useInvalidate()
  return useMutation({
    mutationFn: (input: { key: string; active: boolean }) => api.post<Integration>(`/integrations/${id}/sandbox/scenarios`, input),
    onSuccess: () => inv('integration', 'integrations', 'activity'),
  })
}
export function useRunTest() {
  const inv = useInvalidate()
  return useMutation({ mutationFn: (id: string) => api.post<TestRow>(`/tests/${id}/run`), onSuccess: () => inv('test', 'tests', 'control', 'controls', 'tasks', 'dashboard') })
}
export function useSetTestEnabled() {
  const inv = useInvalidate()
  return useMutation({
    mutationFn: (input: { id: string; enabled: boolean }) => api.patch<TestRow>(`/tests/${input.id}`, { enabled: input.enabled }),
    onSuccess: () => inv('test', 'tests', 'control', 'controls', 'dashboard'),
  })
}
export function useCreateTest() {
  const inv = useInvalidate()
  return useMutation({
    mutationFn: (input: { name: string; description: string; control_id: string; integration_type: string; severity: string; rule: Rule; remediation: { summary: string; steps: string[] } }) =>
      api.post<TestRow>('/tests', input),
    onSuccess: () => inv('tests', 'controls', 'activity'),
  })
}
export function usePreviewRule() {
  return useMutation({ mutationFn: (input: { integration_type: string; rule: Rule }) => api.post<RuleResult>('/tests/preview', input) })
}
export function useUpdateTask() {
  const inv = useInvalidate()
  return useMutation({
    mutationFn: (input: { id: string; status?: string; resolution?: string; assignee_membership_id?: string }) => {
      const { id, ...body } = input
      return api.patch<TaskRow>(`/tasks/${id}`, body)
    },
    onSuccess: () => inv('task', 'tasks', 'dashboard', 'control', 'activity'),
  })
}
export function useRegenerateTask() {
  const inv = useInvalidate()
  return useMutation({ mutationFn: (id: string) => api.post(`/tasks/${id}/regenerate`), onSuccess: () => inv('task', 'tasks') })
}
export function useReadNotifications() {
  const inv = useInvalidate()
  return useMutation({ mutationFn: (ids: string[]) => api.post('/notifications/read', { ids }), onSuccess: () => inv('notifications') })
}

export function useLinkPolicy(controlId: string) {
  const inv = useInvalidate()
  return useMutation({
    mutationFn: (input: { policy_id: string; linked: boolean }) => api.post<ControlDetail>(`/controls/${controlId}/policies`, input),
    onSuccess: () => inv('control', 'policy', 'policies', 'activity'),
  })
}
