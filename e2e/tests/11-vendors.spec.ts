import { fileURLToPath } from 'node:url'
import { expect, test } from '@playwright/test'
import { ACME, login, unique, waitForEmail } from './helpers'

// Module 5 acceptance: upload the sample SOC 2 PDF from /testdata → AI
// assessment with citations to page and section.
test.describe.configure({ timeout: 180_000 })

const SOC2 = fileURLToPath(new URL('../../testdata/vendors/northwind-soc2-type2.pdf', import.meta.url))

test('upload SOC 2 report → AI assessment with page/section citations → apply tier', async ({ page }) => {
  await login(page, ACME.admin.email)
  await page.getByTestId('nav-vendors').click()
  await page.getByTestId('vendor-new').click()
  const name = unique('Fabrikam Backup')
  await page.getByTestId('vendor-name').fill(name)
  await page.getByTestId('vendor-data-access').selectOption('restricted')
  await page.getByTestId('vendor-pii').check()
  await page.getByTestId('vendor-create').click()
  await expect(page.getByTestId('vendor-tier').first()).toHaveText('unassessed')

  await page.getByTestId('vendor-upload-open').click()
  await page.getByTestId('vendor-doc-kind').selectOption('soc2_type2')
  await page.getByTestId('vendor-doc-file').setInputFiles(SOC2)
  await page.getByTestId('vendor-doc-submit').click()
  await expect(page.getByTestId('vendor-doc-soc2_type2')).toBeVisible()

  const a = page.getByTestId('vendor-assessment')
  await expect(a).toBeVisible({ timeout: 60_000 })
  await expect(page.getByTestId('vendor-opinion')).toContainText('unqualified')
  await expect(page.getByTestId('vendor-summary').getByTestId('citation')).toContainText('p. 3')
  const exceptions = page.getByTestId('vendor-exceptions')
  await expect(exceptions).toContainText('CC6.2')
  await expect(exceptions).toContainText('CC7.1')
  await expect(exceptions.getByTestId('citation').first()).toContainText('p. 8')
  await expect(exceptions.getByTestId('citation').first()).toContainText('Section 4')
  await expect(a).toContainText('CUECs')

  await page.getByTestId('vendor-apply-tier').click()
  await expect(page.getByTestId('vendor-recommended')).toHaveCount(0)
  await expect(page.getByTestId('vendor-tier').first()).not.toHaveText('unassessed')
})

test('discovered vendor triage and questionnaire through the vendor portal', async ({ page, browser, request }) => {
  await login(page, ACME.admin.email)
  await page.getByTestId('nav-vendors').click()
  // The seed is idempotent, so an earlier run may have dismissed the app: restore it first.
  await page.getByTestId('vendors-tab-dismissed').click()
  await expect(page.getByTestId('vendors-table')).toBeVisible()
  const dismissed = page.getByTestId('vendor-row-PDF Magic Converter')
  if (await dismissed.count()) {
    await dismissed.getByRole('link').click()
    await page.getByTestId('vendor-restore').click()
  } else {
    await page.getByTestId('vendors-tab-discovered').click()
    await page.getByTestId('vendor-row-PDF Magic Converter').getByRole('link').click()
  }
  await page.getByTestId('vendor-dismiss').click()
  await expect(page.getByTestId('vendor-restore')).toBeVisible()
  await page.getByTestId('nav-vendors').click()
  await page.getByTestId('vendors-tab-discovered').click()
  await expect(page.getByTestId('vendors-table')).toBeVisible()
  await expect(page.getByTestId('vendor-row-PDF Magic Converter')).toHaveCount(0)

  await page.getByRole('tab', { name: 'Inventory' }).click()
  await page.getByTestId('vendor-row-Northwind Cloud Storage').getByRole('link').click()
  await expect(page.getByTestId('vendor-assessment')).toBeVisible()
  await page.getByTestId('vendor-send-questionnaire').click()
  const to = `${unique('security')}@northwind.example`
  await page.getByTestId('vendor-questionnaire-email').fill(to)
  await page.getByTestId('vendor-questionnaire-send').click()
  const link = (await page.getByTestId('vendor-portal-link').textContent())!
  const mail = await waitForEmail(request, to, 'questionnaire')
  expect(mail.text).toContain(link)

  // The vendor answers without an account.
  const vendor = await (await browser.newContext()).newPage()
  await vendor.goto(link)
  const yes = vendor.locator('[data-testid^="portal-a-"][data-testid$="-Yes"]')
  const n = await yes.count()
  expect(n).toBeGreaterThan(10)
  for (let i = 0; i < n; i++) await yes.nth(i).click()
  await vendor.getByTestId('portal-a-certifications-Both').click()
  // One honest "No" and the free-text answers.
  await vendor.locator('[data-testid^="portal-a-"][data-testid$="-No"]').first().click()
  for (const t of await vendor.locator('textarea[data-testid^="portal-a-"]').all()) await t.fill('See our trust center.')
  await vendor.getByTestId('portal-save').click()
  await vendor.reload()
  await expect(vendor.getByTestId('portal-progress')).toContainText('answered')
  await vendor.getByTestId('portal-submit').click()
  await expect(vendor.getByTestId('portal-submitted')).toBeVisible()

  await page.reload()
  await expect(page.getByTestId('vendor-questionnaire').filter({ hasText: to })).toContainText('submitted')
})
