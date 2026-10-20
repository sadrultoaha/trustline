import { Link, Outlet, useNavigate } from '@tanstack/react-router'
import { Building2, Check, ChevronsUpDown, FlaskConical, Layers, LayoutDashboard, ListChecks, ListTodo, LogOut, Plug, Plus, Settings, ShieldCheck, User } from 'lucide-react'
import { toast } from 'sonner'
import { useLogout, useMe, usePermissions, useSwitchOrg } from '@/api/auth'
import { Avatar, AvatarFallback, AvatarImage } from '@/components/ui/avatar'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuLabel, DropdownMenuSeparator, DropdownMenuTrigger } from '@/components/ui/dropdown-menu'
import { cn, initials } from '@/lib/utils'
import { NotificationBell } from './NotificationBell'

const nav = [
  { to: '/app', label: 'Dashboard', icon: LayoutDashboard, exact: true, testid: 'nav-dashboard', perm: '' },
  { to: '/app/frameworks', label: 'Frameworks', icon: Layers, exact: false, testid: 'nav-frameworks', perm: 'controls:read' },
  { to: '/app/controls', label: 'Controls', icon: ListChecks, exact: false, testid: 'nav-controls', perm: 'controls:read' },
  { to: '/app/tests', label: 'Tests', icon: FlaskConical, exact: false, testid: 'nav-tests', perm: 'tests:read' },
  { to: '/app/integrations', label: 'Integrations', icon: Plug, exact: false, testid: 'nav-integrations', perm: 'integrations:read' },
  { to: '/app/tasks', label: 'Tasks', icon: ListTodo, exact: false, testid: 'nav-tasks', perm: 'controls:read' },
  { to: '/app/settings', label: 'Settings', icon: Settings, exact: false, testid: 'nav-settings', perm: '' },
] as const

export function AppShell() {
  const { data: me } = useMe()
  const navigate = useNavigate()
  const logout = useLogout()
  const switchOrg = useSwitchOrg()
  const { can } = usePermissions()
  if (!me || !me.org) return null

  return (
    <div className="flex min-h-screen">
      <aside className="hidden w-60 shrink-0 flex-col border-r bg-muted/30 md:flex">
        <div className="flex h-14 items-center gap-2 border-b px-4 font-semibold">
          <span className="flex h-7 w-7 items-center justify-center rounded-md bg-primary text-primary-foreground">
            <ShieldCheck className="h-4 w-4" />
          </span>
          Trustline
        </div>
        <div className="p-3">
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <button
                data-testid="org-switcher"
                className="flex w-full items-center gap-2 rounded-md border bg-background px-3 py-2 text-left text-sm shadow-sm hover:bg-accent"
              >
                <Building2 className="h-4 w-4 text-muted-foreground" />
                <span className="flex-1 truncate font-medium" data-testid="current-org-name">
                  {me.org.name}
                </span>
                <ChevronsUpDown className="h-4 w-4 text-muted-foreground" />
              </button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="start" className="w-56">
              <DropdownMenuLabel>Organizations</DropdownMenuLabel>
              {me.orgs.map((o) => (
                <DropdownMenuItem
                  key={o.id}
                  data-testid={`org-option-${o.slug}`}
                  onSelect={() => {
                    if (o.id === me.org?.id) return
                    switchOrg.mutate(o.id, {
                      onSuccess: () => {
                        toast.success(`Switched to ${o.name}`)
                        void navigate({ to: '/app' })
                      },
                      onError: () => toast.error('Could not switch organization'),
                    })
                  }}
                >
                  <span className="flex-1 truncate">{o.name}</span>
                  <Badge variant="secondary" className="capitalize">
                    {o.role}
                  </Badge>
                  {o.id === me.org?.id ? <Check className="h-4 w-4" /> : null}
                </DropdownMenuItem>
              ))}
              <DropdownMenuSeparator />
              <DropdownMenuItem onSelect={() => void navigate({ to: '/onboarding' })} data-testid="org-create">
                <Plus className="h-4 w-4" /> Create organization
              </DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu>
        </div>
        <nav className="flex-1 space-y-1 px-3">
          {nav.filter((item) => !item.perm || can(item.perm)).map((item) => (
            <Link
              key={item.to}
              to={item.to}
              data-testid={item.testid}
              activeOptions={{ exact: item.exact }}
              className="flex items-center gap-2 rounded-md px-3 py-2 text-sm text-muted-foreground hover:bg-accent hover:text-foreground"
              activeProps={{ className: cn('bg-accent text-foreground font-medium') }}
            >
              <item.icon className="h-4 w-4" />
              {item.label}
            </Link>
          ))}
        </nav>
        <div className="border-t p-3">
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <button data-testid="user-menu" className="flex w-full items-center gap-2 rounded-md px-2 py-1.5 text-left text-sm hover:bg-accent">
                <Avatar>
                  {me.user.avatar_url ? <AvatarImage src={me.user.avatar_url} alt="" /> : null}
                  <AvatarFallback>{initials(me.user.name)}</AvatarFallback>
                </Avatar>
                <span className="min-w-0 flex-1">
                  <span className="block truncate font-medium">{me.user.name}</span>
                  <span className="block truncate text-xs text-muted-foreground capitalize">{me.role}</span>
                </span>
              </button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="start" className="w-56">
              <DropdownMenuLabel className="truncate">{me.user.email}</DropdownMenuLabel>
              <DropdownMenuSeparator />
              <DropdownMenuItem onSelect={() => void navigate({ to: '/app/settings/profile' })}>
                <User className="h-4 w-4" /> Profile
              </DropdownMenuItem>
              <DropdownMenuItem
                data-testid="logout"
                onSelect={() =>
                  logout.mutate(undefined, {
                    onSuccess: () => void navigate({ to: '/login', search: { next: undefined, error: undefined, invite: undefined } }),
                  })
                }
              >
                <LogOut className="h-4 w-4" /> Sign out
              </DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu>
        </div>
      </aside>
      <div className="flex min-w-0 flex-1 flex-col">
        <header className="flex h-14 items-center justify-between border-b px-4 md:hidden">
          <span className="font-semibold">Trustline</span>
          <Button variant="ghost" size="sm" onClick={() => logout.mutate(undefined, { onSuccess: () => void navigate({ to: '/login', search: { next: undefined, error: undefined, invite: undefined } }) })}>
            Sign out
          </Button>
        </header>
        <div className="hidden h-14 items-center justify-end gap-2 border-b px-6 md:flex">
          <NotificationBell />
        </div>
        <main className="flex-1 p-6 md:p-8">
          <Outlet />
        </main>
      </div>
    </div>
  )
}
