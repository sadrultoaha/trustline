import type { QueryClient } from '@tanstack/react-query'
import { Outlet, createRootRouteWithContext, createRoute, createRouter, redirect } from '@tanstack/react-router'
import { Toaster } from 'sonner'
import { meQueryOptions } from '@/api/auth'
import { AppShell } from '@/components/layout/AppShell'
import { ForgotPasswordPage } from '@/features/auth/ForgotPasswordPage'
import { LoginPage } from '@/features/auth/LoginPage'
import { ResetPasswordPage } from '@/features/auth/ResetPasswordPage'
import { SignupPage } from '@/features/auth/SignupPage'
import { DashboardPage } from '@/features/dashboard/DashboardPage'
import { InvitePage } from '@/features/invite/InvitePage'
import { OnboardingPage } from '@/features/onboarding/OnboardingPage'
import { ActivityPage } from '@/features/settings/ActivityPage'
import { MembersPage } from '@/features/settings/MembersPage'
import { OrganizationPage } from '@/features/settings/OrganizationPage'
import { ProfilePage } from '@/features/settings/ProfilePage'
import { SettingsLayout } from '@/features/settings/SettingsLayout'

export interface RouterContext {
  queryClient: QueryClient
}

const rootRoute = createRootRouteWithContext<RouterContext>()({
  component: () => (
    <>
      <Outlet />
      <Toaster richColors position="top-right" closeButton />
    </>
  ),
  notFoundComponent: () => (
    <div className="flex min-h-screen items-center justify-center text-muted-foreground">Page not found</div>
  ),
})

const str = (v: unknown) => (typeof v === 'string' && v !== '' ? v : undefined)

const indexRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/',
  beforeLoad: () => {
    throw redirect({ to: '/app' })
  },
})

// ---- public ----------------------------------------------------------------

const loginRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/login',
  validateSearch: (s: Record<string, unknown>) => ({ next: str(s.next), error: str(s.error), invite: str(s.invite) }),
  beforeLoad: async ({ context, search }) => {
    const me = await context.queryClient.ensureQueryData(meQueryOptions)
    if (me && !search.invite) throw redirect({ to: me.org ? '/app' : '/onboarding' })
  },
  component: LoginPage,
})

const signupRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/signup',
  validateSearch: (s: Record<string, unknown>) => ({ invite: str(s.invite) }),
  component: SignupPage,
})

const forgotRoute = createRoute({ getParentRoute: () => rootRoute, path: '/forgot-password', component: ForgotPasswordPage })

const resetRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/reset-password',
  validateSearch: (s: Record<string, unknown>) => ({ token: str(s.token) ?? '' }),
  component: ResetPasswordPage,
})

const inviteRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/invite/$token',
  loader: ({ context }) => context.queryClient.ensureQueryData(meQueryOptions),
  component: InvitePage,
})

// ---- authenticated ---------------------------------------------------------

const onboardingRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/onboarding',
  beforeLoad: async ({ context }) => {
    const me = await context.queryClient.ensureQueryData(meQueryOptions)
    if (!me) throw redirect({ to: '/login', search: { next: '/onboarding', error: undefined, invite: undefined } })
  },
  component: OnboardingPage,
})

const appRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/app',
  beforeLoad: async ({ context, location }) => {
    const me = await context.queryClient.ensureQueryData(meQueryOptions)
    if (!me) throw redirect({ to: '/login', search: { next: location.href, error: undefined, invite: undefined } })
    if (!me.org) throw redirect({ to: '/onboarding' })
    return { me }
  },
  component: AppShell,
})

const dashboardRoute = createRoute({ getParentRoute: () => appRoute, path: '/', component: DashboardPage })

const settingsRoute = createRoute({ getParentRoute: () => appRoute, path: 'settings', component: SettingsLayout })
const settingsIndexRoute = createRoute({
  getParentRoute: () => settingsRoute,
  path: '/',
  beforeLoad: () => {
    throw redirect({ to: '/app/settings/organization' })
  },
})
const settingsOrgRoute = createRoute({ getParentRoute: () => settingsRoute, path: 'organization', component: OrganizationPage })
const settingsMembersRoute = createRoute({ getParentRoute: () => settingsRoute, path: 'members', component: MembersPage })
const settingsActivityRoute = createRoute({ getParentRoute: () => settingsRoute, path: 'activity', component: ActivityPage })
const settingsProfileRoute = createRoute({ getParentRoute: () => settingsRoute, path: 'profile', component: ProfilePage })

const routeTree = rootRoute.addChildren([
  indexRoute,
  loginRoute,
  signupRoute,
  forgotRoute,
  resetRoute,
  inviteRoute,
  onboardingRoute,
  appRoute.addChildren([
    dashboardRoute,
    settingsRoute.addChildren([settingsIndexRoute, settingsOrgRoute, settingsMembersRoute, settingsActivityRoute, settingsProfileRoute]),
  ]),
])

export function createAppRouter(queryClient: QueryClient) {
  return createRouter({
    routeTree,
    context: { queryClient },
    defaultPreload: 'intent',
    defaultPendingMs: 200,
    defaultPendingComponent: () => <div className="p-8 text-sm text-muted-foreground">Loading…</div>,
  })
}
