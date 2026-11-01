import { useState } from 'react'
import { useParams } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import { Bot, Download, FileUp, Loader2, Mail, Pencil, RefreshCw } from 'lucide-react'
import { toast } from 'sonner'
import { usePermissions } from '@/api/auth'
import { errorMessage, isApiError } from '@/api/client'
import { frameworksQuery } from '@/api/compliance'
import { membersQuery } from '@/api/org'
import { vendorQuery, useVendorAction, type VendorAssessmentRow, type VendorDetail } from '@/api/phase3'
import { FormField } from '@/components/layout/FormField'
import { PageHeader } from '@/components/layout/PageHeader'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Select } from '@/components/ui/select'
import { connectorName } from '@/lib/connectors'
import { formatDate, timeAgo } from '@/lib/utils'
import { Cite, TierBadge } from './vendorUi'
import { scopeInput, VendorScopeFields, type VendorScope } from './VendorScopeFields'

const kinds = [
  ['soc2_type2', 'SOC 2 Type II report'],
  ['soc2_type1', 'SOC 2 Type I report'],
  ['iso27001', 'ISO 27001 certificate'],
  ['pentest', 'Penetration test summary'],
  ['dpa', 'Data processing agreement'],
  ['baa', 'Business associate agreement'],
  ['insurance', 'Cyber insurance'],
  ['other', 'Other'],
]

