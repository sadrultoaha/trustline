import path from 'node:path'
import { expect, test } from '@playwright/test'
import { unique } from './helpers'

test.describe.configure({ timeout: 150_000 })

const testdata = path.resolve(import.meta.dirname, '../../testdata/evidence')

test('new org: connect AWS → add second framework (crosswalk readiness) → evidence with AI review → custom test', async ({ page }) => {
  const email = `${unique('grc')}@example.test`
  await page.goto('/signup')
  await page.getByTestId('signup-name').fill('GRC Lead')
  await page.getByTestId('signup-email').fill(email)
  await page.getByTestId('signup-password').fill('a-very-long-password-123')
  await page.getByTestId('signup-org').fill(`Crosswalk ${Date.now()}`)
  await page.getByTestId('signup-submit').click()
  await expect(page).toHaveURL(/\/app$/)

  // Add SOC 2.
  await page.getByTestId('nav-frameworks').click()
  await page.getByTestId('framework-adopt-soc2').click()
  await expect(page.getByTestId('framework-card-soc2')).toBeVisible()

  // Connect AWS (sandbox) through the UI.
  await page.getByTestId('nav-integrations').click()
  await page.getByTestId('integration-connect-aws').click()
  await page.getByTestId('cred-role_arn').fill('not-an-arn')
  await page.getByTestId('cred-external_id').fill('ext-id-123')
  await page.getByTestId('integration-connect-submit').click()
  await expect(page.getByTestId('error-cred-role_arn')).toContainText('invalid format')
  await page.getByTestId('cred-role_arn').fill('arn:aws:iam::123456789012:role/TrustlineAudit')
  await page.getByTestId('integration-connect-submit').click()
  await expect(page.getByTestId('integration-detail-status')).toHaveText('connected', { timeout: 30_000 })

  // SOC 2 readiness reflects passing AWS controls once tests ran.
  await page.getByTestId('nav-frameworks').click()
  await expect(page.getByTestId('framework-card-soc2')).toContainText(/[1-9]\d* controls passing|controls passing/)
  await expect
    .poll(async () => Number((await page.getByTestId('framework-card-soc2').locator('.text-success').first().textContent()) ?? 0), { timeout: 30_000, intervals: [1000] })
    .toBeGreaterThan(0)

  // Crosswalk: adding ISO 27001 is immediately non-zero.
  await page.getByTestId('framework-adopt-iso27001').click()
  await expect(page.getByTestId('framework-card-iso27001')).toBeVisible()
  const pct = await page.getByTestId('framework-percent-iso27001').textContent()
  expect(parseFloat(pct ?? '0')).toBeGreaterThan(0)
  await page.getByTestId('framework-card-iso27001').click()
  await expect(page.getByTestId('requirement-A.8.5')).toBeVisible()

  // Evidence: upload a relevant screenshot to a manual control → AI accepts.
  await page.getByTestId('nav-controls').click()
  await page.getByTestId('controls-search').fill('HR-02')
  await page.getByTestId('control-row-HR-02').getByRole('link').click()
  await expect(page.getByTestId('control-status').first()).toHaveAttribute('data-status', 'needs_evidence')
  await page.getByTestId('evidence-upload-open').click()
  await page.getByTestId('evidence-file').setInputFiles(path.join(testdata, 'vacation-photo.png'))
  await page.getByTestId('evidence-title').fill('Summer photo')
  await page.getByTestId('evidence-submit').click()
  await expect(page.getByTestId('evidence-ai-verdict')).toHaveText('Does not satisfy', { timeout: 30_000 })
  await expect(page.getByTestId('control-status').first()).toHaveAttribute('data-status', 'needs_evidence')

  // Human override wins over the AI.
  await page.getByTestId('evidence-accept').click()
  await expect(page.getByTestId('evidence-override')).toContainText('accepted')
  await expect(page.getByTestId('control-status').first()).toHaveAttribute('data-status', 'passing', { timeout: 10_000 })

  // A relevant file passes AI review on the MFA control's evidence.
  await page.getByTestId('nav-controls').click()
  await page.getByTestId('controls-search').fill('IAM-04')
  await page.getByTestId('control-row-IAM-04').getByRole('link').click()
  await page.getByTestId('evidence-upload-open').click()
  await page.getByTestId('evidence-file').setInputFiles({ name: 'q3-user-access-review.txt', mimeType: 'text/plain', buffer: Buffer.from('Q3 2026 user access review: 42 accounts recertified by managers, 3 revoked.') })
  await page.getByTestId('evidence-submit').click()
  await expect(page.getByTestId('evidence-ai-verdict')).toHaveText('Satisfies control', { timeout: 30_000 })
  await expect(page.getByTestId('control-status').first()).toHaveAttribute('data-status', 'passing')

  // No-code custom test builder → preview on live data → save → it runs.
  await page.getByTestId('nav-tests').click()
  await page.getByTestId('test-new').click()
  await page.getByTestId('builder-connector').selectOption('aws')
  await page.getByTestId('builder-resource').selectOption('s3_bucket')
  await page.getByTestId('cond-field-assert-0').selectOption('encryption.enabled')
  await page.getByTestId('cond-op-assert-0').selectOption('eq')
  await page.getByTestId('cond-value-assert-0').selectOption('true')
  await page.getByTestId('add-group-assert').click()
  await page.getByTestId('group-kind-assert-1').selectOption('or')
  await page.getByTestId('cond-field-assert-1-0').selectOption('versioning')
  await page.getByTestId('cond-value-assert-1-0').selectOption('true')
  await expect(page.getByTestId('builder-description')).toContainText('encryption.enabled is true')
  await page.getByTestId('builder-preview').click()
  await expect(page.getByTestId('builder-preview-result')).toContainText('Would pass')
  await page.getByTestId('builder-name').fill('Buckets are encrypted and versioned')
  await page.getByTestId('builder-control').selectOption({ label: 'CRY-01 · Encryption at rest' })
  await page.getByTestId('builder-save').click()
  await expect(page).toHaveURL(/\/app\/tests\/[0-9a-f-]+$/)
  await expect(page.getByTestId('test-status').first()).toHaveAttribute('data-status', 'pass', { timeout: 30_000 })
})
