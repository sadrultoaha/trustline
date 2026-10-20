import { expect, test } from '@playwright/test'
import { ACME, DEMO_PASSWORD, login, logout, unique } from './helpers'

test.describe('Authentication', () => {
  test('signup creates an organization and lands on the dashboard', async ({ page }) => {
    const email = `${unique('founder')}@example.test`
    await page.goto('/signup')
    await page.getByTestId('signup-name').fill('Founder Person')
    await page.getByTestId('signup-email').fill(email)
    await page.getByTestId('signup-password').fill('a-very-long-password-123')
    await page.getByTestId('signup-org').fill('Example Startup')
    await page.getByTestId('signup-submit').click()
    await expect(page).toHaveURL(/\/app$/)
    await expect(page.getByTestId('current-org-name')).toHaveText('Example Startup')
    await expect(page.getByText('signed in as admin')).toBeVisible()

    await logout(page)
    await login(page, email, 'a-very-long-password-123')
    await expect(page.getByTestId('current-org-name')).toHaveText('Example Startup')
  })

  test('signup shows field-level validation errors', async ({ page }) => {
    await page.goto('/signup')
    await page.getByTestId('signup-email').fill('not-an-email')
    await page.getByTestId('signup-password').fill('short')
    await page.getByTestId('signup-submit').click()
    await expect(page.getByTestId('error-name')).toBeVisible()
    await expect(page.getByTestId('error-email')).toBeVisible()
    await expect(page.getByTestId('error-password')).toContainText('12 characters')
    await expect(page.getByTestId('error-org_name')).toBeVisible()
  })

  test('wrong password is rejected and seeded admin can sign in', async ({ page }) => {
    await page.goto('/login')
    await page.getByTestId('login-email').fill(ACME.admin.email)
    await page.getByTestId('login-password').fill('definitely-wrong')
    await page.getByTestId('login-submit').click()
    await expect(page.getByTestId('login-error')).toContainText('incorrect email or password')

    await page.getByTestId('login-password').fill(DEMO_PASSWORD)
    await page.getByTestId('login-submit').click()
    await expect(page.getByTestId('current-org-name')).toHaveText('Acme Corp')
  })

  test('protected routes redirect to login and back', async ({ page }) => {
    await page.goto('/app/settings/members')
    await expect(page).toHaveURL(/\/login\?.*next=/)
    await page.getByTestId('login-email').fill(ACME.admin.email)
    await page.getByTestId('login-password').fill(DEMO_PASSWORD)
    await page.getByTestId('login-submit').click()
    await expect(page).toHaveURL(/\/app\/settings\/members/)
    await expect(page.getByTestId('members-table')).toBeVisible()
  })

  test('password reset via email', async ({ page, request }) => {
    const email = `${unique('reset')}@example.test`
    await page.goto('/signup')
    await page.getByTestId('signup-name').fill('Reset Person')
    await page.getByTestId('signup-email').fill(email)
    await page.getByTestId('signup-password').fill('original-password-123')
    await page.getByTestId('signup-org').fill('Reset Co')
    await page.getByTestId('signup-submit').click()
    await expect(page).toHaveURL(/\/app$/)
    await logout(page)

    await page.goto('/forgot-password')
    await page.getByTestId('forgot-email').fill(email)
    await page.getByTestId('forgot-submit').click()
    await expect(page.getByTestId('forgot-sent')).toBeVisible()

    const mail = await waitFor(request, email)
    const link = mail.text.match(/(https?:\/\/\S+reset-password\?token=\S+)/)?.[1]
    expect(link, 'reset link in email').toBeTruthy()
    await page.goto(link!.replace(/^https?:\/\/[^/]+/, ''))
    await page.getByTestId('reset-password').fill('brand-new-password-456')
    await page.getByTestId('reset-confirm').fill('brand-new-password-456')
    await page.getByTestId('reset-submit').click()
    await expect(page).toHaveURL(/\/login/)
    await login(page, email, 'brand-new-password-456')
  })
})

async function waitFor(request: import('@playwright/test').APIRequestContext, email: string) {
  const { waitForEmail } = await import('./helpers')
  return waitForEmail(request, email, 'Reset')
}
