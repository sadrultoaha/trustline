import { expect, test } from '@playwright/test'
import { unique } from './helpers'

test('custom framework: create with control mapping → adopt → edit → delete', async ({ page }) => {
  await page.goto('/signup')
  await page.getByTestId('signup-name').fill('GRC Lead')
  await page.getByTestId('signup-email').fill(`${unique('cf')}@example.test`)
  await page.getByTestId('signup-password').fill('a-very-long-password-123')
  await page.getByTestId('signup-org').fill(`Custom FW ${Date.now()}`)
  await page.getByTestId('signup-submit').click()
  await expect(page).toHaveURL(/\/app$/)

  await page.getByTestId('nav-frameworks').click()
  await page.getByTestId('framework-new').click()

  // Server-side validation is shown per field.
  await page.getByTestId('cf-save').click()
  await expect(page.getByTestId('error-cf-name')).toContainText('required')

  await page.getByTestId('cf-name').fill('Acme Customer Schedule')
  await page.getByTestId('cf-code-0').fill('ACME-1')
  await page.getByTestId('cf-title-0').fill('Policies are approved by management')
  await page.getByTestId('cf-controls-0').fill('GOV-01')
  await page.getByTestId('cf-option-GOV-01').click()
  await page.getByTestId('cf-add-requirement').click()
  await page.getByTestId('cf-code-1').fill('ACME-2')
  await page.getByTestId('cf-title-1').fill('Admins use MFA')
  await page.getByTestId('cf-controls-1').fill('IAM-01')
  await page.getByTestId('cf-controls-1').press('Enter')
  await page.getByTestId('cf-save').click()

  // Lands on the new framework, out of scope until adopted.
  await expect(page).toHaveURL(/\/app\/frameworks\/custom-/)
  await expect(page.getByTestId('requirement-ACME-1')).toBeVisible()
  await page.getByTestId('framework-adopt').click()
  await expect(page.getByText('Remove from scope')).toBeVisible()
  await expect(page.getByTestId('framework-detail-percent')).toHaveText('0%')

  // Edit: add a third requirement.
  await page.getByTestId('framework-edit').click()
  await expect(page.getByTestId('cf-name')).toHaveValue('Acme Customer Schedule')
  await page.getByTestId('cf-add-requirement').click()
  await page.getByTestId('cf-code-2').fill('ACME-3')
  await page.getByTestId('cf-title-2').fill('Encrypt data at rest')
  await page.getByTestId('cf-controls-2').fill('DAT-01')
  await page.getByTestId('cf-option-DAT-01').click()
  await page.getByTestId('cf-save').click()
  await expect(page.getByTestId('requirement-ACME-3')).toBeVisible()

  // Listed with a badge among in-scope frameworks.
  await page.getByTestId('nav-frameworks').click()
  const card = page.locator('[data-testid^="framework-card-custom-"]')
  await expect(card).toContainText('Acme Customer Schedule')
  await expect(card).toContainText('Custom')
  await expect(card).toContainText('0/3')

  await card.click()
  await page.getByTestId('framework-delete').click()
  await page.getByTestId('framework-delete-confirm').click()
  await expect(page).toHaveURL(/\/app\/frameworks$/)
  await expect(page.locator('[data-testid^="framework-card-custom-"]')).toHaveCount(0)
})
