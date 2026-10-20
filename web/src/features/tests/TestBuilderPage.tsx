import { useState } from 'react'
import { useNavigate } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import { Braces, FlaskConical, Plus, Trash2 } from 'lucide-react'
import { toast } from 'sonner'
import { errorMessage, isApiError } from '@/api/client'
import { controlsQuery, testSchemaQuery, useCreateTest, usePreviewRule, type Condition, type Expr, type ResourceType, type Rule, type SchemaField } from '@/api/compliance'
import { FormField } from '@/components/layout/FormField'
import { PageHeader } from '@/components/layout/PageHeader'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Select } from '@/components/ui/select'
import { Textarea } from '@/components/ui/textarea'
import { cn } from '@/lib/utils'
import { describeRule } from './describeRule'

type Group = { kind: 'and' | 'or'; items: Node[] }
type Node = { id: number; cond?: Condition; group?: Group }

let seq = 1
const newCond = (field = ''): Node => ({ id: seq++, cond: { field, op: 'eq', value: '' } })
const newGroup = (kind: 'and' | 'or' = 'and', items: Node[] = []): Node => ({ id: seq++, group: { kind, items } })

function toExpr(n: Node, fields: Record<string, SchemaField>): Expr | null {
  if (n.cond) {
    const c = n.cond
    if (!c.field) return null
    const out: Condition = { field: c.field, op: c.op }
    if (c.op !== 'exists' && c.op !== 'not_exists') out.value = coerce(c.value, fields[c.field]?.type ?? 'string', c.op)
    if (c.match === 'any') out.match = 'any'
    return out
  }
  const items = n.group!.items.map((x) => toExpr(x, fields)).filter((x): x is Expr => x !== null)
  if (items.length === 0) return null
  if (items.length === 1) return items[0]!
  return n.group!.kind === 'and' ? { and: items } : { or: items }
}

function coerce(v: unknown, type: string, op: string): unknown {
  const s = String(v ?? '')
  if (op === 'in' || op === 'nin') return s.split(',').map((x) => x.trim()).filter(Boolean).map((x) => (type === 'number' ? Number(x) : x))
  if (op === 'len_gt' || op === 'len_lt') return Number(s)
  if (type === 'bool') return s === 'true'
  if (type === 'number') return Number(s)
  return s
}

