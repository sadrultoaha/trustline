import { Link, useParams } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import { CheckCircle2, Circle, CircleSlash } from 'lucide-react'
import { toast } from 'sonner'
import { usePermissions } from '@/api/auth'
import { personQuery, useAttestBackground, useCompleteTask } from '@/api/phase2'
import type { TaskRow } from '@/api/compliance'
import { PageHeader } from '@/components/layout/PageHeader'
import { SeverityBadge } from '@/components/status'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { connectorName } from '@/lib/connectors'
import { formatDate, timeAgo } from '@/lib/utils'
import { PersonStatus, Tick } from './PeoplePage'

export function PersonDetailPage() {
  const { id } = useParams({ from: '/app/people/$id' })
  const { data: p } = useQuery(personQuery(id))
  const { can } = usePermissions()
  const attest = useAttestBackground(id)
  if (!p) return <p className="text-sm text-muted-foreground">Loading…</p>
  // Only the current offboarding round; earlier rounds (re-hire, re-termination) are history.
  const since = p.terminated_at ? new Date(p.terminated_at).getTime() - 60_000 : 0
  const offboarding = p.tasks.filter((t) => t.kind === 'offboarding' && p.status === 'terminated' && new Date(t.created_at).getTime() >= since)
  const onboarding = p.tasks.filter((t) => t.kind === 'onboarding')
  const other = p.tasks.filter((t) => t.kind !== 'offboarding' && t.kind !== 'onboarding')
  const history = p.tasks.filter((t) => t.kind === 'offboarding' && !offboarding.includes(t))
  return (
    <div className="space-y-6">
      <PageHeader title={p.name} description={`${p.title} · ${p.department} · ${p.email}`} actions={<PersonStatus status={p.status} />} />
      <Card>
        <CardContent className="grid gap-4 p-5 text-sm md:grid-cols-4">
          <Info label="Hired" value={p.hire_date ? formatDate(p.hire_date) : '—'} />
          <Info label="Terminated" value={p.termination_date ? formatDate(p.termination_date) : '—'} />
          <Info label="Manager" value={p.manager_email || '—'} />
          <Info label="Employment" value={p.employment_type.replace('_', ' ')} />
          <Info label="Policies acknowledged" value={<Tick ok={p.policies_accepted} />} />
          <Info label="Training" value={<Tick ok={p.training_completed} />} />
          <Info label="Device compliant" value={<Tick ok={p.device_enrolled && p.device_compliant} />} />
          <Info
            label="Background check"
            value={
              <div className="flex items-center gap-2">
                <span className="capitalize">{p.background_check.replace('_', ' ')}</span>
                {can('people:write') && p.background_check !== 'completed' ? (
                  <Button size="sm" variant="outline" onClick={() => attest.mutate('completed', { onSuccess: () => toast.success('Background check attested') })} data-testid="attest-background">
                    Attest completed
                  </Button>
                ) : null}
              </div>
            }
          />
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>Access across systems</CardTitle>
          <CardDescription>From the latest sync of each connected system.</CardDescription>
        </CardHeader>
        <CardContent className="flex flex-wrap gap-3" data-testid="person-access">
          {Object.entries(p.access).map(([k, v]) => (
            <div key={k} className="flex items-center gap-2 rounded-md border px-3 py-2 text-sm" data-testid={`access-${k}`} data-active={v}>
              <Tick ok={p.status === 'terminated' ? !v : v} />
              {connectorName(k)} <span className="text-xs text-muted-foreground">{v ? 'active' : 'no access'}</span>
            </div>
          ))}
        </CardContent>
      </Card>

      {offboarding.length ? <Checklist title="Offboarding" description="Each item is verified automatically from the connected system on its next sync." tasks={offboarding} /> : null}
      {onboarding.length ? <Checklist title="Onboarding" description="Items complete themselves as the person acknowledges policies, passes training and enrolls a device." tasks={onboarding} /> : null}
      {other.length ? <Checklist title="Other tasks" tasks={other} /> : null}
      {history.length ? <Checklist title="Earlier offboarding" tasks={history} /> : null}

      <Card>
        <CardHeader>
          <CardTitle>Devices</CardTitle>
        </CardHeader>
        <CardContent>
          {p.devices.length === 0 ? <p className="text-sm text-muted-foreground">No device reports.</p> : null}
          <ul className="space-y-2 text-sm">
            {p.devices.map((d) => (
              <li key={d.id} className="flex flex-wrap items-center gap-2">
                <span className="font-medium">{d.hostname}</span>
                <span className="text-muted-foreground">
                  {d.platform} {d.os_version} · {d.serial} · seen {timeAgo(d.last_seen_at)}
                </span>
                {d.compliant ? <Badge variant="success">Compliant</Badge> : d.issues.map((i) => <Badge key={i} variant="destructive">{i}</Badge>)}
              </li>
            ))}
          </ul>
        </CardContent>
      </Card>
    </div>
  )
}

function Info({ label, value }: { label: string; value: React.ReactNode }) {
  return (
    <div>
      <div className="text-xs uppercase tracking-wide text-muted-foreground">{label}</div>
      <div className="mt-0.5 capitalize">{value}</div>
    </div>
  )
}

function Checklist({ title, description, tasks }: { title: string; description?: string; tasks: TaskRow[] }) {
  const complete = useCompleteTask()
  const { can } = usePermissions()
  return (
    <Card data-testid={`checklist-${title.toLowerCase().split(' ')[0]}`}>
      <CardHeader>
        <CardTitle>
          {title} · {tasks.filter((t) => t.status === 'done').length}/{tasks.length}
        </CardTitle>
        {description ? <CardDescription>{description}</CardDescription> : null}
      </CardHeader>
      <CardContent>
        <ul className="divide-y">
          {tasks.map((t) => (
            <li key={t.id} className="flex items-start gap-3 py-2.5" data-testid={`checklist-item-${t.item_key || t.kind}`} data-status={t.status}>
              {t.status === 'done' ? <CheckCircle2 className="mt-0.5 h-4 w-4 text-success" /> : t.status === 'dismissed' ? <CircleSlash className="mt-0.5 h-4 w-4 text-muted-foreground" /> : <Circle className="mt-0.5 h-4 w-4 text-muted-foreground" />}
              <div className="min-w-0 flex-1">
                <Link to="/app/tasks/$id" params={{ id: t.id }} className="text-sm font-medium hover:underline">
                  {t.title}
                </Link>
                <div className="text-xs text-muted-foreground">
                  {t.resolution || t.description}
                  {t.status === 'open' && t.due_at ? ` · due ${formatDate(t.due_at)}` : ''}
                </div>
              </div>
              {t.status === 'open' ? <SeverityBadge severity={t.priority} /> : null}
              {t.overdue ? <Badge variant="destructive">overdue</Badge> : null}
              {t.status === 'open' && can('people:write') && (t.item_key === 'devices' || t.item_key === 'background') ? (
                <Button size="sm" variant="outline" onClick={() => complete.mutate({ id: t.id, note: 'Confirmed by HR' }, { onSuccess: () => toast.success('Completed') })}>
                  Mark done
                </Button>
              ) : null}
            </li>
          ))}
        </ul>
      </CardContent>
    </Card>
  )
}
