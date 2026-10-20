import { Link, Outlet } from '@tanstack/react-router'
import { usePermissions } from '@/api/auth'
import { cn } from '@/lib/utils'

export function SettingsLayout() {
  const { can } = usePermissions()
  const items = [
    { to: '/app/settings/organization', label: 'Organization', show: can('org:read') },
    { to: '/app/settings/members', label: 'Members', show: can('members:read') },
    { to: '/app/settings/activity', label: 'Activity log', show: can('activity:read') },
    { to: '/app/settings/profile', label: 'Your profile', show: true },
  ].filter((i) => i.show)
  return (
    <div className="flex flex-col gap-6 md:flex-row">
      <nav className="flex gap-1 md:w-48 md:flex-col" data-testid="settings-nav">
        {items.map((i) => (
          <Link
            key={i.to}
            to={i.to}
            className={cn('rounded-md px-3 py-2 text-sm text-muted-foreground hover:bg-accent hover:text-foreground')}
            activeProps={{ className: 'bg-accent text-foreground font-medium' }}
            data-testid={`settings-nav-${i.label.toLowerCase().split(' ')[0]}`}
          >
            {i.label}
          </Link>
        ))}
      </nav>
      <div className="min-w-0 flex-1">
        <Outlet />
      </div>
    </div>
  )
}
