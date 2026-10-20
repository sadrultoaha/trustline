import { Badge } from '@/components/ui/badge'
import type { Citation } from '@/api/phase3'

export function TierBadge({ tier }: { tier: string }) {
  const v = { critical: 'destructive', high: 'warning', medium: 'secondary', low: 'outline', unassessed: 'outline' } as const
  return (
    <Badge variant={v[tier as keyof typeof v] ?? 'outline'} className="capitalize" data-testid="vendor-tier">
      {tier}
    </Badge>
  )
}

export function Cite({ c }: { c?: Citation | null }) {
  if (!c) return null
  return (
    <span className="ml-1 inline-flex items-center rounded border bg-muted/60 px-1.5 py-0.5 text-[10px] font-medium text-muted-foreground" title={c.quote} data-testid="citation">
      p. {c.page} · {c.section}
    </span>
  )
}
