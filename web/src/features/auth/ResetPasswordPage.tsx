import { useState } from 'react'
import { Link, useNavigate, useSearch } from '@tanstack/react-router'
import { useMutation } from '@tanstack/react-query'
import { api, errorMessage } from '@/api/client'
import { AuthLayout } from '@/components/layout/AuthLayout'
import { FormField } from '@/components/layout/FormField'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'

export function ResetPasswordPage() {
  const { token } = useSearch({ from: '/reset-password' })
  const navigate = useNavigate()
  const [password, setPassword] = useState('')
  const [confirm, setConfirm] = useState('')
  const [localErr, setLocalErr] = useState<string | null>(null)
  const m = useMutation({
    mutationFn: () => api.post('/auth/reset-password', { token, password }),
    onSuccess: () => void navigate({ to: '/login', search: { next: undefined, error: undefined, invite: undefined } }),
  })
  return (
    <AuthLayout
      title="Choose a new password"
      footer={
        <Link to="/login" search={{ next: undefined, error: undefined, invite: undefined }} className="underline">
          Back to sign in
        </Link>
      }
    >
      <form
        className="space-y-4"
        onSubmit={(e) => {
          e.preventDefault()
          setLocalErr(null)
          if (password !== confirm) {
            setLocalErr('Passwords do not match')
            return
          }
          m.mutate()
        }}
        noValidate
      >
        {!token ? (
          <Alert variant="destructive">
            <AlertDescription>This reset link is missing its token. Request a new one.</AlertDescription>
          </Alert>
        ) : null}
        {localErr || m.isError ? (
          <Alert variant="destructive" data-testid="reset-error">
            <AlertDescription>{localErr ?? errorMessage(m.error)}</AlertDescription>
          </Alert>
        ) : null}
        <FormField label="New password" htmlFor="password" hint="At least 12 characters.">
          <Input id="password" type="password" autoComplete="new-password" value={password} onChange={(e) => setPassword(e.target.value)} data-testid="reset-password" />
        </FormField>
        <FormField label="Confirm password" htmlFor="confirm">
          <Input id="confirm" type="password" autoComplete="new-password" value={confirm} onChange={(e) => setConfirm(e.target.value)} data-testid="reset-confirm" />
        </FormField>
        <Button type="submit" className="w-full" loading={m.isPending} disabled={!token} data-testid="reset-submit">
          Set new password
        </Button>
      </form>
    </AuthLayout>
  )
}
