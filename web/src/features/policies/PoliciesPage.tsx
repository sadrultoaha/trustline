import { useState } from 'react'
import { Link, useNavigate } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import { BookOpen, FilePlus2 } from 'lucide-react'
import { toast } from 'sonner'
import { usePermissions } from '@/api/auth'
import { errorMessage } from '@/api/client'
import { policiesQuery, policyLibraryQuery, useCreatePolicy } from '@/api/phase2'
import { PolicyStatusBadge } from './PolicyStatusBadge'
import { FormField } from '@/components/layout/FormField'
import { PageHeader } from '@/components/layout/PageHeader'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Progress } from '@/components/ui/progress'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { formatDate } from '@/lib/utils'

export function PoliciesPage() {
  const { data } = useQuery(policiesQuery())
  const { can } = usePermissions()
  const [libOpen, setLibOpen] = useState(false)
  const [blankOpen, setBlankOpen] = useState(false)
  return (
    <div>
      <PageHeader
        title="Policies"
        description="Versioned policies with approval, publication and staff acknowledgement."
        actions={
          can('policies:write') ? (
            <>
              <Button variant="outline" onClick={() => setBlankOpen(true)}>
                <FilePlus2 /> Blank policy
              </Button>
              <Button onClick={() => setLibOpen(true)} data-testid="policy-library-open">
                <BookOpen /> Add from library
              </Button>
            </>
          ) : null
        }
      />
      <Card>
        <CardContent className="p-0">
          <Table data-testid="policies-table">
            <TableHeader>
              <TableRow>
                <TableHead>Policy</TableHead>
                <TableHead>Status</TableHead>
                <TableHead>Acknowledged</TableHead>
                <TableHead>Owner</TableHead>
                <TableHead>Next review</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {data?.items.map((p) => (
                <TableRow key={p.id} data-testid={`policy-row-${p.slug}`}>
                  <TableCell>
                    <Link to="/app/policies/$id" params={{ id: p.id }} className="font-medium hover:underline">
                      {p.title}
                    </Link>
                    <div className="text-xs text-muted-foreground">{p.category}</div>
                  </TableCell>
                  <TableCell>
                    <PolicyStatusBadge p={p} />
                  </TableCell>
                  <TableCell className="w-48">
                    {p.published_version ? (
                      <div className="space-y-1" data-testid={`policy-ack-${p.slug}`}>
                        <span className="text-xs tabular-nums">
                          {p.accepted}/{p.audience} · {p.acceptance_rate}%
                        </span>
                        <Progress value={p.acceptance_rate} tone={p.acceptance_rate >= 90 ? 'success' : 'warning'} />
                      </div>
                    ) : (
                      <span className="text-xs text-muted-foreground">—</span>
                    )}
                  </TableCell>
                  <TableCell className="text-sm">{p.owner?.name ?? <span className="text-muted-foreground">Unassigned</span>}</TableCell>
                  <TableCell className="text-sm">
                    {p.next_review_at ? formatDate(p.next_review_at) : '—'} {p.review_overdue ? <Badge variant="destructive">overdue</Badge> : null}
                  </TableCell>
                </TableRow>
              ))}
              {data && data.items.length === 0 ? (
                <TableRow>
                  <TableCell colSpan={5} className="py-10 text-center text-sm text-muted-foreground">
                    No policies yet. Start from the library of {can('policies:write') ? '20+' : ''} templates.
                  </TableCell>
                </TableRow>
              ) : null}
            </TableBody>
          </Table>
        </CardContent>
      </Card>
      {libOpen ? <LibraryDialog onClose={() => setLibOpen(false)} /> : null}
      {blankOpen ? <BlankDialog onClose={() => setBlankOpen(false)} /> : null}
    </div>
  )
}

function LibraryDialog({ onClose }: { onClose: () => void }) {
  const { data } = useQuery(policyLibraryQuery())
  const create = useCreatePolicy()
  const navigate = useNavigate()
  return (
    <Dialog open onOpenChange={(o) => !o && onClose()}>
      <DialogContent className="max-w-4xl">
        <DialogHeader>
          <DialogTitle>Policy library</DialogTitle>
          <DialogDescription>Templates are filled in with your organization's settings; you can edit before publishing.</DialogDescription>
        </DialogHeader>
        <div className="grid max-h-[65vh] gap-3 overflow-auto md:grid-cols-2">
          {data?.items.map((t) => (
            <div key={t.id} className="flex flex-col gap-2 rounded-lg border p-3" data-testid={`library-${t.id}`}>
              <div className="flex items-start justify-between gap-2">
                <div>
                  <div className="font-medium">{t.title}</div>
                  <div className="text-xs text-muted-foreground">
                    {t.category} · {t.words} words · {t.controls.join(', ')}
                  </div>
                </div>
                {t.policy_id ? (
                  <Button size="sm" variant="ghost" asChild>
                    <Link to="/app/policies/$id" params={{ id: t.policy_id }} onClick={onClose}>
                      Open
                    </Link>
                  </Button>
                ) : (
                  <Button
                    size="sm"
                    loading={create.isPending && create.variables?.template_id === t.id}
                    data-testid={`library-add-${t.id}`}
                    onClick={() =>
                      create.mutate(
                        { template_id: t.id },
                        {
                          onSuccess: (p) => {
                            toast.success(`${t.title} added as a draft`)
                            onClose()
                            void navigate({ to: '/app/policies/$id', params: { id: p.id } })
                          },
                          onError: (e) => toast.error(errorMessage(e)),
                        },
                      )
                    }
                  >
                    Add
                  </Button>
                )}
              </div>
              <p className="text-xs text-muted-foreground">{t.summary}</p>
            </div>
          ))}
        </div>
      </DialogContent>
    </Dialog>
  )
}

function BlankDialog({ onClose }: { onClose: () => void }) {
  const [title, setTitle] = useState('')
  const create = useCreatePolicy()
  const navigate = useNavigate()
  return (
    <Dialog open onOpenChange={(o) => !o && onClose()}>
      <DialogContent>
        <form
          className="space-y-4"
          onSubmit={(e) => {
            e.preventDefault()
            create.mutate({ title }, { onSuccess: (p) => void navigate({ to: '/app/policies/$id', params: { id: p.id } }), onError: (e) => toast.error(errorMessage(e)) })
          }}
        >
          <DialogHeader>
            <DialogTitle>New blank policy</DialogTitle>
          </DialogHeader>
          <FormField label="Title" htmlFor="pt">
            <Input id="pt" value={title} onChange={(e) => setTitle(e.target.value)} autoFocus />
          </FormField>
          <DialogFooter>
            <Button type="submit" loading={create.isPending}>
              Create draft
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
