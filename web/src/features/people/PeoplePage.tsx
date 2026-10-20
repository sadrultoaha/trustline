import { useState } from 'react'
import { Link } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import { Check, Minus, X } from 'lucide-react'
import { peopleQuery } from '@/api/phase2'
import { PageHeader } from '@/components/layout/PageHeader'
import { Badge } from '@/components/ui/badge'
import { Card, CardContent } from '@/components/ui/card'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { connectorName } from '@/lib/connectors'

export function Tick({ ok, na }: { ok: boolean; na?: boolean }) {
  if (na) return <Minus className="h-4 w-4 text-muted-foreground" />
  return ok ? <Check className="h-4 w-4 text-success" /> : <X className="h-4 w-4 text-destructive" />
}

export function PersonStatus({ status }: { status: string }) {
  const v = { active: 'success', on_leave: 'secondary', terminated: 'destructive' } as const
  return (
    <Badge variant={v[status as keyof typeof v] ?? 'outline'} className="capitalize" data-testid="person-status">
      {status.replace('_', ' ')}
    </Badge>
  )
}

export function PeoplePage() {
  const [status, setStatus] = useState('')
  const { data } = useQuery(peopleQuery(status || undefined))
  return (
    <div>
      <PageHeader title="People" description="Your workforce from the HR system: onboarding, training, devices and access across connected systems." />
      <Tabs value={status || 'all'} onValueChange={(v) => setStatus(v === 'all' ? '' : v)} className="mb-4">
        <TabsList>
          <TabsTrigger value="all">All</TabsTrigger>
          <TabsTrigger value="active">Active</TabsTrigger>
          <TabsTrigger value="terminated" data-testid="people-tab-terminated">
            Terminated
          </TabsTrigger>
        </TabsList>
      </Tabs>
      <Card>
        <CardContent className="p-0">
          <Table data-testid="people-table">
            <TableHeader>
              <TableRow>
                <TableHead>Person</TableHead>
                <TableHead>Status</TableHead>
                <TableHead title="Policies acknowledged">Policies</TableHead>
                <TableHead>Training</TableHead>
                <TableHead>Device</TableHead>
                <TableHead>Background</TableHead>
                <TableHead>Access</TableHead>
                <TableHead>Open tasks</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {data?.items.map((p) => (
                <TableRow key={p.id} data-testid={`person-row-${p.email}`}>
                  <TableCell>
                    <Link to="/app/people/$id" params={{ id: p.id }} className="font-medium hover:underline">
                      {p.name}
                    </Link>
                    <div className="text-xs text-muted-foreground">
                      {p.title} · {p.department}
                    </div>
                  </TableCell>
                  <TableCell>
                    <PersonStatus status={p.status} />
                  </TableCell>
                  <TableCell>
                    <Tick ok={p.policies_accepted} na={p.status === 'terminated'} />
                  </TableCell>
                  <TableCell>
                    <Tick ok={p.training_completed} na={p.status === 'terminated'} />
                  </TableCell>
                  <TableCell>
                    <Tick ok={p.device_enrolled && p.device_compliant} na={p.status === 'terminated'} />
                  </TableCell>
                  <TableCell className="text-xs capitalize">{p.background_check.replace('_', ' ')}</TableCell>
                  <TableCell>
                    <div className="flex flex-wrap gap-1">
                      {Object.entries(p.access)
                        .filter(([, v]) => v)
                        .map(([k]) => (
                          <Badge key={k} variant={p.status === 'terminated' ? 'destructive' : 'outline'} className="font-normal">
                            {connectorName(k)}
                          </Badge>
                        ))}
                    </div>
                  </TableCell>
                  <TableCell className="tabular-nums">{p.open_tasks || ''}</TableCell>
                </TableRow>
              ))}
              {data && data.items.length === 0 ? (
                <TableRow>
                  <TableCell colSpan={8} className="py-10 text-center text-sm text-muted-foreground">
                    No people yet. Connect your HR system under Integrations.
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
