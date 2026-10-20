import { Link } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import { AlertTriangle, CheckCircle2, ListTodo, Plug, XCircle } from 'lucide-react'
import { useMe, usePermissions } from '@/api/auth'
import { dashboardQuery } from '@/api/compliance'
import { PageHeader } from '@/components/layout/PageHeader'
import { ControlStatusBadge } from '@/components/status'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { ReadinessCard } from '@/features/frameworks/FrameworksPage'
import { MyWorkPage } from '@/features/mywork/MyWorkPage'

export function DashboardPage() {
  const { data: me } = useMe()
  const { can } = usePermissions()
  const d = useQuery({ ...dashboardQuery(), enabled: can('controls:read') })
  if (!can('controls:read')) {
    return (
      <div>
        <p className="mb-2 text-sm text-muted-foreground" data-testid="employee-home">
          {me?.org?.name} · signed in as {me?.role}
        </p>
        <MyWorkPage />
      </div>
    )
  }
  const data = d.data
  return (
    <div className="space-y-6">
      <PageHeader title={`Welcome, ${me?.user.name ?? ''}`} description={`${me?.org?.name ?? ''} · signed in as ${me?.role}`} />
      <div className="grid gap-4 md:grid-cols-4">
        <Stat icon={CheckCircle2} tone="text-success" label="Controls passing" value={data ? `${data.controls.passing}/${data.controls.total}` : '…'} testid="dash-passing" />
        <Stat icon={XCircle} tone="text-destructive" label="Controls failing" value={data?.controls.failing ?? '…'} testid="dash-failing" />
        <Stat icon={AlertTriangle} tone="text-amber-600" label="Need evidence" value={data?.controls.needs_evidence ?? '…'} testid="dash-needs-evidence" />
        <Stat icon={ListTodo} tone="text-primary" label="Open tasks" value={data ? `${data.open_tasks}${data.overdue_tasks ? ` (${data.overdue_tasks} overdue)` : ''}` : '…'} testid="dash-open-tasks" />
      </div>
      {data && data.integrations === 0 ? (
        <Card className="border-primary/40 bg-primary/5">
          <CardContent className="flex items-center justify-between gap-4 p-5">
            <div>
              <div className="font-medium">Connect your first integration</div>
              <div className="text-sm text-muted-foreground">Trustline tests your cloud, source control, identity provider and HR system continuously.</div>
            </div>
            <Button asChild>
              <Link to="/app/integrations">
                <Plug /> Connect
              </Link>
            </Button>
          </CardContent>
        </Card>
      ) : null}
      <div>
        <div className="mb-3 flex items-center justify-between">
          <h2 className="text-lg font-semibold">Framework readiness</h2>
          <Link to="/app/frameworks" className="text-sm text-muted-foreground underline">
            All frameworks
          </Link>
        </div>
        {data?.frameworks.length ? (
          <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
            {data.frameworks.map((f) => (
              <ReadinessCard key={f.id} f={f} />
            ))}
          </div>
        ) : (
          <Card>
            <CardContent className="p-6 text-sm text-muted-foreground">
              No frameworks in scope.{' '}
              <Link to="/app/frameworks" className="underline">
                Add SOC 2, ISO 27001 or another framework
              </Link>
              .
            </CardContent>
          </Card>
        )}
      </div>
      <Card>
        <CardHeader>
          <CardTitle>Failing controls</CardTitle>
          <CardDescription>Fix these first — each has a remediation task.</CardDescription>
        </CardHeader>
        <CardContent className="p-0">
          {data?.failing_controls.length ? (
            <ul className="divide-y" data-testid="dash-failing-list">
              {data.failing_controls.map((c) => (
                <li key={c.id} className="flex items-center gap-3 px-6 py-3">
                  <code className="w-16 text-xs font-semibold">{c.code}</code>
                  <Link to="/app/controls/$id" params={{ id: c.id }} className="flex-1 text-sm font-medium hover:underline">
                    {c.name}
                  </Link>
                  <span className="text-xs text-muted-foreground">{c.status_reason}</span>
                  <ControlStatusBadge status={c.status} />
                </li>
              ))}
            </ul>
          ) : (
            <p className="px-6 pb-6 text-sm text-muted-foreground">No failing controls. </p>
          )}
        </CardContent>
      </Card>
    </div>
  )
}

function Stat({ icon: Icon, tone, label, value, testid }: { icon: typeof CheckCircle2; tone: string; label: string; value: React.ReactNode; testid: string }) {
  return (
    <Card>
      <CardContent className="flex items-center gap-4 p-5">
        <Icon className={`h-8 w-8 ${tone}`} />
        <div>
          <div className="text-2xl font-semibold tabular-nums" data-testid={testid}>
            {value}
          </div>
          <div className="text-xs text-muted-foreground">{label}</div>
        </div>
      </CardContent>
    </Card>
  )
}
