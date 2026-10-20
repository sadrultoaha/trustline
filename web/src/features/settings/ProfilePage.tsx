import { useState } from 'react'
import { toast } from 'sonner'
import { useChangePassword, useMe, useUpdateProfile } from '@/api/auth'
import { errorMessage } from '@/api/client'
import type { Me } from '@/api/types'
import { FormField } from '@/components/layout/FormField'
import { PageHeader } from '@/components/layout/PageHeader'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'

export function ProfilePage() {
  const { data: me } = useMe()
  if (!me) return null
  return <ProfileForm key={me.user.id} me={me} />
}

function ProfileForm({ me }: { me: Me }) {
  const update = useUpdateProfile()
  const changePw = useChangePassword()
  const [name, setName] = useState(me.user.name)
  const [pw, setPw] = useState({ current_password: '', new_password: '' })
  return (
    <div>
      <PageHeader title="Your profile" />
      <Card>
        <CardHeader>
          <CardTitle>Details</CardTitle>
        </CardHeader>
        <CardContent>
          <form
            className="grid gap-4 md:grid-cols-2"
            onSubmit={(e) => {
              e.preventDefault()
              update.mutate({ name }, { onSuccess: () => toast.success('Profile updated'), onError: (err) => toast.error(errorMessage(err)) })
            }}
          >
            <FormField label="Name" htmlFor="pname">
              <Input id="pname" value={name} onChange={(e) => setName(e.target.value)} data-testid="profile-name" />
            </FormField>
            <FormField label="Email" htmlFor="pemail">
              <Input id="pemail" value={me.user.email} disabled readOnly />
            </FormField>
            <div className="md:col-span-2">
              <Button type="submit" loading={update.isPending} data-testid="profile-save">
                Save
              </Button>
            </div>
          </form>
        </CardContent>
      </Card>
      <Card className="mt-6">
        <CardHeader>
          <CardTitle>Password</CardTitle>
          <CardDescription>{me.user.has_password ? 'Change the password you use to sign in.' : 'You sign in with Google. Set a password to also sign in with email.'}</CardDescription>
        </CardHeader>
        <CardContent>
          <form
            className="grid gap-4 md:grid-cols-2"
            onSubmit={(e) => {
              e.preventDefault()
              changePw.mutate(pw, {
                onSuccess: () => {
                  toast.success('Password updated')
                  setPw({ current_password: '', new_password: '' })
                },
                onError: (err) => toast.error(errorMessage(err)),
              })
            }}
          >
            {me.user.has_password ? (
              <FormField label="Current password" htmlFor="cur">
                <Input id="cur" type="password" autoComplete="current-password" value={pw.current_password} onChange={(e) => setPw({ ...pw, current_password: e.target.value })} />
              </FormField>
            ) : null}
            <FormField label="New password" htmlFor="new" hint="At least 12 characters.">
              <Input id="new" type="password" autoComplete="new-password" value={pw.new_password} onChange={(e) => setPw({ ...pw, new_password: e.target.value })} />
            </FormField>
            <div className="md:col-span-2">
              <Button type="submit" variant="outline" loading={changePw.isPending}>
                Update password
              </Button>
            </div>
          </form>
        </CardContent>
      </Card>
    </div>
  )
}
