import { useQuery } from '@tanstack/react-query'
import { Link, useNavigate, useParams } from '@tanstack/react-router'
import { useMe } from '@/api/auth'
import { errorMessage } from '@/api/client'
import { invitePreviewQuery, useAcceptInvite } from '@/api/org'
import { AuthLayout } from '@/components/layout/AuthLayout'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { GoogleButton } from '@/features/auth/GoogleButton'

export function InvitePage() {
  const { token } = useParams({ from: '/invite/$token' })
  const preview = useQuery(invitePreviewQuery(token))
  const { data: me } = useMe()
  const accept = useAcceptInvite()
  const navigate = useNavigate()

  if (preview.isLoading) return <AuthLayout title="Loading invitation…">{null}</AuthLayout>
  if (preview.isError || !preview.data) {
    return (
      <AuthLayout title="Invitation not found" subtitle="This invite link is invalid, was revoked, or has already been used.">
        <Button asChild className="w-full">
          <Link to="/login" search={{ next: undefined, error: undefined, invite: undefined }}>
            Go to sign in
          </Link>
        </Button>
      </AuthLayout>
    )
  }
  const inv = preview.data
  const sameUser = me && me.user.email.toLowerCase() === inv.email.toLowerCase()

  return (
    <AuthLayout title={`Join ${inv.org_name}`} subtitle="You have been invited to collaborate on Trustline.">
      <div className="mb-6 rounded-lg border bg-muted/40 p-4 text-sm" data-testid="invite-preview">
        <div className="flex items-center justify-between">
          <span className="text-muted-foreground">Invited email</span>
          <span className="font-medium">{inv.email}</span>
        </div>
        <div className="mt-2 flex items-center justify-between">
          <span className="text-muted-foreground">Role</span>
          <Badge variant="secondary" className="capitalize">
            {inv.role}
          </Badge>
        </div>
      </div>
      {inv.expired ? (
        <Alert variant="destructive">
          <AlertDescription>This invitation has expired. Ask an admin of {inv.org_name} to send a new one.</AlertDescription>
        </Alert>
      ) : me ? (
        sameUser ? (
          <div className="space-y-3">
            {accept.isError ? (
              <Alert variant="destructive">
                <AlertDescription>{errorMessage(accept.error)}</AlertDescription>
              </Alert>
            ) : null}
            <Button className="w-full" loading={accept.isPending} data-testid="invite-accept" onClick={() => accept.mutate(token, { onSuccess: () => void navigate({ to: '/app' }) })}>
              Accept and join as {me.user.name}
            </Button>
          </div>
        ) : (
          <Alert>
            <AlertDescription>
              You are signed in as <strong>{me.user.email}</strong>, but this invite is for <strong>{inv.email}</strong>. Sign out and use that address.
            </AlertDescription>
          </Alert>
        )
      ) : (
        <div className="space-y-3">
          <Button asChild className="w-full">
            <Link to="/signup" search={{ invite: token }} data-testid="invite-signup">
              Create an account to join
            </Link>
          </Button>
          <GoogleButton invite={token} />
          <p className="text-center text-xs text-muted-foreground">
            Already have an account?{' '}
            <Link to="/login" search={{ next: `/invite/${token}`, error: undefined, invite: token }} className="underline">
              Sign in
            </Link>
          </p>
        </div>
      )}
    </AuthLayout>
  )
}
