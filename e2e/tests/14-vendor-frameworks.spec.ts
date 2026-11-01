import { expect, test } from '@playwright/test'
import { unique } from './helpers'

test.describe.configure({ timeout: 120_000 })

test('vendor onboarding: map frameworks → suggested questionnaires → bundle answered in the portal', async ({ page, browser }) => {
  await page.goto('/signup')
  await page.getByTestId('signup-name').fill('GRC Lead')
  await page.getByTestId('signup-email').fill(`${unique('vfw')}@example.test`)
  await page.getByTestId('signup-password').fill('a-very-long-password-123')
  await page.getByTestId('signup-org').fill(`Vendor Scope ${Date.now()}`)
  await page.getByTestId('signup-submit').click()
  await expect(page).toHaveURL(/\/app$/)

  await page.getByTestId('nav-frameworks').click()
  await page.getByTestId('framework-adopt-soc2').click()
  await expect(page.getByTestId('framework-card-soc2')).toBeVisible()
  await page.getByTestId('framework-adopt-hipaa').click()
  await expect(page.getByTestId('framework-card-hipaa')).toBeVisible()

  // Onboard: only in-scope frameworks are offered; mapping HIPAA suggests its questionnaire.
  await page.getByTestId('nav-vendors').click()
  await page.getByTestId('vendor-new').click()
  const name = unique('MedCloud')
  await page.getByTestId('vendor-name').fill(name)
  await expect(page.getByTestId('vendor-fw-gdpr')).toHaveCount(0)
  await expect(page.getByTestId('vendor-q-vendor-security-v1')).toBeChecked()
  await expect(page.getByTestId('vendor-q-hipaa-vendor-v1')).not.toBeChecked()
  await page.getByTestId('vendor-fw-hipaa').check()
  await expect(page.getByTestId('vendor-q-hipaa-vendor-v1')).toBeChecked()
  await expect(page.getByTestId('vendor-question-total')).toContainText('30 questions')
  await page.getByTestId('vendor-create').click()

  const scope = page.getByTestId('vendor-scope')
  await expect(scope.getByTestId('vendor-scope-frameworks')).toContainText('HIPAA')
  await expect(scope.getByTestId('vendor-scope-questionnaires')).toContainText('HIPAA Business Associate Questionnaire')

  // Edit scope: add SOC 2 and drop the general questionnaire.
  await page.getByTestId('vendor-scope-edit').click()
  await page.getByTestId('vendor-fw-soc2').check()
  await page.getByTestId('vendor-q-vendor-security-v1').uncheck()
  await page.getByTestId('vendor-scope-save').click()
  await expect(scope.getByTestId('vendor-scope-frameworks')).toContainText('SOC 2')
  await expect(scope.getByTestId('vendor-scope-questionnaires')).not.toContainText('Vendor Security Questionnaire')

  // Vendors list filters by framework.
  await page.getByTestId('nav-vendors').click()
  await page.getByTestId('vendors-framework').selectOption('hipaa')
  await expect(page.getByTestId(`vendor-row-${name}`)).toBeVisible()
  await page.getByTestId(`vendor-row-${name}`).getByRole('link').click()

  // Send: the portal shows only the attached questionnaire.
  await page.getByTestId('vendor-send-questionnaire').click()
  await expect(page.getByTestId('vendor-send-includes')).toContainText('HIPAA Business Associate Questionnaire')
  await page.getByTestId('vendor-questionnaire-email').fill(`${unique('sec')}@medcloud.example`)
  await page.getByTestId('vendor-questionnaire-send').click()
  const link = (await page.getByTestId('vendor-portal-link').textContent())!

  const vendor = await (await browser.newContext()).newPage()
  await vendor.goto(link)
  await expect(vendor.getByTestId('portal-template-hipaa-vendor-v1')).toBeVisible()
  await expect(vendor.getByTestId('portal-template-vendor-security-v1')).toHaveCount(0)
  const yes = vendor.locator('[data-testid^="portal-a-"][data-testid$="-Yes"]')
  for (let i = 0; i < (await yes.count()); i++) await yes.nth(i).click()
  for (const t of await vendor.locator('textarea[data-testid^="portal-a-"]').all()) await t.fill('Jane Doe, security@medcloud.example')
  await expect(vendor.getByTestId('portal-progress')).toHaveText('10/10 answered')
  await vendor.getByTestId('portal-submit').click()
  await expect(vendor.getByTestId('portal-submitted')).toBeVisible()

  await page.reload()
  await expect(page.getByTestId('vendor-questionnaire')).toContainText('submitted')
})
