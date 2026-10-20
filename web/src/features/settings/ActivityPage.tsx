import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { activityQuery } from '@/api/org'
import type { ActivityEntry } from '@/api/types'
import { PageHeader } from '@/components/layout/PageHeader'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { formatDate } from '@/lib/utils'

export function ActivityPage() {
  const [action, setAction] = useState('')
  const [cursors, setCursors] = useState<string[]>([])
  const cursor = cursors[cursors.length - 1]
  const q = useQuery(activityQuery({ action: action || undefined, cursor }))
  return (
    <div>
      <PageHeader
        title="Activity log"
        description="Immutable record of every change in this organization. Auditors can read it; nobody can edit it."
        actions={
          <Input
            placeholder="Filter by action, e.g. member."
            value={action}
            onChange={(e) => {
              setAction(e.target.value)
              setCursors([])
            }}
            className="w-64"
            data-testid="activity-filter"
          />
        }
      />
      <Card>
        <CardContent className="p-0">
          <ul className="divide-y" data-testid="activity-list">
            {q.data?.items.map((e) => (
              <ActivityRow key={e.id} e={e} />
            ))}
            {q.data && q.data.items.length === 0 ? <li className="p-6 text-sm text-muted-foreground">No activity yet.</li> : null}
          </ul>
        </CardContent>
      </Card>
      <div className="mt-4 flex justify-between">
        <Button variant="outline" size="sm" disabled={cursors.length === 0} onClick={() => setCursors(cursors.slice(0, -1))}>
          Previous
        </Button>
        <Button variant="outline" size="sm" disabled={!q.data?.next_cursor} onClick={() => q.data?.next_cursor && setCursors([...cursors, q.data.next_cursor])} data-testid="activity-next">
          Next
        </Button>
      </div>
    </div>
  )
}

function ActivityRow({ e }: { e: ActivityEntry }) {
  const [open, setOpen] = useState(false)
  const hasMeta = Object.keys(e.metadata ?? {}).length > 0
  return (
    <li className="px-4 py-3 text-sm" data-testid="activity-row" data-action={e.action}>
      <div className="flex flex-wrap items-center gap-2">
        <Badge variant={e.actor_type === 'user' ? 'secondary' : 'outline'}>{e.actor_type}</Badge>
        <span className="font-medium">{e.actor_label}</span>
        <code className="rounded bg-muted px-1.5 py-0.5 text-xs">{e.action}</code>
        {e.target_label ? (
          <span className="text-muted-foreground">
            {e.target_type}: <span className="text-foreground">{e.target_label}</span>
          </span>
        ) : null}
        <span className="ml-auto text-xs text-muted-foreground" title={e.created_at}>
          {formatDate(e.created_at, true)}
        </span>
        {hasMeta ? (
          <button className="text-xs text-muted-foreground underline" onClick={() => setOpen(!open)}>
            {open ? 'hide' : 'details'}
          </button>
        ) : null}
      </div>
      {open ? <pre className="mt-2 overflow-auto rounded bg-muted p-2 text-xs">{JSON.stringify(e.metadata, null, 2)}</pre> : null}
    </li>
  )
}
