import { useState } from 'react'
import { Link, useParams } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import { Bot, Download, FileUp, Play, ShieldCheck, ThumbsDown, ThumbsUp, Trash2, UserRound } from 'lucide-react'
import { toast } from 'sonner'
import { usePermissions } from '@/api/auth'
import { errorMessage, isApiError } from '@/api/client'
import { controlQuery, useDeleteEvidence, useReviewEvidence, useRunTest, useSetControlOwner, useUploadEvidence, type Evidence } from '@/api/compliance'
import { membersQuery } from '@/api/org'
import { FormField } from '@/components/layout/FormField'
import { ConnectorLogo, ControlStatusBadge, SeverityBadge, TestStatusBadge } from '@/components/status'
import { connectorName } from '@/lib/connectors'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Select } from '@/components/ui/select'
import { Textarea } from '@/components/ui/textarea'
import { formatDate, timeAgo } from '@/lib/utils'

export function ControlDetailPage() {
  const { id } = useParams({ from: '/app/controls/$id' })
  const { data: c } = useQuery(controlQuery(id))
  const { can } = usePermissions()
  const members = useQuery({ ...membersQuery, enabled: can('members:read') })
  const setOwner = useSetControlOwner(id)
  const runTest = useRunTest()
  const [uploadOpen, setUploadOpen] = useState(false)
  if (!c) return <p className="text-sm text-muted-foreground">Loading…</p>
  const byFramework = c.requirements.reduce<Record<string, typeof c.requirements>>((acc, r) => {
    ;(acc[r.framework_name] ??= []).push(r)
    return acc
  }, {})
  return (
    <div className="space-y-6">
      <div className="flex flex-wrap items-start justify-between gap-4">
        <div>
          <div className="flex items-center gap-3">
            <code className="rounded bg-muted px-2 py-1 text-sm font-semibold">{c.code}</code>
            <ControlStatusBadge status={c.status} className="text-sm" />
            <Badge variant="outline" className="capitalize">
              {c.evidence_mode} evidence
            </Badge>
          </div>
          <h1 className="mt-2 text-2xl font-semibold tracking-tight" data-testid="control-name">
            {c.name}
          </h1>
          <p className="mt-1 max-w-3xl text-sm text-muted-foreground">{c.description}</p>
          <p className="mt-2 text-sm" data-testid="control-status-reason">
            {c.status_reason} <span className="text-muted-foreground">· since {timeAgo(c.status_changed_at)}</span>
          </p>
        </div>
        <div className="w-64 space-y-1.5">
          <span className="flex items-center gap-1 text-xs font-medium uppercase tracking-wide text-muted-foreground">
            <UserRound className="h-3.5 w-3.5" /> Owner
          </span>
          {can('controls:write') && members.data ? (
            <Select
              value={c.owner?.membership_id ?? ''}
              data-testid="control-owner"
              onChange={(e) => setOwner.mutate(e.target.value, { onSuccess: () => toast.success('Owner updated'), onError: (err) => toast.error(errorMessage(err)) })}
            >
              <option value="">Unassigned</option>
              {members.data.items.map((m) => (
                <option key={m.id} value={m.id}>
                  {m.name} ({m.role})
                </option>
              ))}
            </Select>
          ) : (
            <div className="text-sm">{c.owner?.name ?? 'Unassigned'}</div>
          )}
        </div>
      </div>

      {c.open_tasks.length ? (
        <Card className="border-destructive/40 bg-destructive/5">
          <CardContent className="space-y-2 p-4">
            {c.open_tasks.map((t) => (
              <Link key={t.id} to="/app/tasks/$id" params={{ id: t.id }} className="flex items-center justify-between text-sm hover:underline" data-testid="control-open-task">
                <span className="font-medium">{t.title}</span>
                <span className="text-xs text-muted-foreground">{t.ai_status === 'done' ? 'Remediation plan ready' : 'Preparing remediation plan…'}</span>
              </Link>
            ))}
          </CardContent>
        </Card>
      ) : null}

      <div className="grid gap-6 lg:grid-cols-3">
        <div className="space-y-6 lg:col-span-2">
          <Card>
            <CardHeader>
              <CardTitle>Automated tests</CardTitle>
              <CardDescription>Tests run against every integration sync. Any failing test fails the control.</CardDescription>
            </CardHeader>
            <CardContent className="p-0">
              {c.tests.length ? (
                <ul className="divide-y" data-testid="control-tests">
                  {c.tests.map((t) => (
                    <li key={t.id} className="flex items-center gap-3 px-6 py-3" data-testid={`control-test-${t.catalog_id ?? t.id}`}>
                      <ConnectorLogo type={t.integration_type} className="h-8 w-8 text-[10px]" />
                      <div className="min-w-0 flex-1">
                        <Link to="/app/tests/$id" params={{ id: t.id }} className="text-sm font-medium hover:underline">
                          {t.name}
                        </Link>
                        <div className="truncate text-xs text-muted-foreground">
                          {connectorName(t.integration_type)} · {t.last_summary || 'Not run yet'} {t.last_run_at ? `· ${timeAgo(t.last_run_at)}` : ''}
                        </div>
                      </div>
                      <SeverityBadge severity={t.severity} />
                      <TestStatusBadge status={t.last_status} />
                      {can('tests:write') ? (
                        <Button
                          variant="ghost"
                          size="icon"
                          title="Run now"
                          onClick={() => runTest.mutate(t.id, { onSuccess: (r) => toast.success(`${r.name}: ${r.last_status}`), onError: (e) => toast.error(errorMessage(e)) })}
                        >
                          <Play />
                        </Button>
                      ) : null}
                    </li>
                  ))}
                </ul>
              ) : (
                <p className="px-6 pb-6 text-sm text-muted-foreground">
                  {c.evidence_mode === 'manual' ? 'This control is evidenced manually.' : 'No connected integration monitors this control yet. '}
                  {c.evidence_mode !== 'manual' ? (
                    <Link to="/app/integrations" className="underline">
                      Connect an integration
                    </Link>
                  ) : null}
                </p>
              )}
            </CardContent>
          </Card>

          <Card>
            <CardHeader className="flex-row items-start justify-between space-y-0">
              <div>
                <CardTitle>Evidence</CardTitle>
                <CardDescription>{c.evidence_hint}</CardDescription>
              </div>
              {can('evidence:write') ? (
                <Button size="sm" onClick={() => setUploadOpen(true)} data-testid="evidence-upload-open">
                  <FileUp /> Upload
                </Button>
              ) : null}
            </CardHeader>
            <CardContent className="space-y-3">
              {c.evidence.length === 0 ? <p className="text-sm text-muted-foreground">No evidence uploaded.</p> : null}
              {c.evidence.map((e) => (
                <EvidenceItem key={e.id} e={e} canWrite={can('evidence:write')} />
              ))}
            </CardContent>
          </Card>
        </div>

        <Card className="h-fit">
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <ShieldCheck className="h-4 w-4" /> Mapped requirements
            </CardTitle>
            <CardDescription>This control satisfies these requirements across frameworks.</CardDescription>
          </CardHeader>
          <CardContent className="space-y-4" data-testid="control-requirements">
            {Object.entries(byFramework).map(([name, reqs]) => (
              <div key={name}>
                <div className="mb-1 flex items-center gap-2 text-sm font-medium">
                  {name}
                  {!reqs[0]!.adopted ? (
                    <Badge variant="outline" className="font-normal">
                      not in scope
                    </Badge>
                  ) : null}
                </div>
                <ul className="space-y-1">
                  {reqs.map((r) => (
                    <li key={r.code} className="text-xs">
                      <code className="font-semibold">{r.code}</code> <span className="text-muted-foreground">{r.title}</span>
                    </li>
                  ))}
                </ul>
              </div>
            ))}
          </CardContent>
        </Card>
      </div>
      <UploadDialog controlId={c.id} ttl={c.evidence_ttl_days} open={uploadOpen} onOpenChange={setUploadOpen} />
    </div>
  )
}

