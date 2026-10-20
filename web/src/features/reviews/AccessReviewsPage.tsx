import { useState } from 'react'
import { Link, useNavigate } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import { Plus } from 'lucide-react'
import { toast } from 'sonner'
import { usePermissions } from '@/api/auth'
import { errorMessage, isApiError } from '@/api/client'
import { integrationsQuery } from '@/api/compliance'
import { campaignsQuery, useCreateCampaign } from '@/api/phase2'
import { FormField } from '@/components/layout/FormField'
import { PageHeader } from '@/components/layout/PageHeader'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Progress } from '@/components/ui/progress'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { connectorName } from '@/lib/connectors'
import { formatDate } from '@/lib/utils'

export function AccessReviewsPage() {
  const { data } = useQuery(campaignsQuery())
  const { can } = usePermissions()
  const [open, setOpen] = useState(false)
  return (
    <div>
      <PageHeader
        title="Access reviews"
        description="Periodic reviews of who has access to what. Reviewers approve or revoke; revocations are verified on the next sync."
        actions={
          can('controls:write') ? (
            <Button onClick={() => setOpen(true)} data-testid="campaign-new">
              <Plus /> New review
            </Button>
          ) : null
        }
      />
      <Card>
        <CardContent className="p-0">
          <Table data-testid="campaigns-table">
            <TableHeader>
              <TableRow>
                <TableHead>Review</TableHead>
                <TableHead>Systems</TableHead>
                <TableHead>Progress</TableHead>
                <TableHead>Due</TableHead>
                <TableHead>Status</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {data?.items.map((c) => (
                <TableRow key={c.id}>
                  <TableCell>
                    <Link to="/app/access-reviews/$id" params={{ id: c.id }} className="font-medium hover:underline">
                      {c.name}
                    </Link>
                    <div className="text-xs text-muted-foreground">started {formatDate(c.created_at)}</div>
                  </TableCell>
                  <TableCell className="text-sm">{c.systems.map(connectorName).join(', ')}</TableCell>
                  <TableCell className="w-48">
                    <div className="text-xs tabular-nums">
                      {c.decided}/{c.total} decided · {c.revoked} revoked
                    </div>
                    <Progress value={c.total ? (c.decided / c.total) * 100 : 0} />
                  </TableCell>
                  <TableCell className="text-sm">
                    {formatDate(c.due_at)} {c.overdue ? <Badge variant="destructive">overdue</Badge> : null}
                  </TableCell>
                  <TableCell>
                    <Badge variant={c.status === 'completed' ? 'success' : 'warning'} className="capitalize">
                      {c.status}
                    </Badge>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </CardContent>
      </Card>
      {open ? <NewCampaign onClose={() => setOpen(false)} /> : null}
    </div>
  )
}

function NewCampaign({ onClose }: { onClose: () => void }) {
  const integrations = useQuery(integrationsQuery())
  const reviewable = (integrations.data?.items ?? []).filter((i) => ['okta', 'google_workspace', 'github', 'aws'].includes(i.type))
  const [name, setName] = useState(() => `Access review ${new Date().toISOString().slice(0, 10)}`)
  const [systems, setSystems] = useState<string[]>([])
  const [due, setDue] = useState(14)
  const [err, setErr] = useState('')
  const create = useCreateCampaign()
  const navigate = useNavigate()
  return (
    <Dialog open onOpenChange={(o) => !o && onClose()}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>New access review</DialogTitle>
          <DialogDescription>Accounts are pulled from the latest sync and assigned to each person's manager.</DialogDescription>
        </DialogHeader>
        <FormField label="Name" htmlFor="cn">
          <Input id="cn" value={name} onChange={(e) => setName(e.target.value)} />
        </FormField>
        <div className="space-y-1.5">
          <span className="text-sm font-medium">Systems</span>
          {reviewable.length === 0 ? <p className="text-sm text-muted-foreground">Connect Okta, Google Workspace, GitHub or AWS first.</p> : null}
          {reviewable.map((i) => (
            <label key={i.type} className="flex items-center gap-2 text-sm">
              <input
                type="checkbox"
                checked={systems.includes(i.type)}
                onChange={(e) => setSystems(e.target.checked ? [...systems, i.type] : systems.filter((s) => s !== i.type))}
                data-testid={`campaign-system-${i.type}`}
              />
              {connectorName(i.type)} <span className="text-muted-foreground">({i.resource_count} resources)</span>
            </label>
          ))}
        </div>
        <FormField label="Due in (days)" htmlFor="cd">
          <Input id="cd" type="number" min={1} max={90} value={due} onChange={(e) => setDue(Number(e.target.value))} />
        </FormField>
        {err ? (
          <Alert variant="destructive">
            <AlertDescription>{err}</AlertDescription>
          </Alert>
        ) : null}
        <DialogFooter>
          <Button
            disabled={!systems.length}
            loading={create.isPending}
            data-testid="campaign-create"
            onClick={() =>
              create.mutate(
                { name, systems, due_days: due },
                {
                  onSuccess: (c) => {
                    toast.success(`${c.total} accounts to review`)
                    onClose()
                    void navigate({ to: '/app/access-reviews/$id', params: { id: c.id } })
                  },
                  onError: (e) => setErr(isApiError(e) ? Object.values(e.fields)[0] ?? e.message : errorMessage(e)),
                },
              )
            }
          >
            Start review
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
