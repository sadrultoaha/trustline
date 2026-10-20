import { useQuery } from '@tanstack/react-query'
import { Copy, KeyRound, Trash2 } from 'lucide-react'
import { toast } from 'sonner'
import { usePermissions } from '@/api/auth'
import { deviceTokenQuery, devicesQuery, useDeleteDevice, useRotateDeviceToken } from '@/api/phase2'
import { PageHeader } from '@/components/layout/PageHeader'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { timeAgo } from '@/lib/utils'

export function DevicesPage() {
  const { data } = useQuery(devicesQuery())
  const { can } = usePermissions()
  const del = useDeleteDevice()
  return (
    <div className="space-y-6">
      <PageHeader title="Devices" description="Laptops reporting through the Trustline agent: disk encryption, screen lock and anti-malware." />
      {can('org:write') ? <Enrollment /> : null}
      <Card>
        <CardContent className="p-0">
          <Table data-testid="devices-table">
            <TableHeader>
              <TableRow>
                <TableHead>Device</TableHead>
                <TableHead>Owner</TableHead>
                <TableHead>Encryption</TableHead>
                <TableHead>Screen lock</TableHead>
                <TableHead>Anti-malware</TableHead>
                <TableHead>Last seen</TableHead>
                <TableHead>Status</TableHead>
                <TableHead />
              </TableRow>
            </TableHeader>
            <TableBody>
              {data?.items.map((d) => (
                <TableRow key={d.id} data-testid={`device-row-${d.serial}`}>
                  <TableCell>
                    <div className="font-medium">{d.hostname}</div>
                    <div className="text-xs text-muted-foreground">
                      {d.platform} {d.os_version} · {d.serial}
                    </div>
                  </TableCell>
                  <TableCell className="text-sm">{d.owner_name || d.owner_email}</TableCell>
                  <TableCell>{d.disk_encrypted ? <Badge variant="success">On</Badge> : <Badge variant="destructive">Off</Badge>}</TableCell>
                  <TableCell className="text-sm">{d.screen_lock_minutes != null ? `${d.screen_lock_minutes} min` : '—'}</TableCell>
                  <TableCell className="text-sm">{d.antivirus_running ? d.antivirus_name || 'Running' : <span className="text-destructive">Not running</span>}</TableCell>
                  <TableCell className="text-sm text-muted-foreground">{timeAgo(d.last_seen_at)}</TableCell>
                  <TableCell>{d.compliant ? <Badge variant="success">Compliant</Badge> : <Badge variant="destructive" title={d.issues.join(', ')}>{d.issues.length} issue(s)</Badge>}</TableCell>
                  <TableCell>
                    {can('people:write') ? (
                      <button className="text-muted-foreground hover:text-destructive" title="Remove device" onClick={() => del.mutate(d.id, { onSuccess: () => toast.success('Device removed') })}>
                        <Trash2 className="h-4 w-4" />
                      </button>
                    ) : null}
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </CardContent>
      </Card>
    </div>
  )
}

function Enrollment() {
  const { data } = useQuery(deviceTokenQuery())
  const rotate = useRotateDeviceToken()
  const token = rotate.data?.token
  const cmd = `curl -fsSLO ${window.location.origin}/agent/trustline-agent.sh && TRUSTLINE_URL=${window.location.origin} TRUSTLINE_TOKEN=${token ?? '<token>'} OWNER_EMAIL=you@company.com bash trustline-agent.sh`
  return (
    <Card>
      <CardHeader className="flex-row items-start justify-between space-y-0">
        <div>
          <CardTitle className="flex items-center gap-2">
            <KeyRound className="h-4 w-4" /> Agent enrollment
          </CardTitle>
          <CardDescription>
            {data?.exists ? `Active token ${data.hint}${data.last_used_at ? ` · last used ${timeAgo(data.last_used_at)}` : ''}` : 'No enrollment token yet.'}
          </CardDescription>
        </div>
        <Button variant={data?.exists ? 'outline' : 'default'} loading={rotate.isPending} onClick={() => rotate.mutate()} data-testid="device-token-rotate">
          {data?.exists ? 'Rotate token' : 'Create token'}
        </Button>
      </CardHeader>
      <CardContent className="space-y-3">
        {token ? (
          <Alert>
            <AlertDescription>
              Copy this token now — it is shown only once:
              <code className="mt-1 block break-all rounded bg-muted p-2 text-xs" data-testid="device-token-value">
                {token}
              </code>
            </AlertDescription>
          </Alert>
        ) : null}
        <div>
          <div className="mb-1 flex items-center justify-between text-sm font-medium">
            Install on macOS or Linux
            <Button
              size="sm"
              variant="ghost"
              onClick={() => {
                void navigator.clipboard?.writeText(cmd)
                toast.success('Copied')
              }}
            >
              <Copy /> Copy
            </Button>
          </div>
          <pre className="overflow-auto rounded-md bg-[#0f172a] p-3 text-xs text-slate-100">{cmd}</pre>
          <p className="mt-1 text-xs text-muted-foreground">Schedule it with launchd or cron to report every few hours.</p>
        </div>
      </CardContent>
    </Card>
  )
}
