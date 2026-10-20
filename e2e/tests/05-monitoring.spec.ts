import { expect, test, type Page } from '@playwright/test'
import { ACME, login } from './helpers'

// Module 1 acceptance (UI): flip the sandbox AWS account to a public bucket →
// within one scheduler tick (20s in the compose stack) the control turns red,
// a notification fires and a remediation task with Terraform exists.
test.describe.configure({ timeout: 180_000 })

async function openIntegration(page: Page, type: string) {
  await page.getByTestId('nav-integrations').click()
  await page.getByTestId(`integration-open-${type}`).click()
  await expect(page.getByTestId('sandbox-panel')).toBeVisible()
}

async function openControl(page: Page, code: string) {
  await page.getByTestId('nav-controls').click()
  await page.getByTestId('controls-search').fill(code)
  await page.getByTestId(`control-row-${code}`).getByRole('link').click()
  await expect(page.getByTestId('control-name')).toBeVisible()
}

test('public bucket → control fails within a tick → notification + AI remediation with Terraform → recovery', async ({ page }) => {
  await login(page, ACME.admin.email)
  await openControl(page, 'DAT-01')
  await expect(page.getByTestId('control-status').first()).toHaveAttribute('data-status', 'passing')

  await openIntegration(page, 'aws')
  const toggle = page.getByTestId('scenario-public_bucket')
  if ((await toggle.getAttribute('aria-checked')) === 'true') {
    await toggle.click() // leftover from an aborted run: reset first
    await expect(toggle).toHaveAttribute('aria-checked', 'false')
  }
  await toggle.click()
  await expect(toggle).toHaveAttribute('aria-checked', 'true')

  // No manual sync: the scheduler picks it up. The control page polls every 5s.
  await openControl(page, 'DAT-01')
  await expect(page.getByTestId('control-status').first()).toHaveAttribute('data-status', 'failing', { timeout: 90_000 })
  await expect(page.getByTestId('control-status-reason')).toContainText('failing')

  // Notification.
  await expect(page.getByTestId('notifications-unread')).toBeVisible({ timeout: 20_000 })
  await page.getByTestId('notifications-bell').click()
  await expect(page.getByTestId('notification-item').filter({ hasText: 'DAT-01' }).first()).toBeVisible()
  await page.keyboard.press('Escape')

  // Remediation task with steps and Terraform from the AI layer.
  await page.getByTestId('control-open-task').first().click()
  await expect(page.getByTestId('task-terraform')).toContainText('-marketing-assets', { timeout: 30_000 })
  await expect(page.getByTestId('task-terraform')).toContainText('aws_s3_bucket_public_access_block')
  await expect(page.getByTestId('task-steps').locator('li')).toHaveCount(await page.getByTestId('task-steps').locator('li').count())
  expect(await page.getByTestId('task-steps').locator('li').count()).toBeGreaterThanOrEqual(3)
  const taskUrl = page.url()

  // Remediate in the sandbox and sync now → task closes itself, control recovers.
  await openIntegration(page, 'aws')
  await page.getByTestId('scenario-public_bucket').click()
  await expect(page.getByTestId('scenario-public_bucket')).toHaveAttribute('aria-checked', 'false')
  await page.getByTestId('integration-sync').click()
  await openControl(page, 'DAT-01')
  await expect(page.getByTestId('control-status').first()).toHaveAttribute('data-status', 'passing', { timeout: 60_000 })
  await page.goto(taskUrl)
  await expect(page.getByTestId('task-status')).toHaveText('done', { timeout: 20_000 })
  await expect(page.getByTestId('task-resolution')).toContainText('Resolved automatically')
})

test('auditor sees controls, tests and integrations but cannot change the sandbox', async ({ page }) => {
  await login(page, ACME.auditor.email)
  await page.getByTestId('nav-controls').click()
  await expect(page.getByTestId('controls-table')).toBeVisible()
  await page.getByTestId('nav-tests').click()
  await expect(page.getByTestId('tests-table')).toBeVisible()
  await expect(page.getByTestId('test-new')).toHaveCount(0)
  await page.getByTestId('nav-integrations').click()
  await page.getByTestId('integration-open-aws').click()
  await expect(page.getByTestId('scenario-public_bucket')).toBeDisabled()
  await expect(page.getByTestId('integration-sync')).toHaveCount(0)
})
