import { useState } from 'react'
import { Link } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import { Plus } from 'lucide-react'
import { usePermissions } from '@/api/auth'
import { testsQuery } from '@/api/compliance'
import { PageHeader } from '@/components/layout/PageHeader'
import { ConnectorLogo, SeverityBadge, TestStatusBadge } from '@/components/status'
import { connectorName } from '@/lib/connectors'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Select } from '@/components/ui/select'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { timeAgo } from '@/lib/utils'

export function TestsPage() {
  const [status, setStatus] = useState('')
  const [integration, setIntegration] = useState('')
  const [q, setQ] = useState('')
  const { data } = useQuery(testsQuery({ status, integration, q }))
  const { can } = usePermissions()
  return (
    <div>
      <PageHeader
        title="Tests"
        description="Automated checks evaluated against every integration snapshot."
        actions={
          can('tests:write') ? (
            <Button asChild>
              <Link to="/app/tests/new" data-testid="test-new">
                <Plus /> Custom test
              </Link>
            </Button>
          ) : null
        }
      />
      <div className="mb-4 flex flex-wrap items-center gap-3">
        <Tabs value={status || 'all'} onValueChange={(v) => setStatus(v === 'all' ? '' : v)}>
          <TabsList>
            <TabsTrigger value="all">All</TabsTrigger>
            <TabsTrigger value="fail" data-testid="tests-tab-fail">
              Failing
            </TabsTrigger>
            <TabsTrigger value="pass">Passing</TabsTrigger>
            <TabsTrigger value="pending">Pending</TabsTrigger>
          </TabsList>
        </Tabs>
        <Select value={integration} onChange={(e) => setIntegration(e.target.value)} className="w-48">
          <option value="">All integrations</option>
          {['aws', 'gcp', 'github', 'okta', 'google_workspace', 'hris'].map((t) => (
            <option key={t} value={t}>
              {connectorName(t)}
            </option>
          ))}
        </Select>
        <Input placeholder="Search tests" value={q} onChange={(e) => setQ(e.target.value)} className="w-64" />
        <span className="ml-auto text-sm text-muted-foreground">{data?.items.length ?? 0} tests</span>
      </div>
      <Card>
        <CardContent className="p-0">
          <Table data-testid="tests-table">
            <TableHeader>
              <TableRow>
                <TableHead>Test</TableHead>
                <TableHead>Control</TableHead>
                <TableHead>Severity</TableHead>
                <TableHead>Result</TableHead>
                <TableHead>Last run</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {data?.items.map((t) => (
                <TableRow key={t.id} data-testid={`test-row-${t.catalog_id ?? t.id}`}>
                  <TableCell>
                    <div className="flex items-center gap-3">
                      <ConnectorLogo type={t.integration_type} className="h-8 w-8 text-[10px]" />
                      <div>
                        <Link to="/app/tests/$id" params={{ id: t.id }} className="font-medium hover:underline">
                          {t.name}
                        </Link>
                        {t.custom ? (
                          <Badge variant="outline" className="ml-2">
                            custom
                          </Badge>
                        ) : null}
                        <div className="text-xs text-muted-foreground">{t.last_summary}</div>
                      </div>
                    </div>
                  </TableCell>
                  <TableCell>
                    <Link to="/app/controls/$id" params={{ id: t.control_id }} className="text-sm hover:underline">
                      <code className="text-xs">{t.control_code}</code>
                    </Link>
                  </TableCell>
                  <TableCell>
                    <SeverityBadge severity={t.severity} />
                  </TableCell>
                  <TableCell>
                    <TestStatusBadge status={t.last_status} />
                  </TableCell>
                  <TableCell className="text-sm text-muted-foreground">{t.last_run_at ? timeAgo(t.last_run_at) : '—'}</TableCell>
                </TableRow>
              ))}
              {data && data.items.length === 0 ? (
                <TableRow>
                  <TableCell colSpan={5} className="py-10 text-center text-sm text-muted-foreground">
                    No tests. Connect an integration to activate automated tests.
                  </TableCell>
                </TableRow>
              ) : null}
            </TableBody>
          </Table>
        </CardContent>
      </Card>
    </div>
  )
}
