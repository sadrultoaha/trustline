import { useQuery } from '@tanstack/react-query'
import { api } from '@/api/client'
import { Button } from '@/components/ui/button'

export function GoogleButton({ invite, next, label = 'Continue with Google', divider }: { invite?: string; next?: string; label?: string; divider?: boolean }) {
  // Hidden when the server runs with GOOGLE_OAUTH_MODE=off.
  const providers = useQuery({ queryKey: ['auth-providers'], queryFn: () => api.get<{ google: boolean }>('/auth/providers'), staleTime: Infinity })
  if (!providers.data?.google) return null
  const qs = new URLSearchParams()
  if (invite) qs.set('invite', invite)
  if (next) qs.set('next', next)
  const href = `/auth/google/start${qs.size ? `?${qs.toString()}` : ''}`
  const button = (
    <Button asChild variant="outline" className="w-full" type="button">
      <a href={href} data-testid="login-google">
        <svg className="h-4 w-4" viewBox="0 0 24 24" aria-hidden="true">
          <path fill="#4285F4" d="M22.56 12.25c0-.78-.07-1.53-.2-2.25H12v4.26h5.92a5.06 5.06 0 0 1-2.2 3.32v2.76h3.57c2.08-1.92 3.27-4.74 3.27-8.09z" />
          <path fill="#34A853" d="M12 23c2.97 0 5.46-.98 7.28-2.66l-3.57-2.76c-.98.66-2.23 1.06-3.71 1.06-2.86 0-5.29-1.93-6.16-4.53H2.18v2.84A11 11 0 0 0 12 23z" />
          <path fill="#FBBC05" d="M5.84 14.11a6.6 6.6 0 0 1 0-4.22V7.05H2.18a11 11 0 0 0 0 9.9l3.66-2.84z" />
          <path fill="#EA4335" d="M12 5.38c1.62 0 3.06.56 4.21 1.64l3.15-3.15C17.45 2.09 14.97 1 12 1A11 11 0 0 0 2.18 7.05l3.66 2.84c.87-2.6 3.3-4.51 6.16-4.51z" />
        </svg>
        {label}
      </a>
    </Button>
  )
  if (!divider) return button
  return (
    <>
      <div className="relative py-2 text-center text-xs text-muted-foreground">
        <span className="relative z-10 bg-card px-2">or</span>
        <div className="absolute inset-x-0 top-1/2 -z-0 border-t" />
      </div>
      {button}
    </>
  )
}
