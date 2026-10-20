import type { PolicyRow } from '@/api/phase2'
import { Badge } from '@/components/ui/badge'

export function PolicyStatusBadge({ p }: { p: PolicyRow }) {
  if (p.open_status === 'in_review') return <Badge variant="warning">In review</Badge>
  if (p.open_status === 'approved') return <Badge variant="secondary">Approved — ready to publish</Badge>
  if (p.open_status === 'rejected') return <Badge variant="destructive">Changes requested</Badge>
  if (p.published_version) return <Badge variant="success">Published v{p.published_version}</Badge>
  return <Badge variant="outline">Draft</Badge>
}
