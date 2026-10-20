import { useState } from 'react'
import { MoreHorizontal, UserPlus } from 'lucide-react'
import { toast } from 'sonner'
import { useMe, usePermissions } from '@/api/auth'
import { errorMessage, isApiError } from '@/api/client'
import { useCreateInvite, useInvites, useMembers, useRemoveMember, useResendInvite, useRevokeInvite, useUpdateMemberRole } from '@/api/org'
import { ROLES, type Member, type Role } from '@/api/types'
import { FormField } from '@/components/layout/FormField'
import { PageHeader } from '@/components/layout/PageHeader'
import { Avatar, AvatarFallback, AvatarImage } from '@/components/ui/avatar'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuLabel, DropdownMenuSeparator, DropdownMenuTrigger } from '@/components/ui/dropdown-menu'
import { Input } from '@/components/ui/input'
import { Select } from '@/components/ui/select'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { formatDate, initials } from '@/lib/utils'

const roleVariant: Record<Role, 'default' | 'secondary' | 'outline' | 'warning'> = { admin: 'default', editor: 'secondary', auditor: 'warning', employee: 'outline' }

export function MembersPage() {
  const { data: me } = useMe()
  const { can } = usePermissions()
  const members = useMembers()
  const invites = useInvites()
  const canWrite = can('members:write')
  const [inviteOpen, setInviteOpen] = useState(false)
  const [removing, setRemoving] = useState<Member | null>(null)
  const updateRole = useUpdateMemberRole()
  const remove = useRemoveMember()
  const revoke = useRevokeInvite()
  const resend = useResendInvite()

  return (
    <div>
      <PageHeader
        title="Members"
        description="People with access to this organization and their roles."
        actions={
          canWrite ? (
            <Button onClick={() => setInviteOpen(true)} data-testid="invite-open">
              <UserPlus /> Invite member
            </Button>
          ) : null
        }
      />
      <Card>
        <CardHeader>
          <CardTitle>Members ({members.data?.items.length ?? 0})</CardTitle>
        </CardHeader>
        <CardContent>
          <Table data-testid="members-table">
            <TableHeader>
              <TableRow>
                <TableHead>Person</TableHead>
                <TableHead>Role</TableHead>
                <TableHead>Last sign-in</TableHead>
                <TableHead className="w-10" />
              </TableRow>
            </TableHeader>
            <TableBody>
              {members.data?.items.map((m) => (
                <TableRow key={m.id} data-testid={`member-row-${m.email}`}>
                  <TableCell>
                    <div className="flex items-center gap-3">
                      <Avatar>
                        {m.avatar_url ? <AvatarImage src={m.avatar_url} alt="" /> : null}
                        <AvatarFallback>{initials(m.name)}</AvatarFallback>
                      </Avatar>
                      <div>
                        <div className="font-medium">
                          {m.name} {m.user_id === me?.user.id ? <span className="text-xs text-muted-foreground">(you)</span> : null}
                        </div>
                        <div className="text-xs text-muted-foreground">{m.email}</div>
                      </div>
                    </div>
                  </TableCell>
                  <TableCell>
                    {canWrite && m.user_id !== me?.user.id ? (
                      <Select
                        value={m.role}
                        data-testid={`member-role-${m.email}`}
                        className="h-8 w-36"
                        onChange={(e) => {
                          const role = e.target.value as Role
                          updateRole.mutate({ id: m.id, role }, { onSuccess: () => toast.success(`${m.name} is now ${role}`), onError: (err) => toast.error(errorMessage(err)) })
                        }}
                      >
                        {ROLES.map((r) => (
                          <option key={r.value} value={r.value}>
                            {r.label}
                          </option>
                        ))}
                      </Select>
                    ) : (
                      <Badge variant={roleVariant[m.role]} className="capitalize" data-testid={`member-role-badge-${m.email}`}>
                        {m.role}
                      </Badge>
                    )}
                  </TableCell>
                  <TableCell className="text-muted-foreground">{formatDate(m.last_login_at, true)}</TableCell>
                  <TableCell>
                    {canWrite && m.user_id !== me?.user.id ? (
                      <DropdownMenu>
                        <DropdownMenuTrigger asChild>
                          <Button variant="ghost" size="icon" data-testid={`member-menu-${m.email}`}>
                            <MoreHorizontal />
                          </Button>
                        </DropdownMenuTrigger>
                        <DropdownMenuContent align="end">
                          <DropdownMenuLabel>{m.email}</DropdownMenuLabel>
                          <DropdownMenuSeparator />
                          <DropdownMenuItem className="text-destructive" onSelect={() => setRemoving(m)} data-testid={`member-remove-${m.email}`}>
                            Remove from organization
                          </DropdownMenuItem>
                        </DropdownMenuContent>
                      </DropdownMenu>
                    ) : null}
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </CardContent>
      </Card>

      <Card className="mt-6">
        <CardHeader>
          <CardTitle>Pending invites ({invites.data?.items.length ?? 0})</CardTitle>
          <CardDescription>Invites expire after 7 days. Resending issues a fresh link.</CardDescription>
        </CardHeader>
        <CardContent>
          {invites.data?.items.length ? (
            <Table data-testid="invites-table">
              <TableHeader>
                <TableRow>
                  <TableHead>Email</TableHead>
                  <TableHead>Role</TableHead>
                  <TableHead>Expires</TableHead>
                  <TableHead className="w-10" />
                </TableRow>
              </TableHeader>
              <TableBody>
                {invites.data.items.map((i) => (
                  <TableRow key={i.id} data-testid={`invite-row-${i.email}`}>
                    <TableCell className="font-medium">{i.email}</TableCell>
                    <TableCell>
                      <Badge variant={roleVariant[i.role]} className="capitalize">
                        {i.role}
                      </Badge>
                    </TableCell>
                    <TableCell className="text-muted-foreground">{formatDate(i.expires_at)}</TableCell>
                    <TableCell>
                      {canWrite ? (
                        <DropdownMenu>
                          <DropdownMenuTrigger asChild>
                            <Button variant="ghost" size="icon" data-testid={`invite-menu-${i.email}`}>
                              <MoreHorizontal />
                            </Button>
                          </DropdownMenuTrigger>
                          <DropdownMenuContent align="end">
                            <DropdownMenuItem onSelect={() => resend.mutate(i.id, { onSuccess: () => toast.success('Invite resent'), onError: (e) => toast.error(errorMessage(e)) })}>
                              Resend email
                            </DropdownMenuItem>
                            <DropdownMenuItem
                              className="text-destructive"
                              data-testid={`invite-revoke-${i.email}`}
                              onSelect={() => revoke.mutate(i.id, { onSuccess: () => toast.success('Invite revoked'), onError: (e) => toast.error(errorMessage(e)) })}
                            >
                              Revoke
                            </DropdownMenuItem>
                          </DropdownMenuContent>
                        </DropdownMenu>
                      ) : null}
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          ) : (
            <p className="text-sm text-muted-foreground">No pending invites.</p>
          )}
        </CardContent>
      </Card>

      <InviteDialog open={inviteOpen} onOpenChange={setInviteOpen} />

      <Dialog open={!!removing} onOpenChange={(o) => !o && setRemoving(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Remove {removing?.name}?</DialogTitle>
            <DialogDescription>They will lose access to {me?.org?.name} immediately. Their activity history is kept.</DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setRemoving(null)}>
              Cancel
            </Button>
            <Button
              variant="destructive"
              loading={remove.isPending}
              data-testid="member-remove-confirm"
              onClick={() =>
                removing &&
                remove.mutate(removing.id, {
                  onSuccess: () => {
                    toast.success(`${removing.name} removed`)
                    setRemoving(null)
                  },
                  onError: (e) => toast.error(errorMessage(e)),
                })
              }
            >
              Remove
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}

function InviteDialog({ open, onOpenChange }: { open: boolean; onOpenChange: (o: boolean) => void }) {
  const create = useCreateInvite()
  const [email, setEmail] = useState('')
  const [role, setRole] = useState<Role>('editor')
  const [fields, setFields] = useState<Record<string, string>>({})
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <form
          className="space-y-4"
          onSubmit={(e) => {
            e.preventDefault()
            setFields({})
            create.mutate(
              { email, role },
              {
                onSuccess: () => {
                  toast.success(`Invite sent to ${email}`)
                  setEmail('')
                  onOpenChange(false)
                },
                onError: (err) => {
                  if (isApiError(err) && Object.keys(err.fields).length) setFields(err.fields)
                  else toast.error(errorMessage(err))
                },
              },
            )
          }}
          noValidate
        >
          <DialogHeader>
            <DialogTitle>Invite a member</DialogTitle>
            <DialogDescription>They will receive an email with a link that is valid for 7 days.</DialogDescription>
          </DialogHeader>
          <FormField label="Email" htmlFor="invite-email" error={fields.email}>
            <Input id="invite-email" type="email" value={email} onChange={(e) => setEmail(e.target.value)} data-testid="invite-email" autoFocus />
          </FormField>
          <FormField label="Role" htmlFor="invite-role" error={fields.role} hint={ROLES.find((r) => r.value === role)?.description}>
            <Select id="invite-role" value={role} onChange={(e) => setRole(e.target.value as Role)} data-testid="invite-role">
              {ROLES.map((r) => (
                <option key={r.value} value={r.value}>
                  {r.label}
                </option>
              ))}
            </Select>
          </FormField>
          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
              Cancel
            </Button>
            <Button type="submit" loading={create.isPending} data-testid="invite-submit">
              Send invite
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
