import { useState } from 'react'
import { Link, useNavigate } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import { Plus, Sparkles } from 'lucide-react'
import { toast } from 'sonner'
import { usePermissions } from '@/api/auth'
import { errorMessage, isApiError } from '@/api/client'
import { heatmapQuery, riskAssessmentsQuery, risksQuery, useCreateRisk, useRiskAction, type Risk } from '@/api/phase3'
import { FormField } from '@/components/layout/FormField'
import { PageHeader } from '@/components/layout/PageHeader'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Select } from '@/components/ui/select'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { Textarea } from '@/components/ui/textarea'
import { formatDate, timeAgo } from '@/lib/utils'
import { HeatmapGrid, RiskStatusBadge, ScoreBadge } from './riskUi'

export function RisksPage() {
  const [tab, setTab] = useState('register')
  const [cell, setCell] = useState<[number, number] | null>(null)
  const { data } = useQuery(risksQuery(tab === 'register' ? 'register' : 'suggested'))
  const heat = useQuery(heatmapQuery())
  const assessments = useQuery(riskAssessmentsQuery())
  const suggested = useQuery(risksQuery('suggested'))
  const { can } = usePermissions()
  const act = useRiskAction()
  const [newOpen, setNewOpen] = useState(false)
  const rows = (data?.items ?? []).filter((r) => !cell || (r.likelihood === cell[0] && r.impact === cell[1]))
  const last = assessments.data?.items[0]
  return (
    <div className="space-y-6">
      <PageHeader
        title="Risks"
        description="The risk register: scoring, treatment, owners and the controls that mitigate each risk."
        actions={
          can('risks:write') ? (
            <>
              <Button
                variant="outline"
                loading={act.aiAssess.isPending}
                data-testid="risk-ai-assess"
                onClick={() =>
                  act.aiAssess.mutate(undefined, {
                    onSuccess: (r) => {
                      toast.success(`${r.items.length} risk(s) suggested`)
                      setTab('suggested')
                    },
                    onError: (e) => toast.error(errorMessage(e)),
                  })
                }
              >
                <Sparkles /> Complete with AI
              </Button>
              <Button onClick={() => setNewOpen(true)} data-testid="risk-new">
                <Plus /> New risk
              </Button>
            </>
          ) : null
        }
      />
      <div className="grid gap-6 lg:grid-cols-[auto_1fr]">
        <Card>
          <CardHeader>
            <CardTitle>Inherent risk</CardTitle>
            <CardDescription>Likelihood × impact. Click a cell to filter.</CardDescription>
          </CardHeader>
          <CardContent>
            {heat.data ? <HeatmapGrid grid={heat.data.inherent} selected={cell} onSelect={(l, i) => setCell(cell && cell[0] === l && cell[1] === i ? null : [l, i])} /> : null}
          </CardContent>
        </Card>
        <Card>
          <CardHeader>
            <CardTitle>Risk assessment</CardTitle>
            <CardDescription>{last ? `Last ${last.kind === 'ai' ? 'AI-assisted ' : ''}assessment ${timeAgo(last.created_at)} by ${last.created_by || 'system'}` : 'No assessment recorded yet.'}</CardDescription>
          </CardHeader>
          <CardContent className="space-y-2 text-sm">
            {last ? <p data-testid="risk-last-assessment">{last.summary}</p> : <p className="text-muted-foreground">Use Complete with AI to draft an assessment from your failing tests, systems and vendors.</p>}
            <p className="text-muted-foreground">{suggested.data?.items.length ?? 0} suggested risk(s) waiting for triage.</p>
          </CardContent>
        </Card>
      </div>
      <div className="flex items-center gap-3">
        <Tabs value={tab} onValueChange={setTab}>
          <TabsList>
            <TabsTrigger value="register">Register</TabsTrigger>
            <TabsTrigger value="suggested" data-testid="risks-tab-suggested">
              Suggested ({suggested.data?.items.length ?? 0})
            </TabsTrigger>
          </TabsList>
        </Tabs>
        {cell ? (
          <Button size="sm" variant="ghost" onClick={() => setCell(null)}>
            Clear filter (L{cell[0]} × I{cell[1]})
          </Button>
        ) : null}
      </div>
      <Card>
        <CardContent className="p-0">
          <RiskTable risks={rows} suggested={tab === 'suggested'} />
        </CardContent>
      </Card>
      {newOpen ? <NewRiskDialog onClose={() => setNewOpen(false)} /> : null}
    </div>
  )
}

