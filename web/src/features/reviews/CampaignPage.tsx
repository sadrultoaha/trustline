import { useState } from 'react'
import { Link, useParams } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import { Check, Download, X } from 'lucide-react'
import { toast } from 'sonner'
import { usePermissions } from '@/api/auth'
import { errorMessage } from '@/api/client'
import { campaignQuery, useCompleteCampaign, useDecideItem, type ReviewItem } from '@/api/phase2'
import { PageHeader } from '@/components/layout/PageHeader'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { Progress } from '@/components/ui/progress'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { connectorName } from '@/lib/connectors'
import { formatDate } from '@/lib/utils'

export function CampaignPage() {
  const { id } = useParams({ from: '/app/access-reviews/$id' })
  const { data: c } = useQuery(campaignQuery(id))
  const { can } = usePermissions()
  const complete = useCompleteCampaign()
  const decide = useDecideItem()
  const [filter, setFilter] = useState('all')
  const [bulk, setBulk] = useState(false)
  if (!c) return <p className="text-sm text-muted-foreground">Loading…</p>
  const items = c.items.filter((i) => filter === 'all' || i.decision === filter || (filter === 'flagged' && (i.person_status === 'terminated' || i.is_admin || !i.person_id)))
  return (
    <div className="space-y-6">
      <PageHeader
        title={c.name}
        description={`${c.systems.map(connectorName).join(', ')} · due ${formatDate(c.due_at)} · created by ${c.created_by || 'scheduler'}`}
        actions={
          <>
            <Button variant="outline" asChild>
              <a href={`/api/v1/access-reviews/${id}/export.csv`} data-testid="campaign-export">
                <Download /> Export CSV
              </a>
            </Button>
            {can('controls:write') && c.status === 'open' && c.decided < c.total ? (
              <Button
                variant="outline"
                loading={bulk}
                data-testid="campaign-keep-rest"
                onClick={async () => {
                  setBulk(true)
                  try {
                    for (const it of c.items.filter((x) => x.decision === 'pending')) {
                      await decide.mutateAsync({ id: it.id, decision: 'approve', comment: 'Access confirmed in bulk' })
                    }
                    toast.success('Remaining accounts kept')
                  } catch (e) {
                    toast.error(errorMessage(e))
                  } finally {
                    setBulk(false)
                  }
                }}
              >
                <Check /> Keep all remaining
              </Button>
            ) : null}
            {can('controls:write') && c.status === 'open' ? (
              <Button
                disabled={c.decided < c.total}
                loading={complete.isPending}
                data-testid="campaign-complete"
                onClick={() => complete.mutate(id, { onSuccess: () => toast.success('Review completed — evidence filed on IAM-04'), onError: (e) => toast.error(errorMessage(e)) })}
              >
                Complete review
              </Button>
            ) : null}
          </>
        }
      />
      <Card>
        <CardContent className="flex flex-wrap items-center gap-6 p-5 text-sm">
          <Badge variant={c.status === 'completed' ? 'success' : 'warning'} className="capitalize" data-testid="campaign-status">
            {c.status}
          </Badge>
          <div className="w-64">
            <div className="text-xs tabular-nums">
              {c.decided}/{c.total} decided · {c.revoked} revoked
            </div>
            <Progress value={c.total ? (c.decided / c.total) * 100 : 0} />
          </div>
          {c.evidence_id ? <span className="text-muted-foreground">Evidence filed on control IAM-04.</span> : null}
        </CardContent>
      </Card>
      <Tabs value={filter} onValueChange={setFilter}>
        <TabsList>
          <TabsTrigger value="all">All ({c.items.length})</TabsTrigger>
          <TabsTrigger value="pending">Pending</TabsTrigger>
          <TabsTrigger value="flagged">Flagged</TabsTrigger>
          <TabsTrigger value="revoke">Revoked</TabsTrigger>
        </TabsList>
      </Tabs>
      <Card>
        <CardContent className="p-0">
          <ReviewTable items={items} open={c.status === 'open'} />
        </CardContent>
      </Card>
    </div>
  )
}

export function ReviewTable({ items, open, showCampaign }: { items: ReviewItem[]; open: boolean; showCampaign?: boolean }) {
  const decide = useDecideItem()
  return (
    <Table data-testid="review-items">
      <TableHeader>
        <TableRow>
          <TableHead>Account</TableHead>
          <TableHead>System</TableHead>
          <TableHead>Access</TableHead>
          <TableHead>Reviewer</TableHead>
          <TableHead>Decision</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {items.map((i) => (
          <TableRow key={i.id} data-testid={`review-item-${i.integration_type}-${i.email || i.account_id}`}>
            <TableCell>
              <div className="font-medium">{i.account_label}</div>
              <div className="text-xs text-muted-foreground">
                {i.email || i.account_id}
                {showCampaign ? ` · ${i.campaign_name}` : ''}
              </div>
              <div className="mt-1 flex gap-1">
                {i.person_status === 'terminated' ? <Badge variant="destructive">Terminated employee</Badge> : null}
                {!i.person_id ? <Badge variant="outline">No HR record</Badge> : null}
              </div>
            </TableCell>
            <TableCell className="text-sm">{connectorName(i.integration_type)}</TableCell>
            <TableCell>
              <div className="flex max-w-xs flex-wrap gap-1">
                {i.is_admin ? <Badge variant="warning">admin</Badge> : null}
                {i.entitlements.map((e) => (
                  <Badge key={e} variant="outline" className="font-normal">
                    {e}
                  </Badge>
                ))}
              </div>
            </TableCell>
            <TableCell className="text-sm">{i.reviewer?.name ?? '—'}</TableCell>
            <TableCell>
              {open ? (
                <div className="flex items-center gap-1">
                  <Button
                    size="sm"
                    variant={i.decision === 'approve' ? 'default' : 'outline'}
                    data-testid="decide-approve"
                    onClick={() => decide.mutate({ id: i.id, decision: 'approve' }, { onError: (e) => toast.error(errorMessage(e)) })}
                  >
                    <Check /> Keep
                  </Button>
                  <Button
                    size="sm"
                    variant={i.decision === 'revoke' ? 'destructive' : 'outline'}
                    data-testid="decide-revoke"
                    onClick={() => decide.mutate({ id: i.id, decision: 'revoke', comment: 'Access no longer required' }, { onSuccess: () => toast.success('Revocation task created'), onError: (e) => toast.error(errorMessage(e)) })}
                  >
                    <X /> Revoke
                  </Button>
                </div>
              ) : (
                <Badge variant={i.decision === 'revoke' ? 'destructive' : 'success'} className="capitalize">
                  {i.decision}
                </Badge>
              )}
              {i.revocation_task_id ? (
                <Link to="/app/tasks/$id" params={{ id: i.revocation_task_id }} className="mt-1 block text-xs underline">
                  revocation task
                </Link>
              ) : null}
            </TableCell>
          </TableRow>
        ))}
      </TableBody>
    </Table>
  )
}
