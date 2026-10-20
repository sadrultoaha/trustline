import type { QueryClient } from '@tanstack/react-query'
import { Outlet, createRootRouteWithContext, createRoute, createRouter, redirect } from '@tanstack/react-router'
import { Toaster } from 'sonner'
import { meQueryOptions } from '@/api/auth'
import { AppShell } from '@/components/layout/AppShell'
import { ForgotPasswordPage } from '@/features/auth/ForgotPasswordPage'
import { LoginPage } from '@/features/auth/LoginPage'
import { ResetPasswordPage } from '@/features/auth/ResetPasswordPage'
import { SignupPage } from '@/features/auth/SignupPage'
import { ControlDetailPage } from '@/features/controls/ControlDetailPage'
import { ControlsPage } from '@/features/controls/ControlsPage'
import { DashboardPage } from '@/features/dashboard/DashboardPage'
import { FrameworkDetailPage } from '@/features/frameworks/FrameworkDetailPage'
import { FrameworksPage } from '@/features/frameworks/FrameworksPage'
import { IntegrationDetailPage } from '@/features/integrations/IntegrationDetailPage'
import { IntegrationsPage } from '@/features/integrations/IntegrationsPage'
import { TaskDetailPage } from '@/features/tasks/TaskDetailPage'
import { TasksPage } from '@/features/tasks/TasksPage'
import { TestBuilderPage } from '@/features/tests/TestBuilderPage'
import { TestDetailPage } from '@/features/tests/TestDetailPage'
import { TestsPage } from '@/features/tests/TestsPage'
import { MyWorkPage } from '@/features/mywork/MyWorkPage'
import { TrainingPage } from '@/features/mywork/TrainingPage'
import { DevicesPage } from '@/features/people/DevicesPage'
import { PeoplePage } from '@/features/people/PeoplePage'
import { PersonDetailPage } from '@/features/people/PersonDetailPage'
import { PoliciesPage } from '@/features/policies/PoliciesPage'
import { PolicyDetailPage } from '@/features/policies/PolicyDetailPage'
import { AccessReviewsPage } from '@/features/reviews/AccessReviewsPage'
import { CampaignPage } from '@/features/reviews/CampaignPage'
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

const frameworksRoute = createRoute({ getParentRoute: () => appRoute, path: 'frameworks', component: FrameworksPage })
const frameworkRoute = createRoute({ getParentRoute: () => appRoute, path: 'frameworks/$id', component: FrameworkDetailPage })
const controlsRoute = createRoute({ getParentRoute: () => appRoute, path: 'controls', component: ControlsPage })
const controlRoute = createRoute({ getParentRoute: () => appRoute, path: 'controls/$id', component: ControlDetailPage })
const integrationsRoute = createRoute({ getParentRoute: () => appRoute, path: 'integrations', component: IntegrationsPage })
const integrationRoute = createRoute({ getParentRoute: () => appRoute, path: 'integrations/$id', component: IntegrationDetailPage })
const testsRoute = createRoute({ getParentRoute: () => appRoute, path: 'tests', component: TestsPage })
const testNewRoute = createRoute({ getParentRoute: () => appRoute, path: 'tests/new', component: TestBuilderPage })
const testRoute = createRoute({ getParentRoute: () => appRoute, path: 'tests/$id', component: TestDetailPage })
const tasksRoute = createRoute({ getParentRoute: () => appRoute, path: 'tasks', component: TasksPage })
const taskRoute = createRoute({ getParentRoute: () => appRoute, path: 'tasks/$id', component: TaskDetailPage })

const policiesRoute = createRoute({ getParentRoute: () => appRoute, path: 'policies', component: PoliciesPage })
const policyRoute = createRoute({ getParentRoute: () => appRoute, path: 'policies/$id', component: PolicyDetailPage })
const peopleRoute = createRoute({ getParentRoute: () => appRoute, path: 'people', component: PeoplePage })
const personRoute = createRoute({ getParentRoute: () => appRoute, path: 'people/$id', component: PersonDetailPage })
const devicesRoute = createRoute({ getParentRoute: () => appRoute, path: 'devices', component: DevicesPage })
const reviewsRoute = createRoute({ getParentRoute: () => appRoute, path: 'access-reviews', component: AccessReviewsPage })
const reviewRoute = createRoute({ getParentRoute: () => appRoute, path: 'access-reviews/$id', component: CampaignPage })
const myWorkRoute = createRoute({ getParentRoute: () => appRoute, path: 'my-work', component: MyWorkPage })
const trainingRoute = createRoute({ getParentRoute: () => appRoute, path: 'training', component: TrainingPage })

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
    frameworksRoute,
    frameworkRoute,
    controlsRoute,
    controlRoute,
    integrationsRoute,
    integrationRoute,
    testsRoute,
    testNewRoute,
    testRoute,
    tasksRoute,
    taskRoute,
    policiesRoute,
    policyRoute,
    peopleRoute,
    personRoute,
    devicesRoute,
    reviewsRoute,
    reviewRoute,
    myWorkRoute,
    trainingRoute,
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
