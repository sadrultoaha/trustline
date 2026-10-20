import { expect, test, type Page } from '@playwright/test'
import { ACME, login, unique } from './helpers'

// Module 4 acceptance: a new failing test on a critical control raises a
// suggested risk; accepting it puts it in the register linked to the control.
test.describe.configure({ timeout: 240_000 })

async function setScenario(page: Page, on: boolean) {
  await page.getByTestId('nav-integrations').click()
  await page.getByTestId('integration-open-aws').click()
  const toggle = page.getByTestId('scenario-public_bucket')
  if ((await toggle.getAttribute('aria-checked')) !== String(on)) await toggle.click()
  await expect(toggle).toHaveAttribute('aria-checked', String(on))
  await page.getByTestId('integration-sync').click()
}

test('critical test failure → suggested risk → accept → linked to DAT-01', async ({ page }) => {
  await login(page, ACME.admin.email)
  await setScenario(page, true)

  await page.getByTestId('nav-risks').click()
  await expect(page.getByTestId('risk-heatmap')).toBeVisible()
  await page.getByTestId('risks-tab-suggested').click()
  const row = page.getByTestId('risks-suggested').getByRole('row').filter({ hasText: 'No S3 buckets are publicly accessible' })
  await expect(row).toBeVisible({ timeout: 90_000 })
  await expect(row).toContainText('20')
  await row.getByRole('link').click()
  await expect(page.getByTestId('risk-rationale')).toContainText('DAT-01')
  await expect(page.getByTestId('risk-controls')).toContainText('DAT-01')
  await page.getByTestId('risk-detail-accept').click()
  await expect(page.getByTestId('risk-status')).toHaveText('open')
  const riskUrl = page.url()

  // The control page shows the risk it mitigates.
  await page.getByTestId('nav-controls').click()
  await page.getByTestId('controls-search').fill('DAT-01')
  await page.getByTestId('control-row-DAT-01').getByRole('link').click()
  await expect(page.getByTestId('control-risks')).toContainText('No S3 buckets are publicly accessible')

  // Remediate and close the risk so the spec is repeatable without a reseed.
  await setScenario(page, false)
  await page.goto(riskUrl)
  await page.getByTestId('risk-edit-status').selectOption('closed')
  await page.getByTestId('risk-save').click()
  await expect(page.getByTestId('risk-status')).toHaveText('closed')
})

test('manual risk, heatmap and Complete with AI', async ({ page }) => {
  await login(page, ACME.admin.email)
  await page.getByTestId('nav-risks').click()
  const title = unique('Key person dependency')
  await page.getByTestId('risk-new').click()
  await page.getByTestId('risk-title').fill(title)
  await page.getByTestId('risk-likelihood').selectOption('2')
  await page.getByTestId('risk-impact').selectOption('4')
  await page.getByTestId('risk-create').click()
  await expect(page.getByTestId('risk-score').first()).toContainText('8')
  const opt = page.getByTestId('risk-link-control').locator('option', { hasText: 'GOV-01' })
  await page.getByTestId('risk-link-control').selectOption((await opt.getAttribute('value'))!)
  await expect(page.getByTestId('risk-controls')).toContainText('GOV-01')

  await page.getByTestId('nav-risks').click()
  await expect(page.getByTestId('heat-2-4')).toHaveText(/^[1-9]\d*$/)
  await page.getByTestId('risk-ai-assess').click()
  await expect(page.getByTestId('risks-suggested')).toBeVisible({ timeout: 30_000 })
  await expect(page.getByTestId('risk-last-assessment')).not.toBeEmpty()
})

test('auditor can read the register but not change it', async ({ page }) => {
  await login(page, ACME.auditor.email)
  await page.getByTestId('nav-risks').click()
  await expect(page.getByTestId('risks-table')).toBeVisible()
  await expect(page.getByTestId('risk-new')).toHaveCount(0)
  await expect(page.getByTestId('risk-ai-assess')).toHaveCount(0)
})
