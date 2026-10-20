import { Link } from '@tanstack/react-router'
import { Activity, ShieldCheck, Users } from 'lucide-react'
import { useQuery } from '@tanstack/react-query'
import { useMe, usePermissions } from '@/api/auth'
import { activityQuery, membersQuery } from '@/api/org'
import { PageHeader } from '@/components/layout/PageHeader'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { timeAgo } from '@/lib/utils'

export function DashboardPage() {
  const { data: me } = useMe()
  const { can } = usePermissions()
  const members = useQuery({ ...membersQuery, enabled: can('members:read') })
  const activity = useQuery({ ...activityQuery(), enabled: can('activity:read') })
  return (
    <div>
      <PageHeader title={`Welcome, ${me?.user.name ?? ''}`} description={`${me?.org?.name ?? ''} · signed in as ${me?.role}`} />
      <div className="grid gap-4 md:grid-cols-3">
        <Card>
          <CardHeader className="pb-2">
            <CardDescription className="flex items-center gap-2">
              <ShieldCheck className="h-4 w-4" /> Compliance
            </CardDescription>
            <CardTitle className="text-2xl">Not connected</CardTitle>
          </CardHeader>
          <CardContent className="text-sm text-muted-foreground">Connect an integration to start monitoring controls (Phase 1).</CardContent>
        </Card>
        <Card>
          <CardHeader className="pb-2">
            <CardDescription className="flex items-center gap-2">
              <Users className="h-4 w-4" /> Members
            </CardDescription>
            <CardTitle className="text-2xl" data-testid="dashboard-member-count">
              {can('members:read') ? (members.data?.items.length ?? '…') : '—'}
            </CardTitle>
          </CardHeader>
          <CardContent className="text-sm text-muted-foreground">
            {can('members:write') ? (
              <Link to="/app/settings/members" className="underline">
                Manage members and invites
              </Link>
            ) : (
              'People in this organization'
            )}
          </CardContent>
        </Card>
        <Card>
          <CardHeader className="pb-2">
            <CardDescription className="flex items-center gap-2">
              <Activity className="h-4 w-4" /> Recent activity
            </CardDescription>
            <CardTitle className="text-2xl">{can('activity:read') ? (activity.data?.items.length ?? '…') : '—'}</CardTitle>
          </CardHeader>
          <CardContent className="text-sm text-muted-foreground">
            {can('activity:read') ? (
              <Link to="/app/settings/activity" className="underline">
                View the audit trail
              </Link>
            ) : (
              'Audit trail is visible to admins, editors and auditors'
            )}
          </CardContent>
        </Card>
      </div>
      {can('activity:read') && activity.data?.items.length ? (
        <Card className="mt-6">
          <CardHeader>
            <CardTitle>Latest events</CardTitle>
          </CardHeader>
          <CardContent>
            <ul className="divide-y text-sm">
              {activity.data.items.slice(0, 8).map((e) => (
                <li key={e.id} className="flex items-center justify-between py-2">
                  <span>
                    <span className="font-medium">{e.actor_label}</span> <span className="text-muted-foreground">{e.action}</span>{' '}
                    {e.target_label ? <span>{e.target_label}</span> : null}
                  </span>
                  <span className="text-xs text-muted-foreground">{timeAgo(e.created_at)}</span>
                </li>
              ))}
            </ul>
          </CardContent>
        </Card>
      ) : null}
    </div>
  )
}
