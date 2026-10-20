import { useState } from 'react'
import { useParams } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import { AlertTriangle, Bot, CheckCircle2, FilePen, Send, ShieldCheck, Sparkles, XCircle } from 'lucide-react'
import { toast } from 'sonner'
import { useMe, usePermissions } from '@/api/auth'
import { errorMessage } from '@/api/client'
import { membersQuery } from '@/api/org'
import { controlsQuery } from '@/api/compliance'
import { myWorkQuery, policyQuery, useSaveDraft, usePolicyAction, useVersionAction, type PolicyVersion } from '@/api/phase2'
import { FormField } from '@/components/layout/FormField'
import { PageHeader } from '@/components/layout/PageHeader'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Select } from '@/components/ui/select'
import { Textarea } from '@/components/ui/textarea'
import { formatDate, timeAgo } from '@/lib/utils'
import { PolicyEditor } from './PolicyEditor'
import { PolicyStatusBadge } from './PolicyStatusBadge'

const openStatuses = ['draft', 'in_review', 'approved', 'rejected']

export function PolicyDetailPage() {
  const { id } = useParams({ from: '/app/policies/$id' })
  const { data: p } = useQuery(policyQuery(id))
  const { can } = usePermissions()
  if (!p) return <p className="text-sm text-muted-foreground">Loading…</p>
  const published = p.versions.find((v) => v.status === 'published')
  const writable = can('policies:write')
  return (
    <div className="space-y-6">
      <PageHeader title={p.title} description={`${p.category} · review every ${p.review_cycle_days} days${p.next_review_at ? ` · next review ${formatDate(p.next_review_at)}` : ''}`} actions={<PolicyStatusBadge p={p} />} />
      {writable ? <WriterView policyId={id} /> : <ReaderView title={p.title} version={published} />}
    </div>
  )
}

function ReaderView({ title, version }: { title: string; version?: PolicyVersion }) {
  if (!version) return <p className="text-sm text-muted-foreground">Not published yet.</p>
  return (
    <Card>
      <CardContent className="p-6">
        <div className="policy-content" dangerouslySetInnerHTML={{ __html: version.content_html ?? '' }} />
        <div className="mt-6 border-t pt-4">
          <AcknowledgePanel title={title} version={version} />
        </div>
      </CardContent>
    </Card>
  )
}

function AcknowledgePanel({ title, version }: { title: string; version: PolicyVersion }) {
  const work = useQuery(myWorkQuery())
  const { acknowledge } = useVersionAction()
  const [agree, setAgree] = useState(false)
  const mine = work.data?.policies.find((x) => x.version_id === version.id)
  return (
    <>
          {mine?.accepted_at ? (
            <p className="flex items-center gap-2 text-sm text-success" data-testid="policy-acknowledged">
              <CheckCircle2 className="h-4 w-4" /> You acknowledged v{version.version} on {formatDate(mine.accepted_at)}.
            </p>
          ) : mine ? (
            <div className="flex flex-wrap items-center gap-4">
              <label className="flex items-center gap-2 text-sm">
                <input type="checkbox" checked={agree} onChange={(e) => setAgree(e.target.checked)} data-testid="policy-agree" /> I have read and will follow the {title}.
              </label>
              <Button disabled={!agree} loading={acknowledge.isPending} onClick={() => acknowledge.mutate(version.id, { onSuccess: () => toast.success('Acknowledged') })} data-testid="policy-acknowledge">
                Acknowledge
              </Button>
            </div>
          ) : null}
    </>
  )
}

