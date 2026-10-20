import { useState } from 'react'
import { Link, useNavigate, useSearch } from '@tanstack/react-router'
import { useLogin } from '@/api/auth'
import { errorMessage } from '@/api/client'
import { AuthLayout } from '@/components/layout/AuthLayout'
import { FormField } from '@/components/layout/FormField'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { GoogleButton } from './GoogleButton'

export function LoginPage() {
  const search = useSearch({ from: '/login' })
  const navigate = useNavigate()
  const login = useLogin()
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState<string | null>(search.error ?? null)

  const onSubmit = (e: React.FormEvent) => {
    e.preventDefault()
    setError(null)
    login.mutate(
      { email, password },
      {
        onSuccess: (me) => {
          const next = search.next && search.next.startsWith('/') ? search.next : me.org ? '/app' : '/onboarding'
          void navigate({ to: next })
        },
        onError: (err) => setError(errorMessage(err)),
      },
    )
  }

  return (
    <AuthLayout
      title="Sign in"
      subtitle="Welcome back. Sign in to your organization."
      footer={
        <>
          New here?{' '}
          <Link to="/signup" search={{ invite: search.invite }} className="text-foreground underline">
            Create an organization
          </Link>
        </>
      }
    >
      <form onSubmit={onSubmit} className="space-y-4" noValidate>
        {error ? (
          <Alert variant="destructive" data-testid="login-error">
            <AlertDescription>{error}</AlertDescription>
          </Alert>
        ) : null}
        <FormField label="Email" htmlFor="email">
          <Input id="email" type="email" autoComplete="email" value={email} onChange={(e) => setEmail(e.target.value)} data-testid="login-email" required />
        </FormField>
        <FormField label="Password" htmlFor="password">
          <Input id="password" type="password" autoComplete="current-password" value={password} onChange={(e) => setPassword(e.target.value)} data-testid="login-password" required />
        </FormField>
        <Button type="submit" className="w-full" loading={login.isPending} data-testid="login-submit">
          Sign in
        </Button>
        <div className="text-right text-xs">
          <Link to="/forgot-password" className="text-muted-foreground underline">
            Forgot your password?
          </Link>
        </div>
        <div className="relative py-2 text-center text-xs text-muted-foreground">
          <span className="relative z-10 bg-card px-2">or</span>
          <div className="absolute inset-x-0 top-1/2 -z-0 border-t" />
        </div>
        <GoogleButton invite={search.invite} next={search.next} />
      </form>
    </AuthLayout>
  )
}