export function TestBuilderPage() {
  const schema = useQuery(testSchemaQuery())
  const controls = useQuery(controlsQuery({}))
  const create = useCreateTest()
  const preview = usePreviewRule()
  const navigate = useNavigate()
  const [name, setName] = useState('')
  const [description, setDescription] = useState('')
  const [controlId, setControlId] = useState('')
  const [connector, setConnector] = useState('aws')
  const [resource, setResource] = useState('')
  const [quantifier, setQuantifier] = useState<Rule['quantifier']>('all')
  const [min, setMin] = useState('')
  const [max, setMax] = useState('')
  const [severity, setSeverity] = useState('high')
  const [where, setWhere] = useState<Node>(() => newGroup('and'))
  const [assert, setAssert] = useState<Node>(() => newGroup('and', [newCond()]))
  const [remSummary, setRemSummary] = useState('')
  const [remSteps, setRemSteps] = useState('')
  const [showJSON, setShowJSON] = useState(false)
  const [fieldsErr, setFieldsErr] = useState<Record<string, string>>({})

  const conn = schema.data?.connectors.find((c) => c.type === connector)
  const rt: ResourceType | undefined = conn?.resources.find((r) => r.type === resource) ?? conn?.resources[0]
  const fieldMap: Record<string, SchemaField> = Object.fromEntries((rt?.fields ?? []).map((f) => [f.path, f]))

  const rule: Rule | null = (() => {
    if (!rt) return null
    const a = toExpr(assert, fieldMap)
    if (!a) return null
    const r: Rule = { resource: rt.type, quantifier, assert: a }
    const w = toExpr(where, fieldMap)
    if (w) r.where = w
    if (quantifier === 'count') {
      if (min !== '') r.min = Number(min)
      if (max !== '') r.max = Number(max)
    }
    return r
  })()

  return (
    <div className="space-y-6">
      <PageHeader title="Custom test" description="Build a check without code. It compiles to the same rule format as the built-in library and runs on every sync." />
      <div className="grid gap-6 lg:grid-cols-3">
        <div className="space-y-6 lg:col-span-2">
          <Card>
            <CardHeader>
              <CardTitle>What to check</CardTitle>
            </CardHeader>
            <CardContent className="grid gap-4 md:grid-cols-3">
              <FormField label="Integration" htmlFor="b-conn">
                <Select
                  id="b-conn"
                  value={connector}
                  onChange={(e) => {
                    setConnector(e.target.value)
                    setResource('')
                    setWhere(newGroup('and'))
                    setAssert(newGroup('and', [newCond()]))
                  }}
                  data-testid="builder-connector"
                >
                  {schema.data?.connectors.map((c) => (
                    <option key={c.type} value={c.type}>
                      {c.name}
                    </option>
                  ))}
                </Select>
              </FormField>
              <FormField label="Resource" htmlFor="b-res">
                <Select
                  id="b-res"
                  value={rt?.type ?? ''}
                  onChange={(e) => {
                    setResource(e.target.value)
                    setWhere(newGroup('and'))
                    setAssert(newGroup('and', [newCond()]))
                  }}
                  data-testid="builder-resource"
                >
                  {conn?.resources.map((r) => (
                    <option key={r.type} value={r.type}>
                      {r.label}
                    </option>
                  ))}
                </Select>
              </FormField>
              <FormField label="Pass when" htmlFor="b-q">
                <Select id="b-q" value={quantifier} onChange={(e) => setQuantifier(e.target.value as Rule['quantifier'])} data-testid="builder-quantifier">
                  <option value="all">every resource matches</option>
                  <option value="any">at least one matches</option>
                  <option value="none">no resource matches</option>
                  <option value="count">a number of resources match</option>
                </Select>
              </FormField>
              {quantifier === 'count' ? (
                <>
                  <FormField label="Minimum" htmlFor="b-min">
                    <Input id="b-min" type="number" value={min} onChange={(e) => setMin(e.target.value)} />
                  </FormField>
                  <FormField label="Maximum" htmlFor="b-max">
                    <Input id="b-max" type="number" value={max} onChange={(e) => setMax(e.target.value)} />
                  </FormField>
                </>
              ) : null}
            </CardContent>
          </Card>
          <Card>
            <CardHeader>
              <CardTitle>Only consider resources where (optional)</CardTitle>
            </CardHeader>
            <CardContent>
              <GroupEditor node={where} fields={rt?.fields ?? []} onChange={setWhere} ops={schema.data?.operators ?? []} testid="where" />
            </CardContent>
          </Card>
          <Card>
            <CardHeader>
              <CardTitle>Condition</CardTitle>
              <CardDescription>{quantifier === 'none' ? 'Resources matching this condition fail the test.' : 'Resources must match this condition.'}</CardDescription>
            </CardHeader>
            <CardContent>
              <GroupEditor node={assert} fields={rt?.fields ?? []} onChange={setAssert} ops={schema.data?.operators ?? []} testid="assert" />
            </CardContent>
          </Card>
        </div>

        <div className="space-y-6">
          <Card>
            <CardHeader>
              <CardTitle>Details</CardTitle>
            </CardHeader>
            <CardContent className="space-y-4">
              <FormField label="Name" htmlFor="b-name" error={fieldsErr.name}>
                <Input id="b-name" value={name} onChange={(e) => setName(e.target.value)} placeholder="Confidential buckets use KMS" data-testid="builder-name" />
              </FormField>
              <FormField label="Description" htmlFor="b-desc">
                <Textarea id="b-desc" value={description} onChange={(e) => setDescription(e.target.value)} />
              </FormField>
              <FormField label="Control" htmlFor="b-ctl" error={fieldsErr.control_id}>
                <Select id="b-ctl" value={controlId} onChange={(e) => setControlId(e.target.value)} data-testid="builder-control">
                  <option value="">Choose a control…</option>
                  {controls.data?.items.map((c) => (
                    <option key={c.id} value={c.id}>
                      {c.code} · {c.name}
                    </option>
                  ))}
                </Select>
              </FormField>
              <FormField label="Severity" htmlFor="b-sev">
                <Select id="b-sev" value={severity} onChange={(e) => setSeverity(e.target.value)}>
                  {['critical', 'high', 'medium', 'low'].map((s) => (
                    <option key={s} value={s}>
                      {s}
                    </option>
                  ))}
                </Select>
              </FormField>
              <FormField label="Fix summary" htmlFor="b-rs">
                <Input id="b-rs" value={remSummary} onChange={(e) => setRemSummary(e.target.value)} />
              </FormField>
              <FormField label="Fix steps (one per line)" htmlFor="b-rst">
                <Textarea id="b-rst" value={remSteps} onChange={(e) => setRemSteps(e.target.value)} />
              </FormField>
            </CardContent>
          </Card>
          <Card>
            <CardHeader>
              <CardTitle className="flex items-center justify-between">
                Rule
                <Button size="sm" variant="ghost" onClick={() => setShowJSON(!showJSON)}>
                  <Braces /> {showJSON ? 'Hide' : 'JSON'}
                </Button>
              </CardTitle>
              <CardDescription data-testid="builder-description">{rule ? describeRule(rule) : 'Add at least one condition.'}</CardDescription>
            </CardHeader>
            <CardContent className="space-y-3">
              {showJSON && rule ? <pre className="max-h-64 overflow-auto rounded bg-muted/50 p-2 text-xs" data-testid="builder-json">{JSON.stringify(rule, null, 2)}</pre> : null}
              {fieldsErr.rule ? (
                <Alert variant="destructive">
                  <AlertDescription>{fieldsErr.rule}</AlertDescription>
                </Alert>
              ) : null}
              {preview.data ? (
                <Alert variant={preview.data.status === 'pass' ? 'success' : 'destructive'} data-testid="builder-preview-result">
                  <AlertDescription>
                    <strong>{preview.data.status === 'pass' ? 'Would pass' : 'Would fail'}</strong> — {preview.data.summary}
                    {(preview.data.failing ?? []).slice(0, 5).map((f) => (
                      <div key={f.id} className="text-xs">
                        {f.id}: {f.reason}
                      </div>
                    ))}
                  </AlertDescription>
                </Alert>
              ) : null}
              <div className="flex gap-2">
                <Button
                  variant="outline"
                  disabled={!rule}
                  loading={preview.isPending}
                  data-testid="builder-preview"
                  onClick={() =>
                    rule &&
                    preview.mutate({ integration_type: connector, rule }, { onError: (e) => (isApiError(e) && e.fields.rule ? setFieldsErr({ rule: e.fields.rule }) : toast.error(errorMessage(e))) })
                  }
                >
                  <FlaskConical /> Try on live data
                </Button>
                <Button
                  disabled={!rule}
                  loading={create.isPending}
                  data-testid="builder-save"
                  onClick={() => {
                    if (!rule) return
                    setFieldsErr({})
                    create.mutate(
                      { name, description, control_id: controlId, integration_type: connector, severity, rule, remediation: { summary: remSummary, steps: remSteps.split('\n').map((s) => s.trim()).filter(Boolean) } },
                      {
                        onSuccess: (t) => {
                          toast.success('Custom test created — running now')
                          void navigate({ to: '/app/tests/$id', params: { id: t.id } })
                        },
                        onError: (e) => (isApiError(e) && Object.keys(e.fields).length ? setFieldsErr(e.fields) : toast.error(errorMessage(e))),
                      },
                    )
                  }}
                >
                  Save test
                </Button>
              </div>
            </CardContent>
          </Card>
        </div>
      </div>
    </div>
  )
}