function WriterView({ policyId }: { policyId: string }) {
  const { data: p } = useQuery(policyQuery(policyId))
  const { data: me } = useMe()
  const members = useQuery(membersQuery)
  const act = usePolicyAction(policyId)
  const va = useVersionAction()
  const save = useSaveDraft()
  const [html, setHtml] = useState<string | null>(null)
  const [summary, setSummary] = useState<string | null>(null)
  const [submitOpen, setSubmitOpen] = useState(false)
  const [aiOpen, setAiOpen] = useState(false)
  const [rejectOpen, setRejectOpen] = useState(false)
  if (!p) return null
  const open = p.versions.find((v) => openStatuses.includes(v.status))
  const published = p.versions.find((v) => v.status === 'published')
  const shown = open ?? published
  const editable = !!open && (open.status === 'draft' || open.status === 'rejected')
  const myMembership = members.data?.items.find((m) => m.user_id === me?.user.id)
  const canDecide = open?.status === 'in_review' && (me?.role === 'admin' || open.approver?.membership_id === myMembership?.id)
  const consistency = shown?.consistency
  const dirty = html !== null || summary !== null

  return (
    <div className="grid gap-6 lg:grid-cols-[1fr_340px]">
      <div className="space-y-4">
        <Card>
          <CardHeader className="flex-row flex-wrap items-center justify-between gap-3 space-y-0">
            <div>
              <CardTitle className="flex items-center gap-2">
                Version {shown?.version} <Badge variant="outline" className="capitalize" data-testid="policy-version-status">{shown?.status.replace('_', ' ')}</Badge>
                {shown?.ai_generated ? (
                  <Badge variant="secondary">
                    <Bot className="mr-1 h-3 w-3" /> AI-assisted
                  </Badge>
                ) : null}
              </CardTitle>
              <CardDescription>
                {open?.status === 'in_review' && open.approver ? `Awaiting approval from ${open.approver.name}` : null}
                {open?.status === 'rejected' ? `Changes requested: ${open.review_comment}` : null}
                {!open && published ? `Published ${formatDate(published.published_at, true)} by ${published.published_by}` : null}
              </CardDescription>
            </div>
            <div className="flex flex-wrap gap-2">
              {!open ? (
                <>
                  <Button variant="outline" onClick={() => act.markReviewed.mutate(undefined, { onSuccess: () => toast.success('Marked reviewed'), onError: (e) => toast.error(errorMessage(e)) })} disabled={!published}>
                    Mark reviewed
                  </Button>
                  <Button onClick={() => act.newVersion.mutate(undefined, { onSuccess: () => toast.success('New draft started'), onError: (e) => toast.error(errorMessage(e)) })} data-testid="policy-new-version">
                    <FilePen /> New version
                  </Button>
                </>
              ) : null}
              {editable ? (
                <>
                  <Button variant="outline" onClick={() => setAiOpen(true)} data-testid="policy-ai-draft-open">
                    <Sparkles /> Draft with AI
                  </Button>
                  <Button
                    variant="outline"
                    disabled={!dirty}
                    loading={save.isPending}
                    data-testid="policy-save"
                    onClick={() =>
                      save.mutate(
                        { versionId: open!.id, content_html: html ?? open!.content_html ?? '', change_summary: summary ?? open!.change_summary },
                        {
                          onSuccess: () => {
                            setHtml(null)
                            setSummary(null)
                            toast.success('Draft saved')
                          },
                          onError: (e) => toast.error(errorMessage(e)),
                        },
                      )
                    }
                  >
                    Save draft
                  </Button>
                  <Button disabled={dirty} onClick={() => setSubmitOpen(true)} data-testid="policy-submit-open" title={dirty ? 'Save the draft first' : undefined}>
                    <Send /> Submit for approval
                  </Button>
                </>
              ) : null}
              {canDecide ? (
                <>
                  <Button variant="outline" onClick={() => setRejectOpen(true)} data-testid="policy-reject">
                    <XCircle /> Request changes
                  </Button>
                  <Button onClick={() => va.decide.mutate({ id: open!.id, approve: true, comment: '' }, { onSuccess: () => toast.success('Approved'), onError: (e) => toast.error(errorMessage(e)) })} data-testid="policy-approve">
                    <CheckCircle2 /> Approve
                  </Button>
                </>
              ) : null}
              {open?.status === 'approved' ? (
                <Button loading={va.publish.isPending} onClick={() => va.publish.mutate(open.id, { onSuccess: () => toast.success('Published — everyone has been asked to acknowledge it'), onError: (e) => toast.error(errorMessage(e)) })} data-testid="policy-publish">
                  <ShieldCheck /> Publish
                </Button>
              ) : null}
            </div>
          </CardHeader>
          <CardContent className="space-y-3">
            {editable ? (
              <FormField label="Change summary" htmlFor="chg">
                <Input id="chg" value={summary ?? open!.change_summary} onChange={(e) => setSummary(e.target.value)} placeholder="What changed in this version?" data-testid="policy-change-summary" />
              </FormField>
            ) : null}
            {shown ? <PolicyEditor key={shown.id + (shown.ai_generated ? '-ai' : '') + shown.status} html={shown.content_html ?? ''} editable={editable} onChange={setHtml} /> : null}
          </CardContent>
        </Card>
        {published ? (
          <Card>
            <CardContent className="p-4">
              <div className="mb-2 text-xs font-medium uppercase tracking-wide text-muted-foreground">Your acknowledgement of v{published.version}</div>
              <AcknowledgePanel title={p.title} version={published} />
            </CardContent>
          </Card>
        ) : null}
      </div>

      <div className="space-y-4">
        <Card>
          <CardHeader>
            <CardTitle className="text-base">Details</CardTitle>
          </CardHeader>
          <CardContent className="space-y-3 text-sm">
            <FormField label="Owner" htmlFor="owner">
              <Select id="owner" value={p.owner?.membership_id ?? ''} onChange={(e) => act.setOwner.mutate(e.target.value, { onSuccess: () => toast.success('Owner updated') })}>
                <option value="">Unassigned</option>
                {members.data?.items.map((m) => (
                  <option key={m.id} value={m.id}>
                    {m.name}
                  </option>
                ))}
              </Select>
            </FormField>
            <ControlLinks codes={p.controls} onChange={(codes) => act.setControls.mutate(codes, { onSuccess: () => toast.success('Controls updated'), onError: (e) => toast.error(errorMessage(e)) })} />
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="flex-row items-center justify-between space-y-0">
            <CardTitle className="text-base">Consistency check</CardTitle>
            <Button size="sm" variant="ghost" loading={va.consistency.isPending} disabled={!shown || dirty} onClick={() => shown && va.consistency.mutate(shown.id, { onError: (e) => toast.error(errorMessage(e)) })} data-testid="policy-consistency">
              <Bot /> Check
            </Button>
          </CardHeader>
          <CardContent className="space-y-2 text-sm" data-testid="policy-consistency-result">
            {!consistency ? (
              <p className="text-muted-foreground">Compare this text with your Trustline settings (access review cadence, password length, acknowledgement threshold).</p>
            ) : consistency.issues.length === 0 ? (
              <p className="flex items-center gap-2 text-success">
                <CheckCircle2 className="h-4 w-4" /> Consistent with settings{shown?.consistency_checked_at ? ` · ${timeAgo(shown.consistency_checked_at)}` : ''}
              </p>
            ) : (
              consistency.issues.map((i, n) => (
                <Alert key={n} variant="destructive">
                  <AlertTriangle className="h-4 w-4" />
                  <AlertTitle>
                    Policy says {i.policy_value}, settings say {i.setting_value}
                  </AlertTitle>
                  <AlertDescription>
                    <p className="italic">“{i.quote}”</p>
                    <p className="mt-1">{i.explanation}</p>
                  </AlertDescription>
                </Alert>
              ))
            )}
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle className="text-base">
              Acknowledgements {p.published_version ? `(v${p.published_version}: ${p.accepted}/${p.audience})` : ''}
            </CardTitle>
          </CardHeader>
          <CardContent>
            <ul className="space-y-1 text-sm" data-testid="policy-acknowledgements">
              {p.acceptances.map((a) => (
                <li key={a.membership_id} className="flex items-center justify-between">
                  <span>{a.name}</span>
                  {a.accepted_at ? <span className="text-xs text-success">{formatDate(a.accepted_at)}</span> : <span className="text-xs text-muted-foreground">pending</span>}
                </li>
              ))}
            </ul>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle className="text-base">History</CardTitle>
          </CardHeader>
          <CardContent>
            <ol className="space-y-2 text-sm">
              {p.versions.map((v) => (
                <li key={v.id}>
                  <div className="flex items-center gap-2">
                    <span className="font-medium">v{v.version}</span>
                    <Badge variant="outline" className="capitalize">
                      {v.status.replace('_', ' ')}
                    </Badge>
                  </div>
                  <div className="text-xs text-muted-foreground">
                    {v.change_summary || 'No summary'} · by {v.created_by || 'system'}
                    {v.decided_by ? ` · decided by ${v.decided_by}` : ''}
                  </div>
                </li>
              ))}
            </ol>
          </CardContent>
        </Card>
      </div>

      {submitOpen && open ? <SubmitDialog versionId={open.id} onClose={() => setSubmitOpen(false)} /> : null}
      {aiOpen && open ? <AIDraftDialog versionId={open.id} onClose={() => setAiOpen(false)} onDone={() => setHtml(null)} /> : null}
      {rejectOpen && open ? <RejectDialog versionId={open.id} onClose={() => setRejectOpen(false)} /> : null}
    </div>
  )
}

