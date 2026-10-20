import { expect, test } from '@playwright/test'
import { ACME, login } from './helpers'

// Policies ↔ controls can be linked from either side.
test('link a policy from the control page, unlink it from the policy page', async ({ page }) => {
  await login(page, ACME.admin.email)
  await page.getByTestId('nav-controls').click()
  await page.getByTestId('controls-search').fill('DAT-01')
  await page.getByTestId('control-row-DAT-01').getByRole('link').click()
  const card = page.getByTestId('control-policies')
  await expect(card).toBeVisible()

  const option = page.getByTestId('control-link-policy').locator('option', { hasText: 'Data Retention' })
  const title = (await option.textContent())!.trim()
  await page.getByTestId('control-link-policy').selectOption((await option.getAttribute('value'))!)
  const link = card.getByRole('link', { name: title })
  await expect(link).toBeVisible()

  await link.click()
  const controls = page.getByTestId('policy-controls')
  const badge = controls.getByRole('button', { name: 'Remove DAT-01' })
  await expect(badge).toBeVisible()
  await badge.click()
  await expect(badge).toHaveCount(0)

  await page.goBack()
  await expect(page.getByTestId('control-policies').getByRole('link', { name: title })).toHaveCount(0)
})
