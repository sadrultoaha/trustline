import { Link } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import { Plus } from 'lucide-react'
import { toast } from 'sonner'
import { usePermissions } from '@/api/auth'
import { errorMessage } from '@/api/client'
import { frameworksQuery, useAdoptFramework, type FrameworkSummary } from '@/api/compliance'
import { PageHeader } from '@/components/layout/PageHeader'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Progress } from '@/components/ui/progress'

export function FrameworksPage() {
  const { data } = useQuery(frameworksQuery())
  const adopted = data?.items.filter((f) => f.adopted) ?? []
  const available = data?.items.filter((f) => !f.adopted) ?? []
  return (
    <div>
      <PageHeader title="Frameworks" description="Readiness per framework. One control can satisfy requirements in several frameworks, so adding a framework reuses the work you have already done." />
      <h2 className="mb-3 text-sm font-medium uppercase tracking-wide text-muted-foreground">In scope</h2>
      {adopted.length === 0 ? <p className="mb-6 text-sm text-muted-foreground">No frameworks yet. Add one below to start.</p> : null}
      <div className="mb-8 grid gap-4 md:grid-cols-2 xl:grid-cols-3">
        {adopted.map((f) => (
          <ReadinessCard key={f.id} f={f} />
        ))}
      </div>
      <h2 className="mb-3 text-sm font-medium uppercase tracking-wide text-muted-foreground">Available</h2>
      <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
        {available.map((f) => (
          <AvailableCard key={f.id} f={f} />
        ))}
      </div>
    </div>
  )
}

export function ReadinessCard({ f }: { f: FrameworkSummary }) {
  const r = f.readiness
  const tone = r.percent >= 80 ? 'success' : r.percent >= 40 ? 'primary' : 'warning'
  return (
    <Link to="/app/frameworks/$id" params={{ id: f.id }} data-testid={`framework-card-${f.id}`}>
      <Card className="h-full transition-shadow hover:shadow-md">
        <CardHeader className="pb-3">
          <CardDescription>{f.publisher || 'Framework'}</CardDescription>
          <CardTitle className="flex items-baseline justify-between">
            <span>{f.name}</span>
            <span className="text-2xl tabular-nums" data-testid={`framework-percent-${f.id}`}>
              {r.percent}%
            </span>
          </CardTitle>
        </CardHeader>
        <CardContent className="space-y-3">
          <Progress value={r.percent} tone={tone} />
          <div className="grid grid-cols-3 gap-2 text-xs text-muted-foreground">
            <div>
              <div className="text-base font-semibold text-foreground tabular-nums">
                {r.requirements_met}/{r.requirements_total}
              </div>
              requirements met
            </div>
            <div>
              <div className="text-base font-semibold text-success tabular-nums">{r.controls_passing}</div>
              controls passing
            </div>
            <div>
              <div className="text-base font-semibold text-destructive tabular-nums">{r.controls_failing}</div>
              failing
            </div>
          </div>
        </CardContent>
      </Card>
    </Link>
  )
}

function AvailableCard({ f }: { f: FrameworkSummary }) {
  const { can } = usePermissions()
  const adopt = useAdoptFramework()
  const r = f.readiness
  return (
    <Card className="flex flex-col" data-testid={`framework-available-${f.id}`}>
      <CardHeader className="pb-3">
        <CardDescription>
          {f.publisher} · {f.version}
        </CardDescription>
        <CardTitle>{f.name}</CardTitle>
      </CardHeader>
      <CardContent className="flex flex-1 flex-col justify-between gap-4">
        <p className="line-clamp-3 text-sm text-muted-foreground">{f.description}</p>
        <div className="flex items-center justify-between">
          <span className="text-xs text-muted-foreground">
            {r.controls_passing > 0 ? `${r.controls_passing} of ${r.controls_total} mapped controls already pass` : `${r.requirements_total} requirements`}
          </span>
          {can('controls:write') ? (
            <Button
              size="sm"
              loading={adopt.isPending}
              data-testid={`framework-adopt-${f.id}`}
              onClick={() =>
                adopt.mutate(f.id, {
                  onSuccess: (d) => toast.success(`${f.name} added — ${d.readiness.percent}% ready from existing controls`),
                  onError: (e) => toast.error(errorMessage(e)),
                })
              }
            >
              <Plus /> Add
            </Button>
          ) : null}
        </div>
      </CardContent>
    </Card>
  )
}
