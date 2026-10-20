import { expect, test } from '@playwright/test'
import { ACME, GLOBEX, login } from './helpers'

test('a Globex admin cannot read or mutate Acme data, and the org switcher only lists own orgs', async ({ browser, page }) => {
  // Collect Acme identifiers as the Acme admin.
  await login(page, ACME.admin.email)
  const acmeMembers = (await (await page.request.get('/api/v1/org/members')).json()) as { items: { id: string; email: string }[] }
  const acmeOrg = (await (await page.request.get('/api/v1/org')).json()) as { id: string }
  const acmeMemberId = acmeMembers.items.find((m) => m.email === ACME.editor.email)!.id

  // Globex admin in a separate context.
  const ctx = await browser.newContext()
  const g = await ctx.newPage()
  await login(g, GLOBEX.admin.email)
  await expect(g.getByTestId('current-org-name')).toHaveText('Globex Corporation')
  await g.getByTestId('org-switcher').click()
  await expect(g.getByTestId('org-option-globex')).toBeVisible()
  await expect(g.getByTestId('org-option-acme')).toHaveCount(0)
  await g.keyboard.press('Escape')

  expect((await g.request.get(`/api/v1/org/members/${acmeMemberId}`)).status()).toBe(404)
  expect((await g.request.patch(`/api/v1/org/members/${acmeMemberId}`, { data: { role: 'employee' } })).status()).toBe(404)
  expect((await g.request.delete(`/api/v1/org/members/${acmeMemberId}`)).status()).toBe(404)
  expect((await g.request.post('/api/v1/me/switch-org', { data: { org_id: acmeOrg.id } })).status()).toBe(404)

  const globexMembers = (await (await g.request.get('/api/v1/org/members')).json()) as { items: { email: string }[] }
  expect(globexMembers.items.map((m) => m.email)).not.toContain(ACME.admin.email)

  // Acme's editor is untouched.
  const still = (await (await page.request.get(`/api/v1/org/members/${acmeMemberId}`)).json()) as { role: string }
  expect(still.role).toBe('editor')
  await ctx.close()
})
