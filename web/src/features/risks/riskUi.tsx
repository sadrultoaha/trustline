import { Badge } from '@/components/ui/badge'
import { cn } from '@/lib/utils'

function levelOf(score: number): 'low' | 'medium' | 'high' | 'critical' {
  return score >= 20 ? 'critical' : score >= 12 ? 'high' : score >= 6 ? 'medium' : 'low'
}

const levelCls = { low: 'bg-success/15 text-success', medium: 'bg-warning/20 text-amber-800', high: 'bg-orange-500/20 text-orange-700', critical: 'bg-destructive/15 text-destructive' }

export function ScoreBadge({ score, testid }: { score: number; testid?: string }) {
  const lvl = levelOf(score)
  return (
    <span className={cn('inline-flex items-center rounded-md px-2 py-0.5 text-xs font-semibold tabular-nums', levelCls[lvl])} data-testid={testid} data-level={lvl}>
      {score} · {lvl}
    </span>
  )
}

export function RiskStatusBadge({ status }: { status: string }) {
  const v = { suggested: 'warning', open: 'destructive', in_treatment: 'secondary', accepted: 'outline', closed: 'success', dismissed: 'outline' } as const
  return (
    <Badge variant={v[status as keyof typeof v] ?? 'outline'} className="capitalize" data-testid="risk-status">
      {status.replace('_', ' ')}
    </Badge>
  )
}

export function HeatmapGrid({ grid, onSelect, selected }: { grid: number[][]; onSelect?: (l: number, i: number) => void; selected?: [number, number] | null }) {
  const cell = (l: number, i: number) => {
    const score = l * i
    const bg = score >= 20 ? 'bg-destructive/80' : score >= 12 ? 'bg-orange-400/80' : score >= 6 ? 'bg-amber-300/80' : 'bg-emerald-300/70'
    return bg
  }
  return (
    <div className="inline-grid grid-cols-[auto_repeat(5,2.75rem)] gap-1 text-xs" data-testid="risk-heatmap">
      {[5, 4, 3, 2, 1].map((l) => (
        <div key={l} className="contents">
          <div className="flex items-center pr-2 text-muted-foreground">{l}</div>
          {[1, 2, 3, 4, 5].map((i) => {
            const n = grid[l - 1]?.[i - 1] ?? 0
            const isSel = selected && selected[0] === l && selected[1] === i
            return (
              <button
                key={i}
                type="button"
                onClick={() => onSelect?.(l, i)}
                className={cn('flex h-10 items-center justify-center rounded font-semibold text-slate-900', cell(l, i), n === 0 && 'opacity-40', isSel && 'ring-2 ring-foreground')}
                data-testid={`heat-${l}-${i}`}
                title={`Likelihood ${l} × impact ${i}: ${n} risk(s)`}
              >
                {n || ''}
              </button>
            )
          })}
        </div>
      ))}
      <div />
      {[1, 2, 3, 4, 5].map((i) => (
        <div key={i} className="text-center text-muted-foreground">
          {i}
        </div>
      ))}
      <div />
      <div className="col-span-5 text-center text-muted-foreground">impact →</div>
    </div>
  )
}
