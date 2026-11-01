import { useState } from 'react'
import { Link, useNavigate } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import { Plus } from 'lucide-react'
import { toast } from 'sonner'
import { usePermissions } from '@/api/auth'
import { errorMessage, isApiError } from '@/api/client'
import { frameworksQuery } from '@/api/compliance'
import { useCreateVendor, vendorsQuery, type Vendor } from '@/api/phase3'
import { FormField } from '@/components/layout/FormField'
import { PageHeader } from '@/components/layout/PageHeader'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Select } from '@/components/ui/select'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { connectorName } from '@/lib/connectors'
import { formatDate, timeAgo } from '@/lib/utils'
import { TierBadge } from './vendorUi'
import { scopeInput, VendorScopeFields, type VendorScope } from './VendorScopeFields'

export function VendorsPage() {
  const [status, setStatus] = useState('active')
  const [framework, setFramework] = useState('')
  const { data } = useQuery(vendorsQuery(status === 'all' ? undefined : status, framework || undefined))
  const fws = useQuery(frameworksQuery()).data?.items ?? []
  const names = Object.fromEntries(fws.map((f) => [f.id, f.name]))
  const discovered = useQuery(vendorsQuery('discovered'))
  const { can } = usePermissions()
  const [open, setOpen] = useState(false)
  return (
    <div>
      <PageHeader
        title="Vendors"
        description="Third parties with access to your data: tiering, evidence, questionnaires and AI-assisted reviews. New apps are discovered from SSO and OAuth grants."
        actions={
          can('vendors:write') ? (
            <Button onClick={() => setOpen(true)} data-testid="vendor-new">
              <Plus /> Add vendor
            </Button>
          ) : null
        }
      />
      <div className="mb-4 flex flex-wrap items-center gap-3">
        <Tabs value={status} onValueChange={setStatus}>
          <TabsList>
            <TabsTrigger value="active">Inventory</TabsTrigger>
            <TabsTrigger value="discovered" data-testid="vendors-tab-discovered">
              Discovered ({discovered.data?.items.length ?? 0})
            </TabsTrigger>
            <TabsTrigger value="all">All</TabsTrigger>
            <TabsTrigger value="dismissed" data-testid="vendors-tab-dismissed">
              Dismissed
            </TabsTrigger>
          </TabsList>
        </Tabs>
        <Select value={framework} onChange={(e) => setFramework(e.target.value)} className="w-52" data-testid="vendors-framework">
          <option value="">All frameworks</option>
          {fws
            .filter((f) => f.adopted)
            .map((f) => (
              <option key={f.id} value={f.id}>
                {f.name}
              </option>
            ))}
        </Select>
      </div>
      <Card>
        <CardContent className="p-0">
          <Table data-testid="vendors-table">
            <TableHeader>
              <TableRow>
                <TableHead>Vendor</TableHead>
                <TableHead>Tier</TableHead>
                <TableHead>Data access</TableHead>
                <TableHead>Evidence</TableHead>
                <TableHead>{status === 'discovered' ? 'Discovered' : 'Next review'}</TableHead>
                <TableHead>Owner</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {data?.items.map((v) => (
                <VendorRow key={v.id} v={v} names={names} />
              ))}
              {data && data.items.length === 0 ? (
                <TableRow>
                  <TableCell colSpan={6} className="py-10 text-center text-sm text-muted-foreground">
                    No vendors here.
                  </TableCell>
                </TableRow>
              ) : null}
            </TableBody>
          </Table>
        </CardContent>
      </Card>
      {open ? <NewVendorDialog onClose={() => setOpen(false)} /> : null}
    </div>
  )
}

