import { useState } from 'react'
import { Link, useParams } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import { Bot, X } from 'lucide-react'
import { toast } from 'sonner'
import { usePermissions } from '@/api/auth'
import { errorMessage } from '@/api/client'
import { controlsQuery } from '@/api/compliance'
import { membersQuery } from '@/api/org'
import { riskQuery, useRiskAction, useUpdateRisk, type Risk, type RiskInput } from '@/api/phase3'
import { FormField } from '@/components/layout/FormField'
import { PageHeader } from '@/components/layout/PageHeader'
import { ControlStatusBadge } from '@/components/status'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Select } from '@/components/ui/select'
import { Textarea } from '@/components/ui/textarea'
import { formatDate } from '@/lib/utils'
import { RiskStatusBadge, ScoreBadge } from './riskUi'

export function RiskDetailPage() {
  const { id } = useParams({ from: '/app/risks/$id' })
  const { data: r } = useQuery(riskQuery(id))
  if (!r) return <p className="text-sm text-muted-foreground">Loading…</p>
  return <RiskEditor key={r.id + r.status} r={r} />
}

function RiskEditor({ r }: { r: Risk }) {
  const { can } = usePermissions()
  const writable = can('risks:write')
  const members = useQuery({ ...membersQuery, enabled: writable })
  const controls = useQuery(controlsQuery({}))
  const update = useUpdateRisk(r.id)
  const act = useRiskAction()
  const [f, setF] = useState<RiskInput>({
    title: r.title, description: r.description, category: r.category, likelihood: r.likelihood, impact: r.impact,
    residual_likelihood: r.residual_likelihood, residual_impact: r.residual_impact, treatment: r.treatment, treatment_plan: r.treatment_plan,
    status: r.status === 'suggested' ? undefined : r.status, owner_membership_id: r.owner?.membership_id ?? '', review_cycle_days: r.review_cycle_days,
  })
  const save = () =>
    update.mutate(
      { ...f, status: r.status === 'suggested' ? undefined : f.status },
      { onSuccess: () => toast.success('Risk saved'), onError: (e) => toast.error(errorMessage(e)) },
    )
  const setControls = (ids: string[]) => update.mutate({ control_ids: ids }, { onSuccess: () => toast.success('Controls updated'), onError: (e) => toast.error(errorMessage(e)) })
  const linkable = (controls.data?.items ?? []).filter((c) => !r.controls.some((x) => x.id === c.id))
  const score = (f.likelihood ?? 1) * (f.impact ?? 1)
  const residual = f.residual_likelihood && f.residual_impact ? f.residual_likelihood * f.residual_impact : null
  return (
    <div className="space-y-6">
      <PageHeader
        title={r.title}
        description={`${r.category} · created ${formatDate(r.created_at)} · source: ${r.source}`}
        actions={
          <>
            <RiskStatusBadge status={r.status} />
            {writable && r.status === 'suggested' ? (
              <>
                <Button variant="outline" onClick={() => act.dismiss.mutate(r.id, { onSuccess: () => toast.success('Dismissed') })}>
                  Dismiss
                </Button>
                <Button onClick={() => act.accept.mutate(r.id, { onSuccess: () => toast.success('Added to the register') })} data-testid="risk-detail-accept">
                  Accept into register
                </Button>
              </>
            ) : writable && r.status !== 'closed' ? (
              <Button variant="outline" onClick={() => act.reviewed.mutate(r.id, { onSuccess: () => toast.success('Marked reviewed') })}>
                Mark reviewed
              </Button>
            ) : null}
          </>
        }
      />
      {r.ai_rationale ? (
        <Alert>
          <Bot className="h-4 w-4" />
          <AlertDescription data-testid="risk-rationale">{r.ai_rationale}</AlertDescription>
        </Alert>
      ) : null}
      <div className="grid gap-6 lg:grid-cols-[1fr_360px]">
        <Card>
          <CardHeader>
            <CardTitle>Assessment and treatment</CardTitle>
          </CardHeader>
          <CardContent className="space-y-4">
            <FormField label="Title" htmlFor="t">
              <Input id="t" value={f.title} onChange={(e) => setF({ ...f, title: e.target.value })} disabled={!writable} />
            </FormField>
            <FormField label="Description" htmlFor="d">
              <Textarea id="d" value={f.description} onChange={(e) => setF({ ...f, description: e.target.value })} disabled={!writable} />
            </FormField>
            <div className="grid gap-3 md:grid-cols-4">
              <Scale label="Likelihood" value={f.likelihood} onChange={(v) => setF({ ...f, likelihood: v })} disabled={!writable} testid="risk-edit-likelihood" />
              <Scale label="Impact" value={f.impact} onChange={(v) => setF({ ...f, impact: v })} disabled={!writable} testid="risk-edit-impact" />
              <Scale label="Residual likelihood" value={f.residual_likelihood} onChange={(v) => setF({ ...f, residual_likelihood: v })} disabled={!writable} optional testid="risk-edit-rl" />
              <Scale label="Residual impact" value={f.residual_impact} onChange={(v) => setF({ ...f, residual_impact: v })} disabled={!writable} optional testid="risk-edit-ri" />
            </div>
            <div className="flex items-center gap-4 text-sm">
              Inherent <ScoreBadge score={score} testid="risk-score" /> {residual ? <>Residual <ScoreBadge score={residual} /></> : null}
            </div>
            <div className="grid gap-3 md:grid-cols-3">
              <FormField label="Treatment" htmlFor="tr">
                <Select id="tr" value={f.treatment} onChange={(e) => setF({ ...f, treatment: e.target.value })} disabled={!writable}>
                  {['mitigate', 'accept', 'transfer', 'avoid'].map((t) => (
                    <option key={t}>{t}</option>
                  ))}
                </Select>
              </FormField>
              <FormField label="Status" htmlFor="st">
                <Select id="st" value={f.status ?? ''} onChange={(e) => setF({ ...f, status: e.target.value })} disabled={!writable || r.status === 'suggested'} data-testid="risk-edit-status">
                  {r.status === 'suggested' ? <option value="">suggested</option> : null}
                  {['open', 'in_treatment', 'accepted', 'closed'].map((t) => (
                    <option key={t} value={t}>
                      {t.replace('_', ' ')}
                    </option>
                  ))}
                </Select>
              </FormField>
              <FormField label="Review every (days)" htmlFor="rc">
                <Input id="rc" type="number" value={f.review_cycle_days} onChange={(e) => setF({ ...f, review_cycle_days: Number(e.target.value) })} disabled={!writable} />
              </FormField>
            </div>
            <FormField label="Treatment plan" htmlFor="tp">
              <Textarea id="tp" rows={4} value={f.treatment_plan} onChange={(e) => setF({ ...f, treatment_plan: e.target.value })} disabled={!writable} data-testid="risk-edit-plan" />
            </FormField>
            <FormField label="Owner" htmlFor="ow">
              <Select id="ow" value={f.owner_membership_id} onChange={(e) => setF({ ...f, owner_membership_id: e.target.value })} disabled={!writable}>
                <option value="">Unassigned</option>
                {members.data?.items.map((m) => (
                  <option key={m.id} value={m.id}>
                    {m.name}
                  </option>
                ))}
              </Select>
            </FormField>
            {writable ? (
              <Button loading={update.isPending} onClick={save} data-testid="risk-save">
                Save
              </Button>
            ) : null}
          </CardContent>
        </Card>
        <Card className="h-fit">
          <CardHeader>
            <CardTitle>Mitigating controls</CardTitle>
          </CardHeader>
          <CardContent className="space-y-2" data-testid="risk-controls">
            {r.controls.length === 0 ? <p className="text-sm text-muted-foreground">No controls linked.</p> : null}
            {r.controls.map((c) => (
              <div key={c.id} className="flex items-center justify-between gap-2 text-sm">
                <Link to="/app/controls/$id" params={{ id: c.id }} className="hover:underline">
                  <code className="mr-1 text-xs font-semibold">{c.code}</code>
                  {c.name}
                </Link>
                <span className="flex items-center gap-2">
                  <ControlStatusBadge status={c.status as 'passing'} />
                  {writable ? (
                    <button aria-label={`Unlink ${c.code}`} className="text-muted-foreground hover:text-destructive" onClick={() => setControls(r.controls.filter((x) => x.id !== c.id).map((x) => x.id))}>
                      <X className="h-4 w-4" />
                    </button>
                  ) : null}
                </span>
              </div>
            ))}
            {writable ? (
              <Select value="" onChange={(e) => e.target.value && setControls([...r.controls.map((x) => x.id), e.target.value])} data-testid="risk-link-control">
                <option value="">Link a control…</option>
                {linkable.map((c) => (
                  <option key={c.id} value={c.id}>
                    {c.code} · {c.name}
                  </option>
                ))}
              </Select>
            ) : null}
          </CardContent>
        </Card>
      </div>
    </div>
  )
}

function Scale({ label, value, onChange, disabled, optional, testid }: { label: string; value?: number; onChange: (v?: number) => void; disabled: boolean; optional?: boolean; testid: string }) {
  return (
    <FormField label={label} htmlFor={testid}>
      <Select id={testid} value={value ?? ''} onChange={(e) => onChange(e.target.value ? Number(e.target.value) : undefined)} disabled={disabled} data-testid={testid}>
        {optional ? <option value="">—</option> : null}
        {[1, 2, 3, 4, 5].map((n) => (
          <option key={n} value={n}>
            {n}
          </option>
        ))}
      </Select>
    </FormField>
  )
}
