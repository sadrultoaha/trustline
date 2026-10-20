import { useState } from 'react'
import { Link, useNavigate } from '@tanstack/react-router'
import { useCreateOrg, useMe } from '@/api/auth'
import { errorMessage } from '@/api/client'
import { AuthLayout } from '@/components/layout/AuthLayout'
import { FormField } from '@/components/layout/FormField'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'

export function OnboardingPage() {
  const { data: me } = useMe()
  const navigate = useNavigate()
  const create = useCreateOrg()
  const [name, setName] = useState('')
  return (
    <AuthLayout
      title={me?.orgs.length ? 'Create another organization' : `Welcome, ${me?.user.name ?? ''}`}
      subtitle="Name the organization you want to bring into compliance. You will be its admin."
      footer={
        me?.orgs.length ? (
          <Link to="/app" className="underline">
            Back to {me.org?.name ?? 'the app'}
          </Link>
        ) : (
          <span>Signed in as {me?.user.email}</span>
        )
      }
    >
      <form
        className="space-y-4"
        onSubmit={(e) => {
          e.preventDefault()
          create.mutate(name, { onSuccess: () => void navigate({ to: '/app' }) })
        }}
        noValidate
      >
        {create.isError ? (
          <Alert variant="destructive">
            <AlertDescription>{errorMessage(create.error)}</AlertDescription>
          </Alert>
        ) : null}
        <FormField label="Organization name" htmlFor="org">
          <Input id="org" value={name} onChange={(e) => setName(e.target.value)} data-testid="onboarding-org-name" autoFocus />
        </FormField>
        <Button type="submit" className="w-full" loading={create.isPending} data-testid="onboarding-submit">
          Create organization
        </Button>
      </form>
    </AuthLayout>
  )
}
