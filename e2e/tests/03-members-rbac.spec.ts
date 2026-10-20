import { expect, test } from '@playwright/test'
import { ACME, extractLink, login, logout, unique, waitForEmail } from './helpers'

test.describe('Members, invites and RBAC', () => {
  test('admin invites an editor; invitee joins via email link; roles are enforced', async ({ browser, page, request }) => {
    const inviteeEmail = `${unique('invitee')}@example.test`

    await login(page, ACME.admin.email)
    await page.getByTestId('nav-settings').click()
    await page.getByTestId('settings-nav-members').click()
    await page.getByTestId('invite-open').click()
    await page.getByTestId('invite-email').fill(inviteeEmail)
    await page.getByTestId('invite-role').selectOption('editor')
    await page.getByTestId('invite-submit').click()
    await expect(page.getByTestId(`invite-row-${inviteeEmail}`)).toBeVisible()

    // The worker sends the invite through mailpit.
    const mail = await waitForEmail(request, inviteeEmail, 'Join Acme Corp')
    const link = extractLink(mail.text, '/invite/')

    // Invitee opens the link in a fresh browser context.
    const ctx = await browser.newContext()
    const invitee = await ctx.newPage()
    await invitee.goto(link.replace(/^https?:\/\/[^/]+/, ''))
    await expect(invitee.getByTestId('invite-preview')).toContainText(inviteeEmail)
    await invitee.getByTestId('invite-signup').click()
    await expect(invitee).toHaveURL(/\/signup\?invite=/)
    await invitee.getByTestId('signup-name').fill('New Editor')
    await invitee.getByTestId('signup-email').fill(inviteeEmail)
    await invitee.getByTestId('signup-password').fill('editor-password-123456')
    await invitee.getByTestId('signup-submit').click()
    await expect(invitee).toHaveURL(/\/app$/)
    await expect(invitee.getByTestId('current-org-name')).toHaveText('Acme Corp')
    await expect(invitee.getByText('signed in as editor')).toBeVisible()

    // Editor can view members but has no invite button; API refuses writes.
    await invitee.goto('/app/settings/members')
    await expect(invitee.getByTestId('members-table')).toBeVisible()
    await expect(invitee.getByTestId('invite-open')).toHaveCount(0)
    const forbidden = await invitee.request.post('/api/v1/org/invites', { data: { email: 'x@example.test', role: 'editor' } })
    expect(forbidden.status()).toBe(403)
    // Organization settings are read-only for the editor.
    await invitee.goto('/app/settings/organization')
    await expect(invitee.getByTestId('org-readonly')).toBeVisible()
    await expect(invitee.getByTestId('org-save')).toHaveCount(0)

    // Admin sees the new member, invite is gone, changes the role, activity log records it.
    await page.reload()
    await expect(page.getByTestId(`member-row-${inviteeEmail}`)).toBeVisible()
    await expect(page.getByTestId(`invite-row-${inviteeEmail}`)).toHaveCount(0)
    await page.getByTestId(`member-role-${inviteeEmail}`).selectOption('employee')
    await expect(page.getByText('New Editor is now employee')).toBeVisible()

    await page.getByTestId('settings-nav-activity').click()
    const rows = page.getByTestId('activity-row')
    await expect(rows.filter({ hasText: 'member.role_changed' }).first()).toContainText(inviteeEmail)
    await expect(rows.filter({ hasText: 'invite.accepted' }).first()).toContainText(inviteeEmail)

    // Now an employee: settings nav hides members/activity; members API is 403.
    await invitee.goto('/app/settings')
    await expect(invitee.getByTestId('settings-nav-members')).toHaveCount(0)
    const emp = await invitee.request.get('/api/v1/org/members')
    expect(emp.status()).toBe(403)

    // Remove the member; their next request to the org fails.
    await page.getByTestId('settings-nav-members').click()
    await page.getByTestId(`member-menu-${inviteeEmail}`).click()
    await page.getByTestId(`member-remove-${inviteeEmail}`).click()
    await page.getByTestId('member-remove-confirm').click()
    await expect(page.getByTestId(`member-row-${inviteeEmail}`)).toHaveCount(0)
    const gone = await invitee.request.get('/api/v1/org')
    expect(gone.status()).toBe(403)
    await ctx.close()
  })

  test('auditor is read-only everywhere', async ({ page }) => {
    await login(page, ACME.auditor.email)
    await expect(page.getByText('signed in as auditor')).toBeVisible()
    await page.goto('/app/settings/organization')
    await expect(page.getByTestId('org-readonly')).toBeVisible()
    await page.goto('/app/settings/members')
    await expect(page.getByTestId('members-table')).toBeVisible()
    await expect(page.getByTestId('invite-open')).toHaveCount(0)
    const r = await page.request.patch('/api/v1/org', { data: { name: 'Nope' } })
    expect(r.status()).toBe(403)
    await page.goto('/app/settings/activity')
    await expect(page.getByTestId('activity-row').first()).toBeVisible()
    await logout(page)
  })

  test('admin edits organization settings', async ({ page }) => {
    await login(page, ACME.admin.email)
    await page.goto('/app/settings/organization')
    await page.getByTestId('org-access-review-days').fill('0')
    await page.getByTestId('org-save').click()
    await expect(page.getByTestId('error-ar')).toContainText('between 1 and 365')
    await page.getByTestId('org-access-review-days').fill('90')
    await page.getByTestId('org-save').click()
    await expect(page.getByText('Organization updated')).toBeVisible()
  })
})
