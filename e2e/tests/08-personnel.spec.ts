import { expect, test, type Page } from '@playwright/test'
import { ACME, login } from './helpers'

test.describe.configure({ timeout: 240_000 })

const TOM = 'tom@acme.test'

async function openIntegration(page: Page, type: string) {
  await page.getByTestId('nav-integrations').click()
  await page.getByTestId(`integration-open-${type}`).click()
  await expect(page.getByTestId('sandbox-panel')).toBeVisible()
}

async function personScenario(page: Page, type: string, key: string, email: string) {
  await openIntegration(page, type)
  const applied = page.getByTestId(`applied-${key}:${email}`)
  if (!(await applied.count())) {
    await page.getByTestId('person-scenario-email').selectOption(email)
    await page.getByTestId(`person-scenario-${key}`).click()
    await expect(applied).toBeVisible()
  }
  await page.getByTestId('integration-sync').click()
}

async function undoScenario(page: Page, type: string, key: string, email: string) {
  await openIntegration(page, type)
  const applied = page.getByTestId(`applied-${key}:${email}`)
  if (await applied.count()) {
    await applied.getByRole('button', { name: 'Undo' }).click()
    await expect(applied).toHaveCount(0)
  }
  await page.getByTestId('integration-sync').click()
}

async function idpTestStatus(page: Page): Promise<string | null> {
  await page.goto('/app/tests?')
  const row = page.getByTestId('test-row-trustline.people.terminated_idp_deactivated')
  return row.getByTestId('test-status').getAttribute('data-status')
}

// Module 6 acceptance: terminate in the HRIS → offboarding tasks → the
// "access revoked" test fails until Okta shows the user deactivated.
test('terminate in HRIS → offboarding → IdP test fails until Okta deactivation', async ({ page }) => {
  await login(page, ACME.admin.email)
  try {
    expect(await idpTestStatus(page)).toBe('pass')
    await personScenario(page, 'hris', 'terminate', TOM)

    // Person is terminated with a verified-offboarding checklist.
    await page.getByTestId('nav-people').click()
    await page.getByTestId('people-tab-terminated').click()
    await expect(page.getByTestId(`person-row-${TOM}`)).toBeVisible({ timeout: 30_000 })
    await page.getByTestId(`person-row-${TOM}`).getByRole('link').click()
    const checklist = page.getByTestId('checklist-offboarding')
    await expect(checklist).toBeVisible()
    await expect(checklist.getByTestId('checklist-item-okta')).toHaveAttribute('data-status', 'open')
    await expect(checklist.getByTestId('checklist-item-github')).toHaveAttribute('data-status', 'open')
    await expect(page.getByTestId('access-okta')).toHaveAttribute('data-active', 'true')
    const personUrl = page.url()

    await expect.poll(() => idpTestStatus(page), { timeout: 60_000, intervals: [2000] }).toBe('fail')

    // Deactivate in Okta → test passes and the Okta item verifies itself.
    await personScenario(page, 'okta', 'deactivate', TOM)
    await expect.poll(() => idpTestStatus(page), { timeout: 60_000, intervals: [2000] }).toBe('pass')
    await page.goto(personUrl.replace(/^https?:\/\/[^/]+/, ''))
    await expect(page.getByTestId('checklist-offboarding').getByTestId('checklist-item-okta')).toHaveAttribute('data-status', 'done', { timeout: 30_000 })
    await expect(page.getByTestId('checklist-offboarding').getByTestId('checklist-item-github')).toHaveAttribute('data-status', 'open')
  } finally {
    // Restore the sandbox for later runs (the seed restores Tom's Trustline membership).
    await undoScenario(page, 'okta', 'deactivate', TOM)
    await undoScenario(page, 'hris', 'terminate', TOM)
  }
})

test('device agent enrollment and report', async ({ page }) => {
  await login(page, ACME.admin.email)
  await page.getByTestId('nav-devices').click()
  await page.getByTestId('device-token-rotate').click()
  const token = (await page.getByTestId('device-token-value').textContent())!.trim()
  expect(token).toMatch(/^tld_/)
  const serial = `E2E${Date.now()}`
  const res = await page.request.post('/api/v1/agent/report', {
    headers: { Authorization: `Bearer ${token}` },
    data: { serial, hostname: 'e2e-laptop', owner_email: 'grace@acme.test', platform: 'macos', os_version: '15.1', disk_encrypted: false, screen_lock_minutes: 5, antivirus_running: true, antivirus_name: 'XProtect', agent_version: '1.0.0' },
  })
  expect(res.status()).toBe(200)
  await page.reload()
  const row = page.getByTestId(`device-row-${serial}`)
  await expect(row).toContainText('Off')
  await expect(row).toContainText('issue')
  // Clean up so disk-encryption stays green for the demo.
  await row.getByTitle('Remove device').click()
  await expect(row).toHaveCount(0)
  const script = await page.request.get('/agent/trustline-agent.sh')
  expect(await script.text()).toContain('TRUSTLINE_TOKEN')
})
