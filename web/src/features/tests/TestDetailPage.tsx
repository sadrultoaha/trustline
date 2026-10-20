import { useState } from 'react'
import { Link, useParams } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import { Play } from 'lucide-react'
import { toast } from 'sonner'
import { usePermissions } from '@/api/auth'
import { errorMessage } from '@/api/client'
import { runQuery, testQuery, useRunTest, useSetTestEnabled } from '@/api/compliance'
import { PageHeader } from '@/components/layout/PageHeader'
import { ConnectorLogo, SeverityBadge, TestStatusBadge } from '@/components/status'
import { connectorName } from '@/lib/connectors'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Switch } from '@/components/ui/switch'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { formatDate } from '@/lib/utils'
import { describeRule } from './describeRule'

export function TestDetailPage() {
  const { id } = useParams({ from: '/app/tests/$id' })
  const { data: t } = useQuery(testQuery(id))
  const { can } = usePermissions()
  const run = useRunTest()
  const setEnabled = useSetTestEnabled()
  const [runID, setRunID] = useState<string | null>(null)
  if (!t) return <p className="text-sm text-muted-foreground">Loading…</p>
  const latest = t.runs[0]
  return (
    <div className="space-y-6">
      <PageHeader
        title={t.name}
        description={t.description}
        actions={
          can('tests:write') ? (
            <>
              <label className="flex items-center gap-2 text-sm">
                <Switch checked={t.enabled} onCheckedChange={(v) => setEnabled.mutate({ id, enabled: v }, { onSuccess: () => toast.success(v ? 'Test enabled' : 'Test disabled') })} />
                Enabled
              </label>
              <Button loading={run.isPending} onClick={() => run.mutate(id, { onSuccess: (r) => toast.success(`Result: ${r.last_status}`), onError: (e) => toast.error(errorMessage(e)) })} data-testid="test-run-now">
                <Play /> Run now
              </Button>
            </>
          ) : null
        }
      />
      <Card>
        <CardContent className="flex flex-wrap items-center gap-6 p-5">
          <ConnectorLogo type={t.integration_type} />
          <div>
            <div className="text-xs uppercase tracking-wide text-muted-foreground">Integration</div>
            <div className="text-sm font-medium">{connectorName(t.integration_type)}</div>
          </div>
          <div>
            <div className="text-xs uppercase tracking-wide text-muted-foreground">Control</div>
            <Link to="/app/controls/$id" params={{ id: t.control_id }} className="text-sm font-medium hover:underline">
              {t.control_code} · {t.control_name}
            </Link>
          </div>
          <div>
            <div className="text-xs uppercase tracking-wide text-muted-foreground">Severity</div>
            <SeverityBadge severity={t.severity} />
          </div>
          <div>
            <div className="text-xs uppercase tracking-wide text-muted-foreground">Result</div>
            <TestStatusBadge status={t.last_status} />
          </div>
          {t.custom ? <Badge variant="outline">Custom test</Badge> : <Badge variant="outline">{t.catalog_id}</Badge>}
        </CardContent>
      </Card>

      {latest && latest.failing.length ? (
        <Card className="border-destructive/40">
          <CardHeader>
            <CardTitle>Failing resources</CardTitle>
            <CardDescription>{latest.summary}</CardDescription>
          </CardHeader>
          <CardContent>
            <ul className="space-y-1 text-sm" data-testid="test-failing">
              {latest.failing.map((f) => (
                <li key={f.id + f.reason}>
                  <code className="font-semibold">{f.id}</code> <span className="text-muted-foreground">— {f.reason}</span>
                </li>
              ))}
            </ul>
          </CardContent>
        </Card>
      ) : null}

      <div className="grid gap-6 lg:grid-cols-2">
        <Card>
          <CardHeader>
            <CardTitle>Rule</CardTitle>
            <CardDescription>{describeRule(t.rule)}</CardDescription>
          </CardHeader>
          <CardContent>
            <pre className="max-h-80 overflow-auto rounded-md bg-muted/50 p-3 text-xs">{JSON.stringify(t.rule, null, 2)}</pre>
          </CardContent>
        </Card>
        <Card>
          <CardHeader>
            <CardTitle>How to fix</CardTitle>
            <CardDescription>{t.remediation.summary}</CardDescription>
          </CardHeader>
          <CardContent>
            <ol className="list-decimal space-y-1 pl-5 text-sm">
              {t.remediation.steps.map((s) => (
                <li key={s}>{s}</li>
              ))}
            </ol>
          </CardContent>
        </Card>
      </div>

      <Card>
        <CardHeader>
          <CardTitle>Run history</CardTitle>
          <CardDescription>Each run stores a frozen snapshot of the evaluated resources as evidence.</CardDescription>
        </CardHeader>
        <CardContent className="p-0">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>When</TableHead>
                <TableHead>Result</TableHead>
                <TableHead>Summary</TableHead>
                <TableHead>Trigger</TableHead>
                <TableHead />
              </TableRow>
            </TableHeader>
            <TableBody>
              {t.runs.map((r) => (
                <TableRow key={r.id}>
                  <TableCell className="text-sm">{formatDate(r.created_at, true)}</TableCell>
                  <TableCell>
                    <TestStatusBadge status={r.status} />
                  </TableCell>
                  <TableCell className="text-sm">{r.error || r.summary}</TableCell>
                  <TableCell className="text-sm capitalize text-muted-foreground">{r.trigger}</TableCell>
                  <TableCell>
                    <Button size="sm" variant="ghost" onClick={() => setRunID(r.id)} data-testid="run-evidence">
                      Evidence
                    </Button>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </CardContent>
      </Card>
      {runID ? <RunDialog id={runID} onClose={() => setRunID(null)} /> : null}
    </div>
  )
}

function RunDialog({ id, onClose }: { id: string; onClose: () => void }) {
  const { data: r } = useQuery(runQuery(id))
  return (
    <Dialog open onOpenChange={(o) => !o && onClose()}>
      <DialogContent className="max-w-3xl">
        <DialogHeader>
          <DialogTitle>Run evidence</DialogTitle>
          <DialogDescription>
            {r ? (
              <>
                {r.test_name} · {r.summary} · snapshot {r.snapshot_taken_at ? formatDate(r.snapshot_taken_at, true) : 'unavailable'}
                {r.snapshot_sha256 ? <> · SHA-256 <code>{r.snapshot_sha256.slice(0, 16)}…</code></> : null}
              </>
            ) : (
              'Loading…'
            )}
          </DialogDescription>
        </DialogHeader>
        {r ? <pre className="max-h-[60vh] overflow-auto rounded-md bg-muted/50 p-3 text-xs" data-testid="run-evidence-json">{JSON.stringify({ resource: r.resource, failing: r.failing, resources: r.resources }, null, 2)}</pre> : null}
      </DialogContent>
    </Dialog>
  )
}
