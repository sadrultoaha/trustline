import { AlertTriangle, CheckCircle2, CircleDashed, Clock, XCircle } from 'lucide-react'
import type { ControlStatus, Severity, TestStatus } from '@/api/compliance'
import { Badge } from '@/components/ui/badge'
import { cn } from '@/lib/utils'

const controlStyles: Record<string, { label: string; cls: string; Icon: typeof CheckCircle2 }> = {
  passing: { label: 'Passing', cls: 'bg-success/15 text-success', Icon: CheckCircle2 },
  failing: { label: 'Failing', cls: 'bg-destructive/15 text-destructive', Icon: XCircle },
  needs_evidence: { label: 'Needs evidence', cls: 'bg-warning/20 text-amber-800', Icon: AlertTriangle },
  not_in_scope: { label: 'Not in scope', cls: 'bg-muted text-muted-foreground', Icon: CircleDashed },
}

export function ControlStatusBadge({ status, className }: { status: ControlStatus | 'not_in_scope'; className?: string }) {
  const s = controlStyles[status]
  return (
    <span className={cn('inline-flex items-center gap-1 rounded-md px-2 py-0.5 text-xs font-medium', s.cls, className)} data-testid="control-status" data-status={status}>
      <s.Icon className="h-3.5 w-3.5" />
      {s.label}
    </span>
  )
}

const testStyles: Record<TestStatus, { label: string; cls: string; Icon: typeof CheckCircle2 }> = {
  pass: { label: 'Pass', cls: 'bg-success/15 text-success', Icon: CheckCircle2 },
  fail: { label: 'Fail', cls: 'bg-destructive/15 text-destructive', Icon: XCircle },
  error: { label: 'Error', cls: 'bg-destructive/15 text-destructive', Icon: AlertTriangle },
  pending: { label: 'Pending', cls: 'bg-muted text-muted-foreground', Icon: Clock },
  na: { label: 'Not applicable', cls: 'bg-muted text-muted-foreground', Icon: CircleDashed },
}

export function TestStatusBadge({ status }: { status: TestStatus }) {
  const s = testStyles[status]
  return (
    <span className={cn('inline-flex items-center gap-1 rounded-md px-2 py-0.5 text-xs font-medium', s.cls)} data-testid="test-status" data-status={status}>
      <s.Icon className="h-3.5 w-3.5" />
      {s.label}
    </span>
  )
}

export function SeverityBadge({ severity }: { severity: Severity }) {
  const v = { critical: 'destructive', high: 'warning', medium: 'secondary', low: 'outline' } as const
  return (
    <Badge variant={v[severity]} className="capitalize">
      {severity}
    </Badge>
  )
}

export function ConnectorLogo({ type, className }: { type: string; className?: string }) {
  const colors: Record<string, string> = { aws: 'bg-[#232f3e] text-[#ff9900]', gcp: 'bg-[#e8f0fe] text-[#1a73e8]', github: 'bg-[#24292f] text-white', okta: 'bg-[#00297a] text-white', google_workspace: 'bg-[#e6f4ea] text-[#188038]', hris: 'bg-[#e8f5e9] text-[#2e7d32]' }
  const label: Record<string, string> = { aws: 'aws', gcp: 'GCP', github: 'GH', okta: 'okta', google_workspace: 'GW', hris: 'HR' }
  return <span className={cn('flex h-10 w-10 shrink-0 items-center justify-center rounded-lg text-xs font-bold', colors[type] ?? 'bg-muted', className)}>{label[type] ?? type.slice(0, 2)}</span>
}
