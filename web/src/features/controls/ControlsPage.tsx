import { useState } from 'react'
import { Link } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import { controlsQuery, frameworksQuery } from '@/api/compliance'
import { PageHeader } from '@/components/layout/PageHeader'
import { ControlStatusBadge } from '@/components/status'
import { Badge } from '@/components/ui/badge'
import { Card, CardContent } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Select } from '@/components/ui/select'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'

export function ControlsPage() {
  const [status, setStatus] = useState('')
  const [framework, setFramework] = useState('')
  const [q, setQ] = useState('')
  const { data } = useQuery(controlsQuery({ status, framework, q }))
  const fws = useQuery(frameworksQuery())
  const adopted = fws.data?.items.filter((f) => f.adopted) ?? []
  const names = Object.fromEntries((fws.data?.items ?? []).map((f) => [f.id, f.name]))
  return (
    <div>
      <PageHeader title="Controls" description="Every control in scope, its status, and what evidences it." />
      <div className="mb-4 flex flex-wrap items-center gap-3">
        <Tabs value={status || 'all'} onValueChange={(v) => setStatus(v === 'all' ? '' : v)}>
          <TabsList>
            <TabsTrigger value="all">All</TabsTrigger>
            <TabsTrigger value="failing" data-testid="controls-tab-failing">
              Failing
            </TabsTrigger>
            <TabsTrigger value="needs_evidence">Needs evidence</TabsTrigger>
            <TabsTrigger value="passing">Passing</TabsTrigger>
          </TabsList>
        </Tabs>
        <Select value={framework} onChange={(e) => setFramework(e.target.value)} className="w-48" data-testid="controls-framework">
          <option value="">All frameworks</option>
          {adopted.map((f) => (
            <option key={f.id} value={f.id}>
              {f.name}
            </option>
          ))}
        </Select>
        <Input placeholder="Search code, name or domain" value={q} onChange={(e) => setQ(e.target.value)} className="w-64" data-testid="controls-search" />
        <span className="ml-auto text-sm text-muted-foreground">{data?.items.length ?? 0} controls</span>
      </div>
      <Card>
        <CardContent className="p-0">
          <Table data-testid="controls-table">
            <TableHeader>
              <TableRow>
                <TableHead className="w-24">Code</TableHead>
                <TableHead>Control</TableHead>
                <TableHead>Status</TableHead>
                <TableHead>Frameworks</TableHead>
                <TableHead>Tests</TableHead>
                <TableHead>Evidence</TableHead>
                <TableHead>Owner</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {data?.items.map((c) => (
                <TableRow key={c.id} data-testid={`control-row-${c.code}`}>
                  <TableCell>
                    <code className="text-xs font-semibold">{c.code}</code>
                  </TableCell>
                  <TableCell>
                    <Link to="/app/controls/$id" params={{ id: c.id }} className="font-medium hover:underline">
                      {c.name}
                    </Link>
                    <div className="text-xs text-muted-foreground">{c.domain}</div>
                  </TableCell>
                  <TableCell>
                    <ControlStatusBadge status={c.status} />
                  </TableCell>
                  <TableCell>
                    <div className="flex flex-wrap gap-1">
                      {c.frameworks.map((f) => (
                        <Badge key={f} variant="outline" className="font-normal">
                          {names[f] ?? f}
                        </Badge>
                      ))}
                    </div>
                  </TableCell>
                  <TableCell className="tabular-nums text-sm">
                    {c.tests_total ? (
                      <span className={c.tests_failing ? 'text-destructive' : ''}>
                        {c.tests_total - c.tests_failing}/{c.tests_total}
                      </span>
                    ) : (
                      <span className="text-muted-foreground">—</span>
                    )}
                  </TableCell>
                  <TableCell className="tabular-nums text-sm">{c.evidence_valid || <span className="text-muted-foreground">—</span>}</TableCell>
                  <TableCell className="text-sm">{c.owner?.name ?? <span className="text-muted-foreground">Unassigned</span>}</TableCell>
                </TableRow>
              ))}
              {data && data.items.length === 0 ? (
                <TableRow>
                  <TableCell colSpan={7} className="py-10 text-center text-sm text-muted-foreground">
                    No controls match. Add a framework to bring controls into scope.
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
