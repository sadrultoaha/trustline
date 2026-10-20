import { useState } from 'react'
import { Link } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import { tasksQuery } from '@/api/compliance'
import { PageHeader } from '@/components/layout/PageHeader'
import { SeverityBadge } from '@/components/status'
import { Badge } from '@/components/ui/badge'
import { Card, CardContent } from '@/components/ui/card'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { formatDate, timeAgo } from '@/lib/utils'

export function TasksPage() {
  const [status, setStatus] = useState('open')
  const [mine, setMine] = useState(false)
  const { data } = useQuery(tasksQuery({ status: status === 'all' ? undefined : status, mine: mine ? 'true' : undefined }))
  return (
    <div>
      <PageHeader title="Tasks" description="Remediation work created automatically when tests fail. Tasks close themselves when the test passes again." />
      <div className="mb-4 flex items-center gap-3">
        <Tabs value={status} onValueChange={setStatus}>
          <TabsList>
            <TabsTrigger value="open">Open</TabsTrigger>
            <TabsTrigger value="done">Done</TabsTrigger>
            <TabsTrigger value="dismissed">Dismissed</TabsTrigger>
            <TabsTrigger value="all">All</TabsTrigger>
          </TabsList>
        </Tabs>
        <label className="flex items-center gap-2 text-sm text-muted-foreground">
          <input type="checkbox" checked={mine} onChange={(e) => setMine(e.target.checked)} /> Assigned to me
        </label>
      </div>
      <Card>
        <CardContent className="p-0">
          <Table data-testid="tasks-table">
            <TableHeader>
              <TableRow>
                <TableHead>Task</TableHead>
                <TableHead>Control</TableHead>
                <TableHead>Priority</TableHead>
                <TableHead>Assignee</TableHead>
                <TableHead>Due</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {data?.items.map((t) => (
                <TableRow key={t.id} data-testid={`task-row-${t.control_code}`}>
                  <TableCell>
                    <Link to="/app/tasks/$id" params={{ id: t.id }} className="font-medium hover:underline">
                      {t.title}
                    </Link>
                    <div className="text-xs text-muted-foreground">
                      created {timeAgo(t.created_at)} {t.status !== 'open' ? `· ${t.status}` : ''}
                    </div>
                  </TableCell>
                  <TableCell>
                    <code className="text-xs">{t.control_code}</code>
                  </TableCell>
                  <TableCell>
                    <SeverityBadge severity={t.priority} />
                  </TableCell>
                  <TableCell className="text-sm">{t.assignee?.name ?? <span className="text-muted-foreground">Unassigned</span>}</TableCell>
                  <TableCell className="text-sm">
                    {t.due_at ? formatDate(t.due_at) : '—'} {t.overdue ? <Badge variant="destructive">overdue</Badge> : null}
                  </TableCell>
                </TableRow>
              ))}
              {data && data.items.length === 0 ? (
                <TableRow>
                  <TableCell colSpan={5} className="py-10 text-center text-sm text-muted-foreground">
                    Nothing here.
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