function EvidenceItem({ e, canWrite }: { e: Evidence; canWrite: boolean }) {
  const review = useReviewEvidence()
  const del = useDeleteEvidence()
  const verdictCls = e.verdict === 'pass' ? 'border-success/40' : e.verdict === 'fail' ? 'border-destructive/40' : 'border-border'
  return (
    <div className={`rounded-lg border p-3 ${verdictCls}`} data-testid="evidence-item" data-verdict={e.verdict}>
      <div className="flex flex-wrap items-center gap-2">
        <span className="font-medium">{e.title}</span>
        <span className="text-xs text-muted-foreground">
          {e.filename} · {(e.size_bytes / 1024).toFixed(1)} KB · uploaded {timeAgo(e.created_at)} by {e.uploaded_by || 'system'}
        </span>
        <span className="ml-auto flex items-center gap-1">
          {e.expired ? <Badge variant="destructive">Expired</Badge> : e.expires_at ? <Badge variant="outline">Expires {formatDate(e.expires_at)}</Badge> : null}
          <a href={`/api/v1/evidence/${e.id}/download`} className="rounded p-1.5 hover:bg-accent" title="Download">
            <Download className="h-4 w-4" />
          </a>
          {canWrite ? (
            <button className="rounded p-1.5 text-destructive hover:bg-accent" title="Delete" onClick={() => del.mutate(e.id, { onSuccess: () => toast.success('Evidence deleted') })}>
              <Trash2 className="h-4 w-4" />
            </button>
          ) : null}
        </span>
      </div>
      <div className="mt-2 rounded-md bg-muted/50 p-2 text-sm" data-testid="evidence-ai">
        <div className="flex items-center gap-2 text-xs font-medium">
          <Bot className="h-3.5 w-3.5" />
          {e.ai_status === 'pending' ? (
            <span className="text-muted-foreground">AI review in progress…</span>
          ) : e.ai_status === 'done' ? (
            <>
              AI verdict:
              <Badge variant={e.ai_verdict === 'pass' ? 'success' : 'destructive'} data-testid="evidence-ai-verdict">
                {e.ai_verdict === 'pass' ? 'Satisfies control' : 'Does not satisfy'}
              </Badge>
              {e.ai_confidence != null ? <span className="text-muted-foreground">{Math.round(e.ai_confidence * 100)}% confidence · {e.ai_model}</span> : null}
            </>
          ) : e.ai_status === 'skipped' ? (
            <span className="text-muted-foreground">AI review disabled — needs a human decision</span>
          ) : (
            <span className="text-destructive">AI review failed</span>
          )}
        </div>
        {e.ai_reasoning ? <p className="mt-1 text-xs text-muted-foreground">{e.ai_reasoning}</p> : null}
        {e.override_verdict ? (
          <p className="mt-1 text-xs" data-testid="evidence-override">
            <strong>Reviewer decision:</strong> {e.override_verdict === 'pass' ? 'accepted' : 'rejected'} by {e.override_by}
            {e.override_reason ? ` — ${e.override_reason}` : ''}
          </p>
        ) : null}
      </div>
      {canWrite ? (
        <div className="mt-2 flex gap-2">
          <Button size="sm" variant="outline" onClick={() => review.mutate({ id: e.id, verdict: 'pass', reason: '' }, { onSuccess: () => toast.success('Evidence accepted') })} data-testid="evidence-accept">
            <ThumbsUp /> Accept
          </Button>
          <Button
            size="sm"
            variant="outline"
            onClick={() => {
              const reason = window.prompt('Why is this evidence insufficient?') ?? ''
              review.mutate({ id: e.id, verdict: 'fail', reason }, { onSuccess: () => toast.success('Evidence rejected') })
            }}
            data-testid="evidence-reject"
          >
            <ThumbsDown /> Reject
          </Button>
        </div>
      ) : null}
    </div>
  )
}

