import { expect, test } from '@playwright/test'
import { unique } from './helpers'

test('sign in with (fake) Google, create an org on onboarding, land on dashboard', async ({ page }) => {
  const email = `${unique('oauth')}@example.test`
  await page.goto('/login')
  await page.getByTestId('login-google').click()
  // Fake provider consent page
  await expect(page).toHaveURL(/\/auth\/fake-google\/authorize/)
  await page.getByTestId('fake-google-name').fill('OAuth Person')
  await page.getByTestId('fake-google-email').fill(email)
  await page.getByTestId('fake-google-continue').click()
  // New user has no org → onboarding
  await expect(page).toHaveURL(/\/onboarding/)
  await page.getByTestId('onboarding-org-name').fill('OAuth Org')
  await page.getByTestId('onboarding-submit').click()
  await expect(page).toHaveURL(/\/app$/)
  await expect(page.getByTestId('current-org-name')).toHaveText('OAuth Org')

  // Second sign-in with the same fake account goes straight to the app.
  await page.getByTestId('user-menu').click()
  await page.getByTestId('logout').click()
  await page.getByTestId('login-google').click()
  await page.locator(`[data-testid="fake-google-email"]`).fill(email)
  await page.getByTestId('fake-google-continue').click()
  await expect(page).toHaveURL(/\/app$/)
})
