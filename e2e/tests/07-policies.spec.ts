import { expect, test, type Browser, type Page } from '@playwright/test'
import { ACME, DEMO_PASSWORD, login } from './helpers'

test.describe.configure({ timeout: 240_000 })

async function openPolicy(page: Page, slug: string) {
  await page.getByTestId('nav-policies').click()
  await page.getByTestId(`policy-row-${slug}`).getByRole('link').click()
  await expect(page.getByTestId('policy-editor')).toBeVisible()
}

async function controlStatus(page: Page, code: string): Promise<string | null> {
  await page.getByTestId('nav-controls').click()
  await page.getByTestId('controls-search').fill(code)
  return page.getByTestId(`control-row-${code}`).getByTestId('control-status').getAttribute('data-status')
}

async function asUser(browser: Browser, email: string, fn: (p: Page) => Promise<void>) {
  const ctx = await browser.newContext()
  const p = await ctx.newPage()
  await login(p, email, DEMO_PASSWORD)
  await fn(p)
  await ctx.close()
}

// Module 3 acceptance: publish v2 → everyone gets an acknowledgement task →
// "policies acknowledged" (GOV-05) fails until the threshold is met.
test('policy v2: edit → approve → publish → GOV-05 fails until staff acknowledge', async ({ browser, page }) => {
  await login(page, ACME.admin.email)
  const members = (await (await page.request.get('/api/v1/org/members')).json()) as { items: { email: string; role: string }[] }
  const audience = members.items.filter((m) => m.role !== 'auditor').map((m) => m.email)

  // Admin starts v2 and edits it in the rich-text editor.
  await openPolicy(page, 'information-security')
  await page.getByTestId('policy-new-version').click()
  await expect(page.getByTestId('policy-version-status')).toHaveText('draft')
  const editor = page.getByTestId('policy-editor').locator('.ProseMirror')
  await editor.click()
  await page.keyboard.press('ControlOrMeta+End')
  await page.keyboard.type(' All administrators must use phishing-resistant security keys.')
  await page.getByTestId('policy-change-summary').fill('Require security keys for administrators')
  await page.getByTestId('policy-save').click()
  await expect(page.getByText('Draft saved')).toBeVisible()
  await page.getByTestId('policy-submit-open').click()
  await page.getByTestId('policy-approver').selectOption({ label: 'Grace Hopper (editor)' })
  await page.getByTestId('policy-submit').click()
  await expect(page.getByTestId('policy-version-status')).toHaveText('in review')
  const policyUrl = page.url()

  // Grace approves.
  await asUser(browser, ACME.editor.email, async (g) => {
    await g.goto(policyUrl.replace(/^https?:\/\/[^/]+/, ''))
    await g.getByTestId('policy-approve').click()
    await expect(g.getByTestId('policy-version-status')).toHaveText('approved')
  })

  // Admin publishes.
  await page.reload()
  await page.getByTestId('policy-publish').click()
  await expect(page.getByTestId('policy-version-status')).toHaveText('published')
  await expect(page.getByTestId('policy-editor')).toContainText('phishing-resistant security keys')

  // GOV-05 turns red once the platform re-evaluates (within seconds).
  await expect.poll(() => controlStatus(page, 'GOV-05'), { timeout: 60_000, intervals: [2000] }).toBe('failing')

  // Every member acknowledges from My work.
  for (const email of audience) {
    await asUser(browser, email, async (u) => {
      await u.getByTestId('nav-my-work').click()
      const loaded = u.getByTestId('my-pending-policies').or(u.getByTestId('policies-all-done'))
      await expect(loaded).toBeVisible()
      // Acknowledge everything pending (including leftovers from earlier runs).
      while (await u.getByTestId('my-policy-open').count()) {
        await u.getByTestId('my-policy-open').first().click()
        await u.getByTestId('policy-agree').check()
        await u.getByTestId('policy-acknowledge').click()
        await expect(u.getByTestId('policy-acknowledged')).toBeVisible()
        await u.getByTestId('nav-my-work').click()
        await expect(loaded).toBeVisible()
      }
      await expect(u.getByTestId('policies-all-done')).toBeVisible()
    })
  }
  await expect.poll(() => controlStatus(page, 'GOV-05'), { timeout: 60_000, intervals: [2000] }).toBe('passing')
  await openPolicy(page, 'information-security')
  await expect(page.getByTestId('policy-acknowledgements')).not.toContainText('pending')
})

test('AI draft from the library and policy/settings consistency check', async ({ page }) => {
  await login(page, ACME.admin.email)
  // AI-drafted policy from a template (idempotent across runs).
  await page.getByTestId('nav-policies').click()
  await page.getByTestId('policy-library-open').click()
  const add = page.getByTestId('library-add-password')
  if (await add.count()) {
    await add.click()
  } else {
    await page.getByTestId('library-password').getByRole('link', { name: 'Open' }).click()
  }
  await expect(page.getByTestId('policy-editor')).toContainText('Password')
  if (await page.getByTestId('policy-ai-draft-open').count()) {
    await page.getByTestId('policy-ai-draft-open').click()
    await page.getByTestId('policy-ai-instructions').fill('Service accounts must use 32-character generated secrets.')
    await page.getByTestId('policy-ai-draft').click()
    await expect(page.getByTestId('policy-editor')).toContainText('Systems in scope')
    await expect(page.getByTestId('policy-editor')).toContainText('Amazon Web Services')
  }

  // Consistency: the Access Control policy says reviews every 90 days.
  await openPolicy(page, 'access-control')
  await page.getByTestId('policy-consistency').click()
  await expect(page.getByTestId('policy-consistency-result')).toContainText('Consistent with settings')
  const setDays = async (days: string) => {
    await page.goto('/app/settings/organization')
    await page.getByTestId('org-access-review-days').fill(days)
    await page.getByTestId('org-save').click()
    await expect(page.getByText('Organization updated')).toBeVisible()
  }
  await setDays('180')
  await openPolicy(page, 'access-control')
  await page.getByTestId('policy-consistency').click()
  await expect(page.getByTestId('policy-consistency-result')).toContainText('Policy says 90, settings say 180')
  await setDays('90')
  await openPolicy(page, 'access-control')
  await page.getByTestId('policy-consistency').click()
  await expect(page.getByTestId('policy-consistency-result')).toContainText('Consistent with settings')
})

test('employee completes security training from My work', async ({ page }) => {
  await login(page, 'tom@acme.test')
  await expect(page.getByTestId('employee-home')).toBeVisible()
  await page.getByTestId('training-start').click()
  for (let i = 0; i < 6; i++) await page.getByTestId('training-next').click()
  const answers: Record<string, number> = { q1: 2, q2: 1, q3: 2, q4: 1, q5: 2, q6: 3, q7: 1, q8: 1, q9: 1, q10: 1 }
  for (const [q, a] of Object.entries(answers)) await page.getByTestId(`quiz-${q}-${a}`).check()
  await page.getByTestId('quiz-submit').click()
  await expect(page.getByTestId('quiz-result')).toContainText('Passed')
  await page.getByTestId('nav-my-work').click()
  await expect(page.getByTestId('training-done')).toBeVisible()
  // Employees cannot see admin areas.
  await expect(page.getByTestId('nav-controls')).toHaveCount(0)
  await expect(page.getByTestId('nav-people')).toHaveCount(0)
})