function SubmitDialog({ versionId, onClose }: { versionId: string; onClose: () => void }) {
  const members = useQuery(membersQuery)
  const { submit } = useVersionAction()
  const approvers = members.data?.items.filter((m) => m.role === 'admin' || m.role === 'editor') ?? []
  const [approver, setApprover] = useState('')
  return (
    <Dialog open onOpenChange={(o) => !o && onClose()}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Submit for approval</DialogTitle>
          <DialogDescription>The approver gets a notification and can approve or request changes.</DialogDescription>
        </DialogHeader>
        <FormField label="Approver" htmlFor="appr">
          <Select id="appr" value={approver} onChange={(e) => setApprover(e.target.value)} data-testid="policy-approver">
            <option value="">Choose…</option>
            {approvers.map((m) => (
              <option key={m.id} value={m.id}>
                {m.name} ({m.role})
              </option>
            ))}
          </Select>
        </FormField>
        <DialogFooter>
          <Button
            disabled={!approver}
            loading={submit.isPending}
            data-testid="policy-submit"
            onClick={() => submit.mutate({ id: versionId, approver }, { onSuccess: () => { toast.success('Submitted for approval'); onClose() }, onError: (e) => toast.error(errorMessage(e)) })}
          >
            Submit
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

function AIDraftDialog({ versionId, onClose, onDone }: { versionId: string; onClose: () => void; onDone: () => void }) {
  const { aiDraft } = useVersionAction()
  const [instructions, setInstructions] = useState('')
  return (
    <Dialog open onOpenChange={(o) => !o && onClose()}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Draft with AI</DialogTitle>
          <DialogDescription>Rewrites this draft for your organization: your settings, connected systems, frameworks and team. Review it before submitting.</DialogDescription>
        </DialogHeader>
        <FormField label="Extra instructions (optional)" htmlFor="ins">
          <Textarea id="ins" value={instructions} onChange={(e) => setInstructions(e.target.value)} placeholder="e.g. Contractors must use company-managed laptops." data-testid="policy-ai-instructions" />
        </FormField>
        <DialogFooter>
          <Button
            loading={aiDraft.isPending}
            data-testid="policy-ai-draft"
            onClick={() =>
              aiDraft.mutate(
                { id: versionId, instructions },
                {
                  onSuccess: () => {
                    toast.success('Draft generated')
                    onDone()
                    onClose()
                  },
                  onError: (e) => toast.error(errorMessage(e)),
                },
              )
            }
          >
            <Sparkles /> Generate
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

function RejectDialog({ versionId, onClose }: { versionId: string; onClose: () => void }) {
  const { decide } = useVersionAction()
  const [comment, setComment] = useState('')
  return (
    <Dialog open onOpenChange={(o) => !o && onClose()}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Request changes</DialogTitle>
        </DialogHeader>
        <Textarea value={comment} onChange={(e) => setComment(e.target.value)} placeholder="What needs to change?" data-testid="policy-reject-comment" />
        <DialogFooter>
          <Button
            variant="destructive"
            loading={decide.isPending}
            onClick={() => decide.mutate({ id: versionId, approve: false, comment }, { onSuccess: () => { toast.success('Changes requested'); onClose() }, onError: (e) => toast.error(errorMessage(e)) })}
            data-testid="policy-reject-submit"
          >
            Send back
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

function ControlLinks({ codes, onChange }: { codes: string[]; onChange: (codes: string[]) => void }) {
  const controls = useQuery(controlsQuery({}))
  const options = (controls.data?.items ?? []).filter((c) => !codes.includes(c.code))
  return (
    <div data-testid="policy-controls">
      <div className="text-xs font-medium text-muted-foreground">Supports controls</div>
      <div className="mt-1 flex flex-wrap gap-1">
        {codes.map((c) => (
          <Badge key={c} variant="outline" className="gap-1">
            {c}
            <button aria-label={`Remove ${c}`} className="hover:text-destructive" onClick={() => onChange(codes.filter((x) => x !== c))}>
              <XCircle className="h-3 w-3" />
            </button>
          </Badge>
        ))}
      </div>
      <Select className="mt-2" value="" onChange={(e) => e.target.value && onChange([...codes, e.target.value])} data-testid="policy-add-control">
        <option value="">Add a control…</option>
        {options.map((c) => (
          <option key={c.id} value={c.code}>
            {c.code} · {c.name}
          </option>
        ))}
      </Select>
    </div>
  )
}