function GroupEditor({ node, fields, onChange, ops, depth = 0, onRemove, testid }: { node: Node; fields: SchemaField[]; onChange: (n: Node) => void; ops: { op: string; label: string }[]; depth?: number; onRemove?: () => void; testid: string }) {
  const g = node.group!
  const update = (items: Node[]) => onChange({ ...node, group: { ...g, items } })
  return (
    <div className={cn('space-y-2 rounded-md', depth > 0 && 'border border-dashed p-3')} data-testid={`group-${testid}`}>
      <div className="flex items-center gap-2 text-xs">
        <span className="text-muted-foreground">Match</span>
        <Select value={g.kind} onChange={(e) => onChange({ ...node, group: { ...g, kind: e.target.value as 'and' | 'or' } })} className="h-7 w-24 text-xs" data-testid={`group-kind-${testid}`}>
          <option value="and">ALL of</option>
          <option value="or">ANY of</option>
        </Select>
        {onRemove ? (
          <button className="ml-auto text-muted-foreground hover:text-destructive" onClick={onRemove} title="Remove group">
            <Trash2 className="h-4 w-4" />
          </button>
        ) : null}
      </div>
      {g.items.map((child, i) =>
        child.group ? (
          <GroupEditor
            key={child.id}
            node={child}
            fields={fields}
            ops={ops}
            depth={depth + 1}
            testid={`${testid}-${i}`}
            onChange={(n) => update(g.items.map((x) => (x.id === child.id ? n : x)))}
            onRemove={() => update(g.items.filter((x) => x.id !== child.id))}
          />
        ) : (
          <ConditionRow
            key={child.id}
            cond={child.cond!}
            fields={fields}
            ops={ops}
            testid={`${testid}-${i}`}
            onChange={(c) => update(g.items.map((x) => (x.id === child.id ? { ...x, cond: c } : x)))}
            onRemove={() => update(g.items.filter((x) => x.id !== child.id))}
          />
        ),
      )}
      <div className="flex gap-2">
        <Button size="sm" variant="ghost" onClick={() => update([...g.items, newCond()])} data-testid={`add-cond-${testid}`}>
          <Plus /> Condition
        </Button>
        {depth < 2 ? (
          <Button size="sm" variant="ghost" onClick={() => update([...g.items, newGroup(g.kind === 'and' ? 'or' : 'and', [newCond()])])} data-testid={`add-group-${testid}`}>
            <Plus /> Group
          </Button>
        ) : null}
      </div>
    </div>
  )
}

