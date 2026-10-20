import { useNavigate } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import { Bell } from 'lucide-react'
import { notificationsQuery, useReadNotifications } from '@/api/compliance'
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuLabel, DropdownMenuSeparator, DropdownMenuTrigger } from '@/components/ui/dropdown-menu'
import { cn, timeAgo } from '@/lib/utils'

export function NotificationBell() {
  const { data } = useQuery(notificationsQuery())
  const read = useReadNotifications()
  const navigate = useNavigate()
  const unread = data?.unread ?? 0
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <button className="relative rounded-md p-2 hover:bg-accent" data-testid="notifications-bell" aria-label="Notifications">
          <Bell className="h-5 w-5" />
          {unread ? (
            <span className="absolute -right-0.5 -top-0.5 flex h-5 min-w-5 items-center justify-center rounded-full bg-destructive px-1 text-[10px] font-semibold text-destructive-foreground" data-testid="notifications-unread">
              {unread}
            </span>
          ) : null}
        </button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="w-96">
        <div className="flex items-center justify-between">
          <DropdownMenuLabel>Notifications</DropdownMenuLabel>
          {unread ? (
            <button className="px-2 text-xs text-muted-foreground underline" onClick={() => read.mutate([])}>
              Mark all read
            </button>
          ) : null}
        </div>
        <DropdownMenuSeparator />
        {data?.items.length ? (
          data.items.slice(0, 12).map((n) => (
            <DropdownMenuItem
              key={n.id}
              className="flex flex-col items-start gap-0.5"
              data-testid="notification-item"
              onSelect={() => {
                if (!n.read_at) read.mutate([n.id])
                if (n.link) void navigate({ to: n.link })
              }}
            >
              <span className={cn('text-sm', !n.read_at && 'font-semibold')}>{n.title}</span>
              <span className="line-clamp-2 text-xs text-muted-foreground">{n.body}</span>
              <span className="text-[10px] text-muted-foreground">{timeAgo(n.created_at)}</span>
            </DropdownMenuItem>
          ))
        ) : (
          <p className="px-2 py-6 text-center text-sm text-muted-foreground">You're all caught up.</p>
        )}
      </DropdownMenuContent>
    </DropdownMenu>
  )
}