function VendorRow({ v, names }: { v: Vendor; names: Record<string, string> }) {
  return (
    <TableRow data-testid={`vendor-row-${v.name}`}>
      <TableCell>
        <Link to="/app/vendors/$id" params={{ id: v.id }} className="font-medium hover:underline">
          {v.name}
        </Link>
        <div className="text-xs text-muted-foreground">
          {v.category || '—'} {v.domain ? `· ${v.domain}` : ''}
          {v.status === 'discovered' ? ` · via ${connectorName(v.source)} · ${v.discovery_users} user(s)` : ''}
        </div>
        {v.frameworks.length ? (
          <div className="mt-1 flex flex-wrap gap-1">
            {v.frameworks.map((f) => (
              <Badge key={f} variant="outline" className="font-normal">
                {names[f] ?? f}
              </Badge>
            ))}
          </div>
        ) : null}
      </TableCell>
      <TableCell>
        <TierBadge tier={v.tier} />
      </TableCell>
      <TableCell className="text-sm capitalize">
        {v.data_access}
        {v.handles_pii ? <Badge variant="outline" className="ml-1">PII</Badge> : null}
      </TableCell>
      <TableCell>
        <div className="flex gap-1">
          {v.has_security_report ? <Badge variant="success">Report</Badge> : null}
          {v.has_dpa ? <Badge variant="success">DPA</Badge> : null}
        </div>
      </TableCell>
      <TableCell className="text-sm">
        {v.status === 'discovered' ? (v.discovered_at ? timeAgo(v.discovered_at) : '—') : v.next_review_at ? formatDate(v.next_review_at) : <span className="text-muted-foreground">not assessed</span>}
        {v.status === 'active' && v.review_overdue && v.next_review_at ? <Badge variant="destructive" className="ml-1">overdue</Badge> : null}
      </TableCell>
      <TableCell className="text-sm">{v.owner?.name ?? <span className="text-muted-foreground">—</span>}</TableCell>
    </TableRow>
  )
}

function NewVendorDialog({ onClose }: { onClose: () => void }) {
  const create = useCreateVendor()
  const navigate = useNavigate()
  const [f, setF] = useState({ name: '', domain: '', category: '', data_access: 'none', handles_pii: false })
  const [scope, setScope] = useState<VendorScope>({ frameworks: [], questionnaires: null })
  const [fields, setFields] = useState<Record<string, string>>({})
  return (
    <Dialog open onOpenChange={(o) => !o && onClose()}>
      <DialogContent className="max-h-[90vh] max-w-2xl overflow-y-auto">
        <form
          className="space-y-4"
          onSubmit={(e) => {
            e.preventDefault()
            create.mutate({ ...f, ...scopeInput(scope) }, {
              onSuccess: (v) => {
                onClose()
                void navigate({ to: '/app/vendors/$id', params: { id: v.id } })
              },
              onError: (err) => (isApiError(err) && Object.keys(err.fields).length ? setFields(err.fields) : toast.error(errorMessage(err))),
            })
          }}
        >
          <DialogHeader>
            <DialogTitle>Add vendor</DialogTitle>
            <DialogDescription>Map the vendor to the frameworks it supports and choose the questionnaires it must answer. Tier it after uploading evidence — the AI review recommends a tier.</DialogDescription>
          </DialogHeader>
          <FormField label="Name" htmlFor="vn" error={fields.name}>
            <Input id="vn" value={f.name} onChange={(e) => setF({ ...f, name: e.target.value })} data-testid="vendor-name" />
          </FormField>
          <div className="grid grid-cols-2 gap-3">
            <FormField label="Domain" htmlFor="vd">
              <Input id="vd" value={f.domain} onChange={(e) => setF({ ...f, domain: e.target.value })} placeholder="vendor.com" />
            </FormField>
            <FormField label="Category" htmlFor="vc">
              <Input id="vc" value={f.category} onChange={(e) => setF({ ...f, category: e.target.value })} placeholder="e.g. Storage" />
            </FormField>
          </div>
          <FormField label="Data access" htmlFor="va">
            <Select id="va" value={f.data_access} onChange={(e) => setF({ ...f, data_access: e.target.value })} data-testid="vendor-data-access">
              {['none', 'internal', 'confidential', 'restricted'].map((d) => (
                <option key={d}>{d}</option>
              ))}
            </Select>
          </FormField>
          <label className="flex items-center gap-2 text-sm">
            <input type="checkbox" checked={f.handles_pii} onChange={(e) => setF({ ...f, handles_pii: e.target.checked })} data-testid="vendor-pii" /> Processes personal data
          </label>
          <VendorScopeFields value={scope} onChange={setScope} errors={fields} />
          <DialogFooter>
            <Button type="submit" loading={create.isPending} data-testid="vendor-create">
              Add vendor
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