function UploadDialog({ controlId, ttl, open, onOpenChange }: { controlId: string; ttl: number; open: boolean; onOpenChange: (o: boolean) => void }) {
  const upload = useUploadEvidence(controlId)
  const [file, setFile] = useState<File | null>(null)
  const [title, setTitle] = useState('')
  const [description, setDescription] = useState('')
  const [expires, setExpires] = useState('')
  const [fields, setFields] = useState<Record<string, string>>({})
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <form
          className="space-y-4"
          onSubmit={(e) => {
            e.preventDefault()
            if (!file) {
              setFields({ file: 'Choose a file' })
              return
            }
            setFields({})
            upload.mutate(
              { file, title, description, expires_at: expires || undefined },
              {
                onSuccess: () => {
                  toast.success('Evidence uploaded — AI review started')
                  setFile(null)
                  setTitle('')
                  setDescription('')
                  onOpenChange(false)
                },
                onError: (err) => (isApiError(err) && Object.keys(err.fields).length ? setFields(err.fields) : toast.error(errorMessage(err))),
              },
            )
          }}
        >
          <DialogHeader>
            <DialogTitle>Upload evidence</DialogTitle>
            <DialogDescription>PDF, image, CSV, JSON or text up to 20 MB. Evidence expires after {ttl} days unless you set a date.</DialogDescription>
          </DialogHeader>
          <FormField label="File" htmlFor="ev-file" error={fields.file}>
            <Input id="ev-file" type="file" onChange={(e) => setFile(e.target.files?.[0] ?? null)} data-testid="evidence-file" />
          </FormField>
          <FormField label="Title" htmlFor="ev-title">
            <Input id="ev-title" value={title} onChange={(e) => setTitle(e.target.value)} placeholder="Defaults to the file name" data-testid="evidence-title" />
          </FormField>
          <FormField label="Description" htmlFor="ev-desc">
            <Textarea id="ev-desc" value={description} onChange={(e) => setDescription(e.target.value)} placeholder="What does this show?" />
          </FormField>
          <FormField label="Expires on" htmlFor="ev-exp" error={fields.expires_at}>
            <Input id="ev-exp" type="date" value={expires} onChange={(e) => setExpires(e.target.value)} />
          </FormField>
          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
              Cancel
            </Button>
            <Button type="submit" loading={upload.isPending} data-testid="evidence-submit">
              Upload
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
