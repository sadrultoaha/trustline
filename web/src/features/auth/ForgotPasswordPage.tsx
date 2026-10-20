import { useState } from 'react'
import { Link } from '@tanstack/react-router'
import { useMutation } from '@tanstack/react-query'
import { api, errorMessage } from '@/api/client'
import { AuthLayout } from '@/components/layout/AuthLayout'
import { FormField } from '@/components/layout/FormField'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'

export function ForgotPasswordPage() {
  const [email, setEmail] = useState('')
  const m = useMutation({ mutationFn: (email: string) => api.post('/auth/forgot-password', { email }) })
  return (
    <AuthLayout
      title="Reset your password"
      subtitle="Enter your email and we will send you a reset link."
      footer={
        <Link to="/login" search={{ next: undefined, error: undefined, invite: undefined }} className="underline">
          Back to sign in
        </Link>
      }
    >
      {m.isSuccess ? (
        <Alert variant="success" data-testid="forgot-sent">
          <AlertDescription>If an account exists for {email}, a reset link is on its way. Check your inbox.</AlertDescription>
        </Alert>
      ) : (
        <form
          className="space-y-4"
          onSubmit={(e) => {
            e.preventDefault()
            m.mutate(email)
          }}
          noValidate
        >
          {m.isError ? (
            <Alert variant="destructive">
              <AlertDescription>{errorMessage(m.error)}</AlertDescription>
            </Alert>
          ) : null}
          <FormField label="Email" htmlFor="email">
            <Input id="email" type="email" value={email} onChange={(e) => setEmail(e.target.value)} data-testid="forgot-email" required />
          </FormField>
          <Button type="submit" className="w-full" loading={m.isPending} data-testid="forgot-submit">
            Send reset link
          </Button>
        </form>
      )}
    </AuthLayout>
  )
}