function RiskTable({ risks, suggested }: { risks: Risk[]; suggested: boolean }) {
  const act = useRiskAction()
  const { can } = usePermissions()
  return (
    <Table data-testid={suggested ? 'risks-suggested' : 'risks-table'}>
      <TableHeader>
        <TableRow>
          <TableHead>Risk</TableHead>
          <TableHead>Score</TableHead>
          <TableHead>{suggested ? 'Why' : 'Residual'}</TableHead>
          <TableHead>Controls</TableHead>
          <TableHead>{suggested ? '' : 'Owner'}</TableHead>
          <TableHead>{suggested ? '' : 'Status'}</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {risks.map((r) => (
          <TableRow key={r.id} data-testid={`risk-row-${r.id}`}>
            <TableCell className="max-w-md">
              <Link to="/app/risks/$id" params={{ id: r.id }} className="font-medium hover:underline">
                {r.title}
              </Link>
              <div className="text-xs text-muted-foreground">
                {r.category} · {r.treatment}
                {r.review_overdue ? <Badge variant="destructive" className="ml-2">review overdue</Badge> : r.next_review_at ? ` · review ${formatDate(r.next_review_at)}` : ''}
              </div>
            </TableCell>
            <TableCell>
              <ScoreBadge score={r.score} />
            </TableCell>
            <TableCell className="max-w-xs text-xs">{suggested ? <span className="text-muted-foreground">{r.ai_rationale}</span> : r.residual_score != null ? <ScoreBadge score={r.residual_score} /> : '—'}</TableCell>
            <TableCell>
              <div className="flex flex-wrap gap-1">
                {r.controls.map((c) => (
                  <Badge key={c.id} variant="outline" className="font-normal">
                    {c.code}
                  </Badge>
                ))}
              </div>
            </TableCell>
            <TableCell className="text-sm">
              {suggested ? (
                can('risks:write') ? (
                  <div className="flex gap-1">
                    <Button size="sm" data-testid="risk-accept" onClick={() => act.accept.mutate(r.id, { onSuccess: () => toast.success('Added to the register') })}>
                      Accept
                    </Button>
                    <Button size="sm" variant="ghost" data-testid="risk-dismiss" onClick={() => act.dismiss.mutate(r.id, { onSuccess: () => toast.success('Dismissed') })}>
                      Dismiss
                    </Button>
                  </div>
                ) : null
              ) : (
                (r.owner?.name ?? <span className="text-muted-foreground">Unassigned</span>)
              )}
            </TableCell>
            <TableCell>{suggested ? null : <RiskStatusBadge status={r.status} />}</TableCell>
          </TableRow>
        ))}
        {risks.length === 0 ? (
          <TableRow>
            <TableCell colSpan={6} className="py-10 text-center text-sm text-muted-foreground">
              {suggested ? 'No suggestions. Failing critical tests and AI assessments create suggestions here.' : 'No risks match.'}
            </TableCell>
          </TableRow>
        ) : null}
      </TableBody>
    </Table>
  )
}

function NewRiskDialog({ onClose }: { onClose: () => void }) {
  const create = useCreateRisk()
  const navigate = useNavigate()
  const [f, setF] = useState({ title: '', description: '', category: 'Security', likelihood: 3, impact: 3, treatment: 'mitigate', treatment_plan: '' })
  const [fields, setFields] = useState<Record<string, string>>({})
  return (
    <Dialog open onOpenChange={(o) => !o && onClose()}>
      <DialogContent className="max-w-xl">
        <form
          className="space-y-4"
          onSubmit={(e) => {
            e.preventDefault()
            create.mutate(f, {
              onSuccess: (r) => {
                onClose()
                void navigate({ to: '/app/risks/$id', params: { id: r.id } })
              },
              onError: (err) => (isApiError(err) && Object.keys(err.fields).length ? setFields(err.fields) : toast.error(errorMessage(err))),
            })
          }}
        >
          <DialogHeader>
            <DialogTitle>New risk</DialogTitle>
            <DialogDescription>Score inherent risk before treatment; residual risk can be set later.</DialogDescription>
          </DialogHeader>
          <FormField label="Title" htmlFor="rt" error={fields.title}>
            <Input id="rt" value={f.title} onChange={(e) => setF({ ...f, title: e.target.value })} data-testid="risk-title" />
          </FormField>
          <FormField label="Description" htmlFor="rd">
            <Textarea id="rd" value={f.description} onChange={(e) => setF({ ...f, description: e.target.value })} />
          </FormField>
          <div className="grid grid-cols-3 gap-3">
            <FormField label="Category" htmlFor="rc">
              <Select id="rc" value={f.category} onChange={(e) => setF({ ...f, category: e.target.value })}>
                {['Security', 'Privacy', 'Operational', 'Third party', 'Compliance', 'People', 'AI'].map((c) => (
                  <option key={c}>{c}</option>
                ))}
              </Select>
            </FormField>
            <FormField label="Likelihood (1-5)" htmlFor="rl">
              <Select id="rl" value={f.likelihood} onChange={(e) => setF({ ...f, likelihood: Number(e.target.value) })} data-testid="risk-likelihood">
                {[1, 2, 3, 4, 5].map((n) => (
                  <option key={n}>{n}</option>
                ))}
              </Select>
            </FormField>
            <FormField label="Impact (1-5)" htmlFor="ri">
              <Select id="ri" value={f.impact} onChange={(e) => setF({ ...f, impact: Number(e.target.value) })} data-testid="risk-impact">
                {[1, 2, 3, 4, 5].map((n) => (
                  <option key={n}>{n}</option>
                ))}
              </Select>
            </FormField>
          </div>
          <FormField label="Treatment plan" htmlFor="rp">
            <Textarea id="rp" value={f.treatment_plan} onChange={(e) => setF({ ...f, treatment_plan: e.target.value })} />
          </FormField>
          <DialogFooter>
            <Button type="submit" loading={create.isPending} data-testid="risk-create">
              Create risk
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
