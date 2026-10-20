import { expect, test } from '@playwright/test'
import { ACME, login } from './helpers'

test.describe.configure({ timeout: 180_000 })

test('access review: revoke one account, approve the rest, complete, export evidence', async ({ page }) => {
  await login(page, ACME.admin.email)
  await page.getByTestId('nav-access-reviews').click()
  await page.getByTestId('campaign-new').click()
  await page.getByTestId('campaign-system-okta').check()
  await page.getByTestId('campaign-system-github').check()
  await page.getByTestId('campaign-create').click()
  await expect(page.getByTestId('campaign-status')).toHaveText('open')

  // Revoke Tom's Okta account (he is inactive), keep everyone else.
  const tom = page.getByTestId('review-item-okta-tom@acme.test')
  await tom.getByTestId('decide-revoke').click()
  await expect(tom.getByRole('link', { name: 'revocation task' })).toBeVisible()
  // One explicit keep, then bulk-keep the rest.
  await page.getByTestId('review-item-okta-grace@acme.test').getByTestId('decide-approve').click()
  await page.getByTestId('campaign-keep-rest').click()
  await expect(page.getByTestId('campaign-complete')).toBeEnabled({ timeout: 30_000 })
  await page.getByTestId('campaign-complete').click()
  await expect(page.getByTestId('campaign-status')).toHaveText('completed')

  // CSV export contains the decisions.
  const href = await page.getByTestId('campaign-export').getAttribute('href')
  const csv = await (await page.request.get(href!)).text()
  expect(csv).toContain('decision')
  expect(csv).toContain('revoke')
  expect(csv.trim().split('\n').length).toBeGreaterThan(10)

  // Evidence was filed on the access review control.
  await page.getByTestId('nav-controls').click()
  await page.getByTestId('controls-search').fill('IAM-04')
  await page.getByTestId('control-row-IAM-04').getByRole('link').click()
  await expect(page.getByTestId('evidence-item').first()).toContainText('Access review')
})
