import { useState } from 'react'
import { Link, useNavigate, useParams } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import { FlaskConical, RefreshCw, Unplug } from 'lucide-react'
import { toast } from 'sonner'
import { usePermissions } from '@/api/auth'
import { errorMessage } from '@/api/client'
import { integrationQuery, integrationTypesQuery, snapshotQuery, testsQuery, useDisconnectIntegration, useSetScenario, useSyncIntegration, type ScenarioInfo } from '@/api/compliance'
import { Select } from '@/components/ui/select'
import { PageHeader } from '@/components/layout/PageHeader'
import { ConnectorLogo, SeverityBadge, TestStatusBadge } from '@/components/status'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Switch } from '@/components/ui/switch'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { formatDate, timeAgo } from '@/lib/utils'

export function IntegrationDetailPage() {
  const { id } = useParams({ from: '/app/integrations/$id' })
  const { data: i } = useQuery(integrationQuery(id))
  const types = useQuery(integrationTypesQuery())
  const snap = useQuery({ ...snapshotQuery(id), enabled: !!i?.last_sync_at })
  const tests = useQuery({ ...testsQuery({ integration: i?.type }), enabled: !!i })
  const { can } = usePermissions()
  const sync = useSyncIntegration()
  const scenario = useSetScenario(id)
  const disconnect = useDisconnectIntegration()
  const navigate = useNavigate()
  const [confirm, setConfirm] = useState(false)
  if (!i) return <p className="text-sm text-muted-foreground">Loading…</p>
  const type = types.data?.items.find((t) => t.type === i.type)
  return (
    <div className="space-y-6">
      <PageHeader
        title={i.name}
        description={`${type?.category ?? ''} · connected ${formatDate(i.created_at)}`}
        actions={
          can('integrations:write') ? (
            <>
              <Button variant="outline" loading={sync.isPending} onClick={() => sync.mutate(id, { onSuccess: () => toast.success('Sync queued') })} data-testid="integration-sync">
                <RefreshCw /> Sync now
              </Button>
              <Button variant="outline" onClick={() => setConfirm(true)} data-testid="integration-disconnect">
                <Unplug /> Disconnect
              </Button>
            </>
          ) : null
        }
      />
      <Card>
        <CardContent className="flex flex-wrap items-center gap-6 p-5">
          <ConnectorLogo type={i.type} className="h-12 w-12" />
          <Info label="Status" value={<Badge variant={i.status === 'error' ? 'destructive' : i.status === 'connected' ? 'success' : 'secondary'} data-testid="integration-detail-status">{i.status}</Badge>} />
          <Info label="Mode" value={<span className="capitalize">{i.mode}</span>} />
          <Info label="Last sync" value={i.last_sync_at ? timeAgo(i.last_sync_at) : 'pending'} />
          <Info label="Resources" value={i.resource_count} />
          <Info label="Tests" value={`${i.tests_total - i.tests_failing}/${i.tests_total} passing`} />
          {Object.entries(i.credentials_hint).map(([k, v]) => (
            <Info key={k} label={k.replace(/_/g, ' ')} value={<code className="text-xs">{v}</code>} />
          ))}
        </CardContent>
      </Card>
      {i.last_error ? (
        <Alert variant="destructive">
          <AlertDescription>Last sync failed: {i.last_error}</AlertDescription>
        </Alert>
      ) : null}

      {i.mode === 'sandbox' && type ? (
        <Card data-testid="sandbox-panel">
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <FlaskConical className="h-4 w-4" /> Sandbox controls
            </CardTitle>
            <CardDescription>Change the simulated {type.name} account. Changes are picked up by the next sync — scheduled, or click Sync now.</CardDescription>
          </CardHeader>
          <CardContent className="divide-y">
            {type.scenarios.filter((s) => !s.per_person).map((s) => {
              const active = i.active_scenarios.includes(s.key)
              return (
                <div key={s.key} className="flex items-center justify-between gap-4 py-3">
                  <div>
                    <div className="text-sm font-medium">{s.label}</div>
                    <div className="text-xs text-muted-foreground">{s.description}</div>
                  </div>
                  <Switch
                    checked={active}
                    disabled={!can('integrations:write') || scenario.isPending}
                    data-testid={`scenario-${s.key}`}
                    onCheckedChange={(v) =>
                      scenario.mutate({ key: s.key, active: v }, { onSuccess: () => toast.success(`${s.label}: ${v ? 'on' : 'off'} — applies on next sync`), onError: (e) => toast.error(errorMessage(e)) })
                    }
                  />
                </div>
              )
            })}
            {type.scenarios.some((s) => s.per_person) ? <PersonScenarios integrationId={id} scenarios={type.scenarios.filter((s) => s.per_person)} active={i.active_scenarios} canWrite={can('integrations:write')} resources={snap.data?.resources} /> : null}
          </CardContent>
        </Card>
      ) : null}

      <Tabs defaultValue="tests">
        <TabsList>
          <TabsTrigger value="tests">Tests ({tests.data?.items.length ?? 0})</TabsTrigger>
          <TabsTrigger value="resources" data-testid="tab-resources">
            Resources
          </TabsTrigger>
        </TabsList>
        <TabsContent value="tests">
          <Card>
            <CardContent className="p-0">
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>Test</TableHead>
                    <TableHead>Control</TableHead>
                    <TableHead>Severity</TableHead>
                    <TableHead>Result</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {tests.data?.items.map((t) => (
                    <TableRow key={t.id}>
                      <TableCell>
                        <Link to="/app/tests/$id" params={{ id: t.id }} className="font-medium hover:underline">
                          {t.name}
                        </Link>
                        <div className="text-xs text-muted-foreground">{t.last_summary}</div>
                      </TableCell>
                      <TableCell>
                        <Link to="/app/controls/$id" params={{ id: t.control_id }} className="text-xs hover:underline">
                          <code>{t.control_code}</code>
                        </Link>
                      </TableCell>
                      <TableCell>
                        <SeverityBadge severity={t.severity} />
                      </TableCell>
                      <TableCell>
                        <TestStatusBadge status={t.last_status} />
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </CardContent>
          </Card>
        </TabsContent>
        <TabsContent value="resources">
          {snap.data ? (
            <Card>
              <CardHeader>
                <CardDescription>
                  Snapshot from {formatDate(snap.data.taken_at, true)} · SHA-256 <code className="text-xs">{snap.data.sha256.slice(0, 16)}…</code>
                </CardDescription>
              </CardHeader>
              <CardContent className="space-y-4">
                {Object.entries(snap.data.resources).map(([rt, list]) => (
                  <details key={rt} className="rounded-md border">
                    <summary className="cursor-pointer px-3 py-2 text-sm font-medium">
                      {type?.resources.find((r) => r.type === rt)?.label ?? rt} <span className="text-muted-foreground">({list.length})</span>
                    </summary>
                    <pre className="max-h-96 overflow-auto bg-muted/50 p-3 text-xs">{JSON.stringify(list, null, 2)}</pre>
                  </details>
                ))}
              </CardContent>
            </Card>
          ) : (
            <p className="text-sm text-muted-foreground">No snapshot yet.</p>
          )}
        </TabsContent>
      </Tabs>

      <Dialog open={confirm} onOpenChange={setConfirm}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Disconnect {i.name}?</DialogTitle>
            <DialogDescription>Its tests stop running and open remediation tasks are dismissed. Past snapshots and test results are kept as audit evidence.</DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setConfirm(false)}>
              Cancel
            </Button>
            <Button
              variant="destructive"
              loading={disconnect.isPending}
              onClick={() =>
                disconnect.mutate(id, {
                  onSuccess: () => {
                    toast.success(`${i.name} disconnected`)
                    void navigate({ to: '/app/integrations' })
                  },
                })
              }
            >
              Disconnect
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}

function Info({ label, value }: { label: string; value: React.ReactNode }) {
  return (
    <div>
      <div className="text-xs uppercase tracking-wide text-muted-foreground">{label}</div>
      <div className="mt-0.5 text-sm font-medium">{value}</div>
    </div>
  )
}

function peopleEmails(resources?: Record<string, Record<string, unknown>[]>): string[] {
  if (!resources) return []
  const set = new Set<string>()
  for (const r of resources.employee ?? []) set.add(String(r.email))
  for (const r of resources.user ?? []) set.add(String(r.email ?? r.id))
  for (const r of resources.member ?? []) set.add(String(r.email))
  const domain = [...set].find((e) => e.includes('@'))?.split('@')[1]
  for (const r of resources.iam_user ?? []) if (!r.is_service_account && domain) set.add(`${String(r.id)}@${domain}`)
  return [...set].filter((e) => e.includes('@')).sort()
}

function PersonScenarios({ integrationId, scenarios, active, canWrite, resources }: { integrationId: string; scenarios: ScenarioInfo[]; active: string[]; canWrite: boolean; resources?: Record<string, Record<string, unknown>[]> }) {
  const set = useSetScenario(integrationId)
  const [email, setEmail] = useState('')
  const emails = peopleEmails(resources)
  const applied = active.filter((a) => a.includes(':'))
  return (
    <div className="space-y-3 py-3" data-testid="person-scenarios">
      <div className="text-sm font-medium">Per-person changes</div>
      <div className="flex flex-wrap items-center gap-2">
        <Select value={email} onChange={(e) => setEmail(e.target.value)} className="w-64" data-testid="person-scenario-email" disabled={!canWrite}>
          <option value="">Choose a person…</option>
          {emails.map((e) => (
            <option key={e} value={e}>
              {e}
            </option>
          ))}
        </Select>
        {scenarios.map((s) => (
          <Button
            key={s.key}
            size="sm"
            variant="outline"
            disabled={!email || !canWrite || active.includes(`${s.key}:${email}`)}
            data-testid={`person-scenario-${s.key}`}
            title={s.description}
            onClick={() => set.mutate({ key: `${s.key}:${email}`, active: true }, { onSuccess: () => toast.success(`${s.label}: ${email} — applies on next sync`), onError: (e) => toast.error(errorMessage(e)) })}
          >
            {s.label}
          </Button>
        ))}
      </div>
      {applied.length ? (
        <ul className="space-y-1 text-sm">
          {applied.map((a) => {
            const [key, who] = a.split(':')
            const label = scenarios.find((s) => s.key === key)?.label ?? key
            return (
              <li key={a} className="flex items-center justify-between rounded border px-2 py-1" data-testid={`applied-${a}`}>
                <span>
                  {label}: <span className="font-medium">{who}</span>
                </span>
                {canWrite ? (
                  <Button size="sm" variant="ghost" onClick={() => set.mutate({ key: a, active: false }, { onSuccess: () => toast.success('Reverted — applies on next sync') })}>
                    Undo
                  </Button>
                ) : null}
              </li>
            )
          })}
        </ul>
      ) : null}
    </div>
  )
}
