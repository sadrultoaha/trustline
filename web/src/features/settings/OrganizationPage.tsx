import { useState } from 'react'
import { toast } from 'sonner'
import { usePermissions } from '@/api/auth'
import { errorMessage, isApiError } from '@/api/client'
import { useOrg, useUpdateOrg } from '@/api/org'
import type { Org, OrgSettings } from '@/api/types'
import { FormField } from '@/components/layout/FormField'
import { PageHeader } from '@/components/layout/PageHeader'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Switch } from '@/components/ui/switch'

export function OrganizationPage() {
  const { data: org, isLoading } = useOrg()
  if (isLoading || !org) return <p className="text-sm text-muted-foreground">Loading…</p>
  // Keyed on updated_at so the form re-initialises after a save or org switch.
  return <OrganizationForm key={`${org.id}-${org.updated_at}`} org={org} />
}

function OrganizationForm({ org }: { org: Org }) {
  const { can } = usePermissions()
  const update = useUpdateOrg()
  const readOnly = !can('org:write')
  const [name, setName] = useState(org.name)
  const [settings, setSettings] = useState<OrgSettings>(org.settings)
  const [fields, setFields] = useState<Record<string, string>>({})

  const num = (k: keyof OrgSettings) => (e: React.ChangeEvent<HTMLInputElement>) => setSettings({ ...settings, [k]: Number(e.target.value) })

  return (
    <div>
      <PageHeader title="Organization" description="General settings. Values here are referenced by policy consistency checks." />
      {readOnly ? (
        <Alert className="mb-4" data-testid="org-readonly">
          <AlertDescription>Your role is read-only here. Only admins can change organization settings.</AlertDescription>
        </Alert>
      ) : null}
      <form
        className="space-y-6"
        noValidate
        onSubmit={(e) => {
          e.preventDefault()
          setFields({})
          update.mutate(
            { name, settings },
            {
              onSuccess: () => toast.success('Organization updated'),
              onError: (err) => {
                if (isApiError(err) && Object.keys(err.fields).length) setFields(err.fields)
                toast.error(errorMessage(err))
              },
            },
          )
        }}
      >
        <Card>
          <CardHeader>
            <CardTitle>Profile</CardTitle>
          </CardHeader>
          <CardContent className="grid gap-4 md:grid-cols-2">
            <FormField label="Organization name" htmlFor="org-name" error={fields.name}>
              <Input id="org-name" value={name} onChange={(e) => setName(e.target.value)} disabled={readOnly} data-testid="org-name" />
            </FormField>
            <FormField label="Slug" htmlFor="org-slug" hint="Used for your public trust center URL.">
              <Input id="org-slug" value={org.slug} disabled readOnly />
            </FormField>
            <FormField label="Timezone" htmlFor="tz" error={fields['settings.timezone']}>
              <Input id="tz" value={settings.timezone} onChange={(e) => setSettings({ ...settings, timezone: e.target.value })} disabled={readOnly} data-testid="org-timezone" />
            </FormField>
          </CardContent>
        </Card>
        <Card>
          <CardHeader>
            <CardTitle>Security baseline</CardTitle>
            <CardDescription>These values are compared against your published policies to detect inconsistencies.</CardDescription>
          </CardHeader>
          <CardContent className="grid gap-4 md:grid-cols-2">
            <FormField label="Access review cadence (days)" htmlFor="ar" error={fields['settings.access_review_days']}>
              <Input id="ar" type="number" min={1} max={365} value={settings.access_review_days} onChange={num('access_review_days')} disabled={readOnly} data-testid="org-access-review-days" />
            </FormField>
            <FormField label="Minimum password length" htmlFor="pml" error={fields['settings.password_min_length']}>
              <Input id="pml" type="number" min={8} max={128} value={settings.password_min_length} onChange={num('password_min_length')} disabled={readOnly} data-testid="org-password-min" />
            </FormField>
            <FormField label="Policy acceptance threshold (%)" htmlFor="pat" error={fields['settings.policy_acceptance_threshold']} hint="The 'policies acknowledged' control passes when at least this share of employees accepted.">
              <Input id="pat" type="number" min={1} max={100} value={settings.policy_acceptance_threshold} onChange={num('policy_acceptance_threshold')} disabled={readOnly} data-testid="org-policy-threshold" />
            </FormField>
            <div className="space-y-1.5">
              <span className="text-sm font-medium">AI features</span>
              <div className="flex items-center gap-3 pt-1">
                <Switch checked={settings.ai_enabled} onCheckedChange={(v) => setSettings({ ...settings, ai_enabled: v })} disabled={readOnly} data-testid="org-ai-enabled" />
                <span className="text-sm text-muted-foreground">{settings.ai_enabled ? 'Enabled: drafts, validation and the assistant use the AI provider.' : 'Disabled: no data is sent to the AI provider.'}</span>
              </div>
            </div>
          </CardContent>
        </Card>
        <Card>
          <CardHeader>
            <CardTitle>Notifications</CardTitle>
            <CardDescription>Where control status changes are announced.</CardDescription>
          </CardHeader>
          <CardContent className="grid gap-4 md:grid-cols-2">
            <FormField label="Slack incoming webhook URL" htmlFor="slack" error={fields['settings.slack_webhook_url']} hint="Failing and recovered controls are posted to this channel.">
              <Input id="slack" value={settings.slack_webhook_url ?? ''} onChange={(e) => setSettings({ ...settings, slack_webhook_url: e.target.value })} disabled={readOnly} placeholder="https://hooks.slack.com/services/…" data-testid="org-slack" />
            </FormField>
            <div className="space-y-1.5">
              <span className="text-sm font-medium">Email alerts</span>
              <div className="flex items-center gap-3 pt-1">
                <Switch checked={settings.email_notifications} onCheckedChange={(v) => setSettings({ ...settings, email_notifications: v })} disabled={readOnly} data-testid="org-email-notifications" />
                <span className="text-sm text-muted-foreground">Email admins, editors and control owners when a control fails or recovers.</span>
              </div>
            </div>
          </CardContent>
        </Card>
        {!readOnly ? (
          <div className="flex justify-end">
            <Button type="submit" loading={update.isPending} data-testid="org-save">
              Save changes
            </Button>
          </div>
        ) : null}
      </form>
    </div>
  )
}