function ConditionRow({ cond, fields, ops, onChange, onRemove, testid }: { cond: Condition; fields: SchemaField[]; ops: { op: string; label: string }[]; onChange: (c: Condition) => void; onRemove: () => void; testid: string }) {
  const f = fields.find((x) => x.path === cond.field)
  const needsValue = cond.op !== 'exists' && cond.op !== 'not_exists'
  const fanout = cond.field.includes('[*]')
  const leafFields = fields.filter((x) => x.type !== 'object[]')
  return (
    <div className="flex flex-wrap items-center gap-2" data-testid={`cond-${testid}`}>
      <Select value={cond.field} onChange={(e) => onChange({ ...cond, field: e.target.value, value: '' })} className="w-64" data-testid={`cond-field-${testid}`}>
        <option value="">Field…</option>
        {leafFields.map((x) => (
          <option key={x.path} value={x.path}>
            {x.label} ({x.path})
          </option>
        ))}
      </Select>
      <Select value={cond.op} onChange={(e) => onChange({ ...cond, op: e.target.value })} className="w-44" data-testid={`cond-op-${testid}`}>
        {ops.map((o) => (
          <option key={o.op} value={o.op}>
            {o.label}
          </option>
        ))}
      </Select>
      {needsValue ? (
        f?.type === 'bool' && cond.op !== 'in' && cond.op !== 'nin' ? (
          <Select value={String(cond.value ?? '')} onChange={(e) => onChange({ ...cond, value: e.target.value })} className="w-28" data-testid={`cond-value-${testid}`}>
            <option value="">…</option>
            <option value="true">true</option>
            <option value="false">false</option>
          </Select>
        ) : (
          <Input
            value={String(cond.value ?? '')}
            onChange={(e) => onChange({ ...cond, value: e.target.value })}
            type={f?.type === 'number' && cond.op !== 'in' && cond.op !== 'nin' ? 'number' : 'text'}
            placeholder={cond.op === 'in' || cond.op === 'nin' ? 'a, b, c' : 'value'}
            className="w-44"
            data-testid={`cond-value-${testid}`}
          />
        )
      ) : null}
      {fanout ? (
        <label className="flex items-center gap-1 text-xs text-muted-foreground">
          <input type="checkbox" checked={cond.match === 'any'} onChange={(e) => onChange({ ...cond, match: e.target.checked ? 'any' : undefined })} /> any item
        </label>
      ) : null}
      <button className="text-muted-foreground hover:text-destructive" onClick={onRemove} title="Remove condition">
        <Trash2 className="h-4 w-4" />
      </button>
    </div>
  )
}

