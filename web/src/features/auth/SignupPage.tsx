import { useState } from 'react'
import { Link, useNavigate, useSearch } from '@tanstack/react-router'
import { useSignup } from '@/api/auth'
import { isApiError } from '@/api/client'
import { AuthLayout } from '@/components/layout/AuthLayout'
import { FormField } from '@/components/layout/FormField'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { GoogleButton } from './GoogleButton'

export function SignupPage() {
  const search = useSearch({ from: '/signup' })
  const navigate = useNavigate()
  const signup = useSignup()
  const [form, setForm] = useState({ name: '', email: '', password: '', org_name: '' })
  const [fields, setFields] = useState<Record<string, string>>({})
  const [error, setError] = useState<string | null>(null)
  const set = (k: keyof typeof form) => (e: React.ChangeEvent<HTMLInputElement>) => setForm({ ...form, [k]: e.target.value })

  const onSubmit = (e: React.FormEvent) => {
    e.preventDefault()
    setFields({})
    setError(null)
    signup.mutate(
      { ...form, invite_token: search.invite },
      {
        onSuccess: () => void navigate({ to: '/app' }),
        onError: (err) => {
          if (isApiError(err) && Object.keys(err.fields).length) setFields(err.fields)
          else setError(err.message)
        },
      },
    )
  }

  return (
    <AuthLayout
      title={search.invite ? 'Create your account' : 'Create your organization'}
      subtitle={search.invite ? 'Finish setting up your account to join the organization.' : 'Start automating compliance in minutes.'}
      footer={
        <>
          Already have an account?{' '}
          <Link to="/login" search={{ next: undefined, error: undefined, invite: search.invite }} className="text-foreground underline">
            Sign in
          </Link>
        </>
      }
    >
      <form onSubmit={onSubmit} className="space-y-4" noValidate>
        {error ? (
          <Alert variant="destructive" data-testid="signup-error">
            <AlertDescription>{error}</AlertDescription>
          </Alert>
        ) : null}
        <FormField label="Your name" htmlFor="name" error={fields.name}>
          <Input id="name" value={form.name} onChange={set('name')} data-testid="signup-name" aria-invalid={!!fields.name} />
        </FormField>
        <FormField label="Work email" htmlFor="email" error={fields.email}>
          <Input id="email" type="email" autoComplete="email" value={form.email} onChange={set('email')} data-testid="signup-email" aria-invalid={!!fields.email} />
        </FormField>
        <FormField label="Password" htmlFor="password" error={fields.password} hint="At least 12 characters.">
          <Input id="password" type="password" autoComplete="new-password" value={form.password} onChange={set('password')} data-testid="signup-password" aria-invalid={!!fields.password} />
        </FormField>
        {!search.invite ? (
          <FormField label="Organization name" htmlFor="org_name" error={fields.org_name}>
            <Input id="org_name" value={form.org_name} onChange={set('org_name')} data-testid="signup-org" aria-invalid={!!fields.org_name} />
          </FormField>
        ) : null}
        {fields.invite_token ? (
          <Alert variant="destructive">
            <AlertDescription>{fields.invite_token}</AlertDescription>
          </Alert>
        ) : null}
        <Button type="submit" className="w-full" loading={signup.isPending} data-testid="signup-submit">
          {search.invite ? 'Create account & join' : 'Create organization'}
        </Button>
        <div className="relative py-2 text-center text-xs text-muted-foreground">
          <span className="relative z-10 bg-card px-2">or</span>
          <div className="absolute inset-x-0 top-1/2 -z-0 border-t" />
        </div>
        <GoogleButton invite={search.invite} label="Sign up with Google" />
      </form>
    </AuthLayout>
  )
}
