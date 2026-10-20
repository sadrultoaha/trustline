import { Link, useParams } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import { Bot, Check, Copy, Loader2, RefreshCw, X } from 'lucide-react'
import { toast } from 'sonner'
import { usePermissions } from '@/api/auth'
import { errorMessage } from '@/api/client'
import { taskQuery, useRegenerateTask, useUpdateTask } from '@/api/compliance'
import { membersQuery } from '@/api/org'
import { PageHeader } from '@/components/layout/PageHeader'
import { SeverityBadge } from '@/components/status'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Select } from '@/components/ui/select'
import { formatDate } from '@/lib/utils'

export function TaskDetailPage() {
  const { id } = useParams({ from: '/app/tasks/$id' })
  const { data: t } = useQuery(taskQuery(id))
  const { can } = usePermissions()
  const members = useQuery({ ...membersQuery, enabled: can('members:read') })
  const update = useUpdateTask()
  const regen = useRegenerateTask()
  if (!t) return <p className="text-sm text-muted-foreground">Loading…</p>
  const writable = can('controls:write')
  return (
    <div className="space-y-6">
      <PageHeader
        title={t.title}
        description={`Created ${formatDate(t.created_at, true)}${t.due_at ? ` · due ${formatDate(t.due_at)}` : ''}`}
        actions={
          writable && t.status === 'open' ? (
            <>
              <Button variant="outline" onClick={() => update.mutate({ id, status: 'dismissed', resolution: 'Dismissed manually' }, { onSuccess: () => toast.success('Task dismissed') })}>
                <X /> Dismiss
              </Button>
              <Button onClick={() => update.mutate({ id, status: 'done', resolution: 'Marked done manually' }, { onSuccess: () => toast.success('Task completed') })} data-testid="task-done">
                <Check /> Mark done
              </Button>
            </>
          ) : writable ? (
            <Button variant="outline" onClick={() => update.mutate({ id, status: 'open' }, { onError: (e) => toast.error(errorMessage(e)) })}>
              Reopen
            </Button>
          ) : null
        }
      />
      <Card>
        <CardContent className="flex flex-wrap items-center gap-6 p-5 text-sm">
          <div>
            <div className="text-xs uppercase tracking-wide text-muted-foreground">Status</div>
            <Badge variant={t.status === 'open' ? 'warning' : 'success'} className="capitalize" data-testid="task-status">
              {t.status}
            </Badge>
          </div>
          <div>
            <div className="text-xs uppercase tracking-wide text-muted-foreground">Priority</div>
            <SeverityBadge severity={t.priority} />
          </div>
          {t.control_id ? (
            <div>
              <div className="text-xs uppercase tracking-wide text-muted-foreground">Control</div>
              <Link to="/app/controls/$id" params={{ id: t.control_id }} className="font-medium hover:underline">
                {t.control_code}
              </Link>
            </div>
          ) : null}
          {t.test_id ? (
            <div>
              <div className="text-xs uppercase tracking-wide text-muted-foreground">Test</div>
              <Link to="/app/tests/$id" params={{ id: t.test_id }} className="font-medium hover:underline">
                View test
              </Link>
            </div>
          ) : null}
          <div className="w-56">
            <div className="text-xs uppercase tracking-wide text-muted-foreground">Assignee</div>
            {writable && members.data ? (
              <Select value={t.assignee?.membership_id ?? ''} onChange={(e) => update.mutate({ id, assignee_membership_id: e.target.value }, { onSuccess: () => toast.success('Assignee updated') })}>
                <option value="">Unassigned</option>
                {members.data.items.map((m) => (
                  <option key={m.id} value={m.id}>
                    {m.name}
                  </option>
                ))}
              </Select>
            ) : (
              <span>{t.assignee?.name ?? 'Unassigned'}</span>
            )}
          </div>
          {t.resolution ? <div className="basis-full text-muted-foreground" data-testid="task-resolution">{t.resolution}</div> : null}
        </CardContent>
      </Card>

      <Card>
        <CardHeader className="flex-row items-start justify-between space-y-0">
          <div>
            <CardTitle className="flex items-center gap-2">
              <Bot className="h-4 w-4" /> Remediation plan
            </CardTitle>
            <CardDescription className="whitespace-pre-line">{t.description}</CardDescription>
          </div>
          {writable && t.kind === 'remediation' ? (
            <Button size="sm" variant="ghost" loading={regen.isPending} onClick={() => regen.mutate(id, { onSuccess: () => toast.success('Regenerating plan') })}>
              <RefreshCw /> Regenerate
            </Button>
          ) : null}
        </CardHeader>
        <CardContent className="space-y-4">
          {t.ai_status === 'pending' ? (
            <p className="flex items-center gap-2 text-sm text-muted-foreground" data-testid="task-ai-pending">
              <Loader2 className="h-4 w-4 animate-spin" /> Generating step-by-step fix and Terraform…
            </p>
          ) : null}
          <ol className="list-decimal space-y-1.5 pl-5 text-sm" data-testid="task-steps">
            {t.steps.map((s, i) => (
              <li key={i}>{s}</li>
            ))}
          </ol>
          {t.terraform ? (
            <div>
              <div className="mb-1 flex items-center justify-between">
                <span className="text-sm font-medium">Terraform</span>
                <Button
                  size="sm"
                  variant="ghost"
                  onClick={() => {
                    void navigator.clipboard?.writeText(t.terraform)
                    toast.success('Copied')
                  }}
                >
                  <Copy /> Copy
                </Button>
              </div>
              <pre className="overflow-auto rounded-md bg-[#0f172a] p-4 text-xs text-slate-100" data-testid="task-terraform">
                {t.terraform}
              </pre>
            </div>
          ) : null}
          {t.ai_notes ? <p className="text-xs text-muted-foreground">{t.ai_notes}</p> : null}
          {t.ai_model ? <p className="text-xs text-muted-foreground">Generated by {t.ai_model}</p> : null}
        </CardContent>
      </Card>
    </div>
  )
}
