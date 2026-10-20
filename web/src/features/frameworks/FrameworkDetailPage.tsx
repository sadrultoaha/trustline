import { useState } from 'react'
import { Link, useNavigate, useParams } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import { CheckCircle2, ChevronDown, ChevronRight, XCircle } from 'lucide-react'
import { toast } from 'sonner'
import { usePermissions } from '@/api/auth'
import { errorMessage } from '@/api/client'
import { frameworkQuery, useAdoptFramework, useRemoveFramework, type SectionView } from '@/api/compliance'
import { PageHeader } from '@/components/layout/PageHeader'
import { ControlStatusBadge } from '@/components/status'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { Progress } from '@/components/ui/progress'
import { cn } from '@/lib/utils'

export function FrameworkDetailPage() {
  const { id } = useParams({ from: '/app/frameworks/$id' })
  const { data: f } = useQuery(frameworkQuery(id))
  const { can } = usePermissions()
  const adopt = useAdoptFramework()
  const remove = useRemoveFramework()
  const navigate = useNavigate()
  const [onlyGaps, setOnlyGaps] = useState(false)
  if (!f) return <p className="text-sm text-muted-foreground">Loading…</p>
  const r = f.readiness
  return (
    <div>
      <PageHeader
        title={f.name}
        description={`${f.publisher} · ${f.version}`}
        actions={
          can('controls:write') ? (
            f.adopted ? (
              <Button
                variant="outline"
                onClick={() =>
                  remove.mutate(f.id, {
                    onSuccess: () => {
                      toast.success(`${f.name} removed from scope`)
                      void navigate({ to: '/app/frameworks' })
                    },
                  })
                }
              >
                Remove from scope
              </Button>
            ) : (
              <Button loading={adopt.isPending} onClick={() => adopt.mutate(f.id, { onError: (e) => toast.error(errorMessage(e)) })} data-testid="framework-adopt">
                Add to scope
              </Button>
            )
          ) : null
        }
      />
      <Card className="mb-6">
        <CardContent className="grid gap-6 p-6 md:grid-cols-4">
          <div className="md:col-span-2">
            <div className="flex items-baseline gap-2">
              <span className="text-4xl font-semibold tabular-nums" data-testid="framework-detail-percent">
                {r.percent}%
              </span>
              <span className="text-sm text-muted-foreground">ready</span>
            </div>
            <Progress className="mt-3" value={r.percent} tone={r.percent >= 80 ? 'success' : 'primary'} />
            <p className="mt-3 text-sm text-muted-foreground">{f.description}</p>
          </div>
          <Stat label="Requirements met" value={`${r.requirements_met} / ${r.requirements_total}`} />
          <Stat label="Controls passing" value={`${r.controls_passing} / ${r.controls_total}`} sub={r.controls_failing ? `${r.controls_failing} failing` : undefined} />
        </CardContent>
      </Card>
      <div className="mb-3 flex items-center justify-between">
        <h2 className="text-lg font-semibold">Requirements</h2>
        <label className="flex items-center gap-2 text-sm text-muted-foreground">
          <input type="checkbox" checked={onlyGaps} onChange={(e) => setOnlyGaps(e.target.checked)} data-testid="framework-only-gaps" />
          Show only gaps
        </label>
      </div>
      <div className="space-y-3">
        {f.sections.map((s) => (
          <Section key={s.name} section={s} onlyGaps={onlyGaps} />
        ))}
      </div>
    </div>
  )
}

function Stat({ label, value, sub }: { label: string; value: string; sub?: string }) {
  return (
    <div>
      <div className="text-xs uppercase tracking-wide text-muted-foreground">{label}</div>
      <div className="mt-1 text-2xl font-semibold tabular-nums">{value}</div>
      {sub ? <div className="text-xs text-destructive">{sub}</div> : null}
    </div>
  )
}

function Section({ section, onlyGaps }: { section: SectionView; onlyGaps: boolean }) {
  const [open, setOpen] = useState(section.met < section.total)
  const reqs = onlyGaps ? section.requirements.filter((r) => !r.met) : section.requirements
  if (onlyGaps && reqs.length === 0) return null
  return (
    <Card>
      <button className="flex w-full items-center gap-3 px-4 py-3 text-left" onClick={() => setOpen(!open)}>
        {open ? <ChevronDown className="h-4 w-4" /> : <ChevronRight className="h-4 w-4" />}
        <span className="flex-1 font-medium">{section.name}</span>
        <span className="text-sm tabular-nums text-muted-foreground">
          {section.met}/{section.total}
        </span>
        <Progress value={(section.met / section.total) * 100} className="w-24" tone={section.met === section.total ? 'success' : 'primary'} />
      </button>
      {open ? (
        <div className="divide-y border-t">
          {reqs.map((r) => (
            <div key={r.code} className="grid gap-3 px-4 py-3 md:grid-cols-[180px_1fr_320px]" data-testid={`requirement-${r.code}`}>
              <div className="flex items-start gap-2">
                {r.met ? <CheckCircle2 className="mt-0.5 h-4 w-4 shrink-0 text-success" /> : <XCircle className="mt-0.5 h-4 w-4 shrink-0 text-muted-foreground" />}
                <code className="text-sm font-medium">{r.code}</code>
              </div>
              <div>
                <div className="text-sm font-medium">{r.title}</div>
                <div className="text-xs text-muted-foreground">{r.description}</div>
              </div>
              <div className="flex flex-wrap gap-1.5">
                {r.controls.map((c) =>
                  c.id ? (
                    <Link key={c.code} to="/app/controls/$id" params={{ id: c.id }} className={cn('inline-flex items-center gap-1 rounded border px-1.5 py-0.5 text-xs hover:bg-accent')} title={c.name}>
                      <code>{c.code}</code>
                      <ControlStatusBadge status={c.status} className="px-1 py-0" />
                    </Link>
                  ) : (
                    <span key={c.code} className="inline-flex items-center gap-1 rounded border border-dashed px-1.5 py-0.5 text-xs text-muted-foreground" title={c.name}>
                      <code>{c.code}</code> not in scope
                    </span>
                  ),
                )}
              </div>
            </div>
          ))}
        </div>
      ) : null}
    </Card>
  )
}
