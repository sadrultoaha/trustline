import { Link } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import { CheckCircle2, GraduationCap, Laptop, ScrollText } from 'lucide-react'
import { useMe } from '@/api/auth'
import { myWorkQuery } from '@/api/phase2'
import { PageHeader } from '@/components/layout/PageHeader'
import { SeverityBadge } from '@/components/status'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { formatDate, timeAgo } from '@/lib/utils'
import { ReviewTable } from '@/features/reviews/CampaignPage'

export function MyWorkPage() {
  const { data: me } = useMe()
  const { data: w } = useQuery(myWorkQuery())
  if (!w) return <p className="text-sm text-muted-foreground">Loading…</p>
  const pendingPolicies = w.policies.filter((p) => !p.accepted_at)
  const pendingReviews = w.review_items.filter((i) => i.decision === 'pending')
  return (
    <div className="space-y-6">
      <PageHeader title="My work" description={w.pending ? `${w.pending} thing(s) need your attention, ${me?.user.name}.` : `You're all set, ${me?.user.name}.`} />
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <ScrollText className="h-4 w-4" /> Policies to acknowledge
          </CardTitle>
        </CardHeader>
        <CardContent>
          {pendingPolicies.length === 0 ? (
            <p className="flex items-center gap-2 text-sm text-success" data-testid="policies-all-done">
              <CheckCircle2 className="h-4 w-4" /> All {w.policies.length} published policies acknowledged.
            </p>
          ) : (
            <ul className="divide-y" data-testid="my-pending-policies">
              {pendingPolicies.map((p) => (
                <li key={p.version_id} className="flex items-center justify-between py-2" data-testid={`my-policy-${p.title}`}>
                  <span className="text-sm">
                    <span className="font-medium">{p.title}</span> <span className="text-muted-foreground">v{p.version} · published {formatDate(p.published_at)}</span>
                  </span>
                  <Button size="sm" asChild>
                    <Link to="/app/policies/$id" params={{ id: p.policy_id }} data-testid="my-policy-open">
                      Read & acknowledge
                    </Link>
                  </Button>
                </li>
              ))}
            </ul>
          )}
        </CardContent>
      </Card>

      <div className="grid gap-6 md:grid-cols-2">
        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <GraduationCap className="h-4 w-4" /> Security training
            </CardTitle>
            <CardDescription>{w.training?.course.title}</CardDescription>
          </CardHeader>
          <CardContent className="space-y-3 text-sm">
            {w.training?.completed ? (
              <p className="text-success" data-testid="training-done">
                Passed{w.training.last_score != null ? ` with ${w.training.last_score}%` : ''} · valid until {formatDate(w.training.expires_at)}
              </p>
            ) : (
              <p className="text-muted-foreground">{w.training?.last_passed === false ? `Last attempt scored ${w.training.last_score}% — ${w.training.course.passing_score}% needed.` : 'Not completed yet.'}</p>
            )}
            <Button asChild variant={w.training?.completed ? 'outline' : 'default'} size="sm">
              <Link to="/app/training" data-testid="training-start">
                {w.training?.completed ? 'Review course' : 'Start course'}
              </Link>
            </Button>
          </CardContent>
        </Card>
        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <Laptop className="h-4 w-4" /> My devices
            </CardTitle>
          </CardHeader>
          <CardContent className="space-y-2 text-sm">
            {w.devices.length === 0 ? <p className="text-muted-foreground">No device enrolled. Ask your admin for the Trustline agent install command.</p> : null}
            {w.devices.map((d) => (
              <div key={d.id} className="flex flex-wrap items-center gap-2">
                <span className="font-medium">{d.hostname}</span>
                <span className="text-muted-foreground">seen {timeAgo(d.last_seen_at)}</span>
                {d.compliant ? <Badge variant="success">Compliant</Badge> : d.issues.map((i) => <Badge key={i} variant="destructive">{i}</Badge>)}
              </div>
            ))}
          </CardContent>
        </Card>
      </div>

      {pendingReviews.length || w.review_items.length ? (
        <Card>
          <CardHeader>
            <CardTitle>Access reviews assigned to me</CardTitle>
            <CardDescription>{pendingReviews.length} pending</CardDescription>
          </CardHeader>
          <CardContent className="p-0">
            <ReviewTable items={w.review_items} open showCampaign />
          </CardContent>
        </Card>
      ) : null}

      <Card>
        <CardHeader>
          <CardTitle>My tasks</CardTitle>
        </CardHeader>
        <CardContent>
          {w.tasks.length === 0 ? <p className="text-sm text-muted-foreground">Nothing assigned to you.</p> : null}
          <ul className="divide-y">
            {w.tasks.map((t) => (
              <li key={t.id} className="flex items-center justify-between py-2 text-sm">
                <Link to="/app/tasks/$id" params={{ id: t.id }} className="font-medium hover:underline">
                  {t.title}
                </Link>
                <span className="flex items-center gap-2">
                  {t.due_at ? <span className="text-xs text-muted-foreground">due {formatDate(t.due_at)}</span> : null}
                  <SeverityBadge severity={t.priority} />
                </span>
              </li>
            ))}
          </ul>
        </CardContent>
      </Card>
    </div>
  )
}
