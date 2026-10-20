import { useState } from 'react'
import { Link, useNavigate } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import { CheckCircle2, FlaskConical, Plug } from 'lucide-react'
import { toast } from 'sonner'
import { usePermissions } from '@/api/auth'
import { errorMessage, isApiError } from '@/api/client'
import { integrationTypesQuery, integrationsQuery, useConnectIntegration, type IntegrationType } from '@/api/compliance'
import { FormField } from '@/components/layout/FormField'
import { PageHeader } from '@/components/layout/PageHeader'
import { ConnectorLogo } from '@/components/status'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { timeAgo } from '@/lib/utils'

export function IntegrationsPage() {
  const types = useQuery(integrationTypesQuery())
  const integrations = useQuery(integrationsQuery())
  const byType = Object.fromEntries((integrations.data?.items ?? []).map((i) => [i.type, i]))
  const [connecting, setConnecting] = useState<IntegrationType | null>(null)
  const { can } = usePermissions()
  return (
    <div>
      <PageHeader title="Integrations" description="Connected systems are synced on a schedule; every sync runs the automated tests for your controls." />
      <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
        {types.data?.items.map((t) => {
          const i = byType[t.type]
          return (
            <Card key={t.type} className="flex flex-col" data-testid={`integration-card-${t.type}`}>
              <CardContent className="flex flex-1 flex-col gap-4 p-5">
                <div className="flex items-start gap-3">
                  <ConnectorLogo type={t.type} />
                  <div className="min-w-0 flex-1">
                    <div className="flex items-center gap-2 font-semibold">
                      {t.name}
                      {i ? (
                        <Badge variant={i.status === 'error' ? 'destructive' : 'success'} data-testid={`integration-status-${t.type}`}>
                          {i.status}
                        </Badge>
                      ) : null}
                    </div>
                    <div className="text-xs text-muted-foreground">
                      {t.category} · {t.test_count} automated tests
                    </div>
                  </div>
                </div>
                <p className="flex-1 text-sm text-muted-foreground">{t.description}</p>
                {i ? (
                  <div className="flex items-center justify-between text-sm">
                    <span className="text-muted-foreground">
                      {i.resource_count} resources · {i.last_sync_at ? `synced ${timeAgo(i.last_sync_at)}` : 'first sync pending'}
                      {i.tests_failing ? <span className="text-destructive"> · {i.tests_failing} failing</span> : null}
                    </span>
                    <Button asChild size="sm" variant="outline">
                      <Link to="/app/integrations/$id" params={{ id: i.id }} data-testid={`integration-open-${t.type}`}>
                        Manage
                      </Link>
                    </Button>
                  </div>
                ) : can('integrations:write') ? (
                  <Button size="sm" onClick={() => setConnecting(t)} data-testid={`integration-connect-${t.type}`}>
                    <Plug /> Connect
                  </Button>
                ) : null}
              </CardContent>
            </Card>
          )
        })}
      </div>
      {connecting ? <ConnectDialog type={connecting} onClose={() => setConnecting(null)} /> : null}
    </div>
  )
}

function ConnectDialog({ type, onClose }: { type: IntegrationType; onClose: () => void }) {
  const connect = useConnectIntegration()
  const navigate = useNavigate()
  const [creds, setCreds] = useState<Record<string, string>>({})
  const [fields, setFields] = useState<Record<string, string>>({})
  return (
    <Dialog open onOpenChange={(o) => !o && onClose()}>
      <DialogContent className="max-w-xl">
        <form
          className="space-y-4"
          onSubmit={(e) => {
            e.preventDefault()
            setFields({})
            connect.mutate(
              { type: type.type, mode: 'sandbox', credentials: creds },
              {
                onSuccess: (i) => {
                  toast.success(`${type.name} connected — first sync started`)
                  onClose()
                  void navigate({ to: '/app/integrations/$id', params: { id: i.id } })
                },
                onError: (err) => (isApiError(err) && Object.keys(err.fields).length ? setFields(err.fields) : toast.error(errorMessage(err))),
              },
            )
          }}
        >
          <DialogHeader>
            <DialogTitle className="flex items-center gap-2">
              <ConnectorLogo type={type.type} className="h-8 w-8 text-[10px]" /> Connect {type.name}
            </DialogTitle>
            <DialogDescription>Credentials are encrypted at rest (AES-256-GCM) and never shown again.</DialogDescription>
          </DialogHeader>
          <Alert>
            <FlaskConical className="h-4 w-4" />
            <AlertDescription>
              <strong>Sandbox mode.</strong> This build connects to a realistic simulated {type.name} account seeded with your company's people. You can flip settings in the sandbox to see controls fail and recover. Live API clients ship in Phase 6.
            </AlertDescription>
          </Alert>
          {type.credentials.map((f) => (
            <FormField key={f.key} label={f.label} htmlFor={`cred-${f.key}`} error={fields[`credentials.${f.key}`]} hint={f.help}>
              <Input
                id={`cred-${f.key}`}
                type={f.secret ? 'password' : 'text'}
                placeholder={f.placeholder}
                value={creds[f.key] ?? ''}
                onChange={(e) => setCreds({ ...creds, [f.key]: e.target.value })}
                data-testid={`cred-${f.key}`}
                autoComplete="off"
              />
            </FormField>
          ))}
          {fields.type || fields.mode ? (
            <Alert variant="destructive">
              <AlertDescription>{fields.type ?? fields.mode}</AlertDescription>
            </Alert>
          ) : null}
          <DialogFooter>
            <Button type="button" variant="outline" onClick={onClose}>
              Cancel
            </Button>
            <Button type="submit" loading={connect.isPending} data-testid="integration-connect-submit">
              <CheckCircle2 /> Connect
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