export function VendorDetailPage() {
  const { id } = useParams({ from: '/app/vendors/$id' })
  const { data: v } = useQuery(vendorQuery(id))
  const { can } = usePermissions()
  const act = useVendorAction(id)
  const members = useQuery({ ...membersQuery, enabled: can('vendors:write') })
  const [upload, setUpload] = useState(false)
  const [send, setSend] = useState(false)
  const [editScope, setEditScope] = useState(false)
  const fws = useQuery(frameworksQuery()).data?.items ?? []
  if (!v) return <p className="text-sm text-muted-foreground">Loading…</p>
  const writable = can('vendors:write')
  const latest = v.assessments.find((a) => a.status === 'done' && a.result)
  return (
    <div className="space-y-6">
      <PageHeader
        title={v.name}
        description={`${v.category || 'Vendor'}${v.domain ? ` · ${v.domain}` : ''} · ${v.status}${v.status === 'discovered' ? ` via ${connectorName(v.source)}` : ''}`}
        actions={
          writable ? (
            v.status === 'dismissed' ? (
              <Button variant="outline" onClick={() => act.restore.mutate(undefined, { onSuccess: () => toast.success('Restored') })} data-testid="vendor-restore">
                Restore
              </Button>
            ) : v.status === 'discovered' ? (
              <>
                <Button variant="outline" onClick={() => act.triage.mutate(false, { onSuccess: () => toast.success('Dismissed') })} data-testid="vendor-dismiss">
                  Dismiss
                </Button>
                <Button onClick={() => act.triage.mutate(true, { onSuccess: () => toast.success('Added to the inventory') })} data-testid="vendor-add-inventory">
                  Add to inventory
                </Button>
              </>
            ) : (
              <>
                <Button variant="outline" onClick={() => setSend(true)} data-testid="vendor-send-questionnaire">
                  <Mail /> Send questionnaire
                </Button>
                <Button variant="outline" onClick={() => act.reviewed.mutate(undefined, { onSuccess: () => toast.success('Marked reviewed') })}>
                  Mark reviewed
                </Button>
                <Button onClick={() => setUpload(true)} data-testid="vendor-upload-open">
                  <FileUp /> Upload document
                </Button>
              </>
            )
          ) : null
        }
      />
      {v.status === 'discovered' && v.discovery_note ? (
        <Alert>
          <AlertTitle>Discovered from {connectorName(v.source)}</AlertTitle>
          <AlertDescription>
            {v.discovery_note} · {v.discovery_users} user(s)
          </AlertDescription>
        </Alert>
      ) : null}
      <div className="grid gap-6 lg:grid-cols-[1fr_340px]">
        <div className="space-y-6">
          {latest?.result ? <AssessmentCard a={latest} v={v} /> : null}
          {v.assessments.some((a) => a.status === 'pending') ? (
            <p className="flex items-center gap-2 text-sm text-muted-foreground" data-testid="vendor-assessment-pending">
              <Loader2 className="h-4 w-4 animate-spin" /> AI review in progress…
            </p>
          ) : null}
          <Card>
            <CardHeader>
              <CardTitle>Documents</CardTitle>
            </CardHeader>
            <CardContent>
              {v.documents_list.length === 0 ? <p className="text-sm text-muted-foreground">No documents yet. Upload the vendor's SOC 2 report and DPA.</p> : null}
              <ul className="divide-y">
                {v.documents_list.map((d) => (
                  <li key={d.id} className="flex items-center justify-between gap-2 py-2 text-sm" data-testid={`vendor-doc-${d.kind}`}>
                    <span>
                      <span className="font-medium">{d.title}</span>{' '}
                      <span className="text-muted-foreground">
                        · {kinds.find((k) => k[0] === d.kind)?.[1]} · {timeAgo(d.created_at)}
                        {d.expires_at ? ` · expires ${formatDate(d.expires_at)}` : ''}
                      </span>
                    </span>
                    <span className="flex gap-1">
                      {writable && ['soc2_type2', 'soc2_type1', 'iso27001', 'pentest'].includes(d.kind) ? (
                        <Button size="sm" variant="ghost" title="Re-run AI review" onClick={() => act.assess.mutate(d.id, { onSuccess: () => toast.success('AI review started') })}>
                          <RefreshCw />
                        </Button>
                      ) : null}
                      <a href={`/api/v1/vendors/${v.id}/documents/${d.id}/download`} className="rounded p-1.5 hover:bg-accent" title="Download">
                        <Download className="h-4 w-4" />
                      </a>
                    </span>
                  </li>
                ))}
              </ul>
            </CardContent>
          </Card>
          <Card>
            <CardHeader>
              <CardTitle>Questionnaires</CardTitle>
            </CardHeader>
            <CardContent className="space-y-3">
              {v.questionnaires.length === 0 ? <p className="text-sm text-muted-foreground">No questionnaire sent.</p> : null}
              {v.questionnaires.map((q) => (
                <details key={q.id} className="rounded-md border" data-testid="vendor-questionnaire">
                  <summary className="flex cursor-pointer items-center justify-between px-3 py-2 text-sm">
                    <span>
                      To {q.recipient_email} · sent {formatDate(q.sent_at)}
                      <span className="text-muted-foreground"> · {q.templates.map((t) => t.title).join(', ')}</span>
                    </span>
                    <Badge variant={q.status === 'submitted' ? 'success' : 'outline'} className="capitalize">
                      {q.status.replace('_', ' ')}
                    </Badge>
                  </summary>
                  <div className="space-y-4 border-t p-3 text-sm">
                    {q.templates.map((t) => (
                      <div key={t.id} className="space-y-1">
                        {q.templates.length > 1 ? <div className="font-medium">{t.title}</div> : null}
                        {t.questions.map((qq) => (
                          <div key={qq.id} className="grid grid-cols-[1fr_auto] gap-2">
                            <span className="text-muted-foreground">{qq.question}</span>
                            <span className={q.answers[qq.id] === 'No' ? 'font-medium text-destructive' : 'font-medium'}>{q.answers[qq.id] || '—'}</span>
                          </div>
                        ))}
                      </div>
                    ))}
                  </div>
                </details>
              ))}
            </CardContent>
          </Card>
        </div>
        <div className="space-y-6">
          <Card data-testid="vendor-scope">
            <CardHeader className="flex-row items-center justify-between space-y-0">
              <CardTitle>Compliance scope</CardTitle>
              {writable ? (
                <Button size="sm" variant="ghost" onClick={() => setEditScope(true)} data-testid="vendor-scope-edit">
                  <Pencil /> Edit
                </Button>
              ) : null}
            </CardHeader>
            <CardContent className="space-y-3 text-sm">
              <div>
                <div className="mb-1 text-muted-foreground">Frameworks</div>
                {v.frameworks.length ? (
                  <div className="flex flex-wrap gap-1" data-testid="vendor-scope-frameworks">
                    {v.frameworks.map((id) => {
                      const f = fws.find((x) => x.id === id)
                      return (
                        <Badge key={id} variant="outline" className="font-normal" title={f && !f.adopted ? 'This framework is no longer in scope' : undefined}>
                          {f?.name ?? id}
                          {f && !f.adopted ? ' (not in scope)' : ''}
                        </Badge>
                      )
                    })}
                  </div>
                ) : (
                  <p className="text-muted-foreground">Not mapped to a framework.</p>
                )}
              </div>
              <div>
                <div className="mb-1 text-muted-foreground">Questionnaires{v.questionnaires_default ? ' (suggested from frameworks)' : ''}</div>
                <ul className="space-y-0.5" data-testid="vendor-scope-questionnaires">
                  {v.questionnaire_templates.map((t) => (
                    <li key={t.id}>
                      {t.title} <span className="text-xs text-muted-foreground">· {t.question_count} questions</span>
                    </li>
                  ))}
                </ul>
              </div>
            </CardContent>
          </Card>
          <Card className="h-fit">
            <CardHeader>
              <CardTitle>Profile</CardTitle>
            </CardHeader>
            <CardContent className="space-y-3 text-sm">
              <div className="flex items-center justify-between">
                <span className="text-muted-foreground">Tier</span>
                <TierBadge tier={v.tier} />
              </div>
              {v.recommended_tier && v.recommended_tier !== v.tier ? (
                <Alert data-testid="vendor-recommended">
                  <AlertDescription className="flex items-center justify-between gap-2">
                    <span>
                      AI recommends <strong className="capitalize">{v.recommended_tier}</strong>
                    </span>
                    {writable ? (
                      <Button size="sm" onClick={() => act.applyTier.mutate(undefined, { onSuccess: () => toast.success('Tier applied') })} data-testid="vendor-apply-tier">
                        Apply
                      </Button>
                    ) : null}
                  </AlertDescription>
                </Alert>
              ) : null}
              <FormField label="Data access" htmlFor="da">
                <Select id="da" value={v.data_access} disabled={!writable} onChange={(e) => act.update.mutate({ data_access: e.target.value })}>
                  {['none', 'internal', 'confidential', 'restricted'].map((d) => (
                    <option key={d}>{d}</option>
                  ))}
                </Select>
              </FormField>
              <label className="flex items-center gap-2">
                <input type="checkbox" checked={v.handles_pii} disabled={!writable} onChange={(e) => act.update.mutate({ handles_pii: e.target.checked })} /> Processes personal data
              </label>
              <FormField label="Tier" htmlFor="ti">
                <Select id="ti" value={v.tier} disabled={!writable} onChange={(e) => act.update.mutate({ tier: e.target.value })} data-testid="vendor-tier-select">
                  {['critical', 'high', 'medium', 'low', 'unassessed'].map((d) => (
                    <option key={d}>{d}</option>
                  ))}
                </Select>
              </FormField>
              <FormField label="Owner" htmlFor="ow">
                <Select id="ow" value={v.owner?.membership_id ?? ''} disabled={!writable} onChange={(e) => act.update.mutate({ owner_membership_id: e.target.value })}>
                  <option value="">Unassigned</option>
                  {members.data?.items.map((m) => (
                    <option key={m.id} value={m.id}>
                      {m.name}
                    </option>
                  ))}
                </Select>
              </FormField>
              <div className="text-xs text-muted-foreground">
                Review every {v.review_cycle_days} days · {v.last_reviewed_at ? `last ${formatDate(v.last_reviewed_at)}` : 'never reviewed'}
                {v.next_review_at ? ` · next ${formatDate(v.next_review_at)}` : ''}
              </div>
            </CardContent>
          </Card>
        </div>
      </div>
      {upload ? <UploadDialog v={v} onClose={() => setUpload(false)} /> : null}
      {send ? <SendDialog v={v} onClose={() => setSend(false)} /> : null}
      {editScope ? <ScopeDialog v={v} onClose={() => setEditScope(false)} /> : null}
    </div>
  )
}

function AssessmentCard({ a, v }: { a: VendorAssessmentRow; v: VendorDetail }) {
  const r = a.result!
  const failed = r.controls_tested.filter((c) => c.result === 'exceptions')
  return (
    <Card data-testid="vendor-assessment">
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <Bot className="h-4 w-4" /> AI review
          {r.opinion !== 'not_applicable' && r.opinion !== 'unknown' ? (
            <Badge variant={r.opinion === 'unqualified' ? 'success' : 'destructive'} className="capitalize" data-testid="vendor-opinion">
              {r.opinion} opinion
            </Badge>
          ) : null}
        </CardTitle>
        <CardDescription>
          {r.auditor ? `${r.auditor} · ` : ''}
          {r.period_start ? `${r.period_start} – ${r.period_end} · ` : ''}reviewed {a.completed_at ? timeAgo(a.completed_at) : ''} by {a.model}
          {a.questionnaire_id ? ' · from questionnaire answers' : ''}
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-4 text-sm">
        <p data-testid="vendor-summary">
          {r.summary}
          <Cite c={r.opinion_citation} />
        </p>
        <Alert variant={r.exceptions.length ? 'destructive' : 'success'}>
          <AlertDescription data-testid="vendor-risk-summary">{r.risk_summary}</AlertDescription>
        </Alert>
        {r.exceptions.length ? (
          <div>
            <div className="mb-1 font-medium">Exceptions</div>
            <ul className="space-y-2" data-testid="vendor-exceptions">
              {r.exceptions.map((e) => (
                <li key={e.control} className="rounded-md border p-2">
                  <div className="flex items-center gap-2">
                    <code className="font-semibold">{e.control}</code>
                    <Badge variant={e.severity === 'high' ? 'destructive' : 'warning'}>{e.severity}</Badge>
                    <Cite c={e.citation} />
                  </div>
                  <p className="mt-1">{e.description}</p>
                  {e.management_response ? <p className="mt-1 text-xs text-muted-foreground">Management response: {e.management_response}</p> : null}
                </li>
              ))}
            </ul>
          </div>
        ) : null}
        {r.cuecs.length ? (
          <div>
            <div className="mb-1 font-medium">Your responsibilities (CUECs)</div>
            <ul className="list-disc space-y-1 pl-5">
              {r.cuecs.map((c) => (
                <li key={c.description}>
                  {c.description}
                  <Cite c={c.citation} />
                </li>
              ))}
            </ul>
          </div>
        ) : null}
        {r.subservice_organizations.length ? (
          <div className="text-xs text-muted-foreground">
            Subservice organizations: {r.subservice_organizations.map((s) => s.description).join('; ')}
          </div>
        ) : null}
        {r.controls_tested.length ? (
          <details>
            <summary className="cursor-pointer text-sm font-medium">
              {r.controls_tested.length} controls tested · {failed.length} with exceptions
            </summary>
            <ul className="mt-2 space-y-1">
              {r.controls_tested.map((c) => (
                <li key={c.id} className="text-xs">
                  <code className="font-semibold">{c.id}</code> {c.description} — <span className={c.result === 'exceptions' ? 'text-destructive' : 'text-success'}>{c.result.replace('_', ' ')}</span>
                  <Cite c={c.citation} />
                </li>
              ))}
            </ul>
          </details>
        ) : null}
        <p className="text-xs text-muted-foreground">
          Recommended tier: <strong className="capitalize">{r.recommended_tier}</strong> — {r.recommended_tier_reason} (current: {v.tier})
        </p>
      </CardContent>
    </Card>
  )
}

function UploadDialog({ v, onClose }: { v: VendorDetail; onClose: () => void }) {
  const act = useVendorAction(v.id)
  const [file, setFile] = useState<File | null>(null)
  const [kind, setKind] = useState('soc2_type2')
  const [title, setTitle] = useState('')
  return (
    <Dialog open onOpenChange={(o) => !o && onClose()}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Upload a document</DialogTitle>
          <DialogDescription>SOC 2, ISO and pen-test reports are reviewed by AI with citations to the page and section.</DialogDescription>
        </DialogHeader>
        <FormField label="Type" htmlFor="dk">
          <Select id="dk" value={kind} onChange={(e) => setKind(e.target.value)} data-testid="vendor-doc-kind">
            {kinds.map(([k, l]) => (
              <option key={k} value={k}>
                {l}
              </option>
            ))}
          </Select>
        </FormField>
        <FormField label="File" htmlFor="df">
          <Input id="df" type="file" onChange={(e) => setFile(e.target.files?.[0] ?? null)} data-testid="vendor-doc-file" />
        </FormField>
        <FormField label="Title" htmlFor="dt">
          <Input id="dt" value={title} onChange={(e) => setTitle(e.target.value)} placeholder="Defaults to the file name" />
        </FormField>
        <DialogFooter>
          <Button
            disabled={!file}
            loading={act.upload.isPending}
            data-testid="vendor-doc-submit"
            onClick={() =>
              file &&
              act.upload.mutate(
                { file, kind, title },
                {
                  onSuccess: () => {
                    toast.success('Uploaded')
                    onClose()
                  },
                  onError: (e) => toast.error(errorMessage(e)),
                },
              )
            }
          >
            Upload
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

function SendDialog({ v, onClose }: { v: VendorDetail; onClose: () => void }) {
  const act = useVendorAction(v.id)
  const [email, setEmail] = useState('')
  const [link, setLink] = useState<string | null>(null)
  return (
    <Dialog open onOpenChange={(o) => !o && onClose()}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Send security questionnaire</DialogTitle>
          <DialogDescription>
            The vendor gets a magic link to a portal — no account needed. {v.questionnaire_templates.reduce((n, t) => n + t.question_count, 0)} questions; the link expires in 30 days.
          </DialogDescription>
        </DialogHeader>
        {!link ? (
          <ul className="list-disc space-y-0.5 pl-5 text-sm" data-testid="vendor-send-includes">
            {v.questionnaire_templates.map((t) => (
              <li key={t.id}>
                {t.title} <span className="text-muted-foreground">· {t.question_count} questions</span>
              </li>
            ))}
          </ul>
        ) : null}
        {link ? (
          <Alert variant="success">
            <AlertDescription>
              Sent. Link for your records: <code className="break-all text-xs" data-testid="vendor-portal-link">{link}</code>
            </AlertDescription>
          </Alert>
        ) : (
          <FormField label="Vendor contact email" htmlFor="ve">
            <Input id="ve" type="email" value={email} onChange={(e) => setEmail(e.target.value)} data-testid="vendor-questionnaire-email" />
          </FormField>
        )}
        <DialogFooter>
          {link ? (
            <Button onClick={onClose}>Done</Button>
          ) : (
            <Button
              loading={act.sendQuestionnaire.isPending}
              data-testid="vendor-questionnaire-send"
              onClick={() => act.sendQuestionnaire.mutate(email, { onSuccess: (q) => setLink(q.link ?? ''), onError: (e) => toast.error(errorMessage(e)) })}
            >
              Send
            </Button>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

function ScopeDialog({ v, onClose }: { v: VendorDetail; onClose: () => void }) {
  const act = useVendorAction(v.id)
  const [scope, setScope] = useState<VendorScope>({ frameworks: v.frameworks, questionnaires: v.questionnaires_default ? null : v.questionnaire_templates.map((t) => t.id) })
  const [fields, setFields] = useState<Record<string, string>>({})
  return (
    <Dialog open onOpenChange={(o) => !o && onClose()}>
      <DialogContent className="max-h-[90vh] max-w-2xl overflow-y-auto">
        <DialogHeader>
          <DialogTitle>Compliance scope</DialogTitle>
          <DialogDescription>Map {v.name} to the frameworks it supports and choose the questionnaires sent with the next request. Questionnaires already sent are not changed.</DialogDescription>
        </DialogHeader>
        <VendorScopeFields value={scope} onChange={setScope} errors={fields} keep={v.frameworks} />
        <DialogFooter>
          <Button variant="outline" onClick={onClose}>
            Cancel
          </Button>
          <Button
            loading={act.update.isPending}
            data-testid="vendor-scope-save"
            onClick={() =>
              act.update.mutate(scopeInput(scope), {
                onSuccess: () => {
                  toast.success('Scope updated')
                  onClose()
                },
                onError: (e) => (isApiError(e) && Object.keys(e.fields).length ? setFields(e.fields) : toast.error(errorMessage(e))),
              })
            }
          >
            Save
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
