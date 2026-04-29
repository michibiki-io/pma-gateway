import { mkdir } from 'node:fs/promises';
import path from 'node:path';
import { expect, test, type Page, type Route, type TestInfo } from '@playwright/test';

test.beforeEach(async ({ page }) => {
  if (!isRealApp()) {
    await mockGatewayApi(page);
  }
});

test('top page renders without layout failure', async ({ page }, testInfo) => {
  await page.goto(frontendPath('/'));

  await expect(page).toHaveTitle(/pma-gateway/i);
  await expectUsablePage(page);
  await expect(page.getByText('Available credentials')).toBeVisible();
  await expect(credentialHeading(page)).toBeVisible();

  await saveScreenScreenshot(page, testInfo, 'top-page');
});

test('account navigation works', async ({ page }, testInfo) => {
  await page.goto(frontendPath('/'));

  await page.getByRole('button', { name: 'Open account view' }).click();

  await expect(page).toHaveURL(/\/_gateway\/account$/);
  await expectUsablePage(page);
  await expect(page.getByRole('heading', { name: 'Account' })).toBeVisible();
  await expect(page.getByRole('heading', { name: 'User' })).toBeVisible();
  await expect(page.getByText('2 groups')).toBeVisible();

  await saveScreenScreenshot(page, testInfo, 'account');
});

test('safe navigation views render without layout failure', async ({ page }, testInfo) => {
  await page.goto(frontendPath('/'));

  const routes = [
    {
      name: 'Credentials',
      screenshot: 'nav-credentials',
      assert: async () => {
        await expect(page.getByText('Available credentials')).toBeVisible();
        await expect(credentialHeading(page)).toBeVisible();
      },
    },
    {
      name: 'Admin credentials',
      screenshot: 'nav-admin-credentials',
      assert: async () => {
        await expect(page.getByRole('heading', { name: 'Create credential' })).toBeVisible();
      },
    },
    {
      name: 'Mappings',
      screenshot: 'nav-mappings',
      assert: async () => {
        await expect(page.getByRole('heading', { name: 'Create mapping' })).toBeVisible();
      },
    },
    {
      name: 'Audit log',
      screenshot: 'nav-audit-log',
      assert: async () => {
        await expect(page.getByRole('heading', { name: 'Audit log' })).toBeVisible();
      },
    },
  ];

  for (const route of routes) {
    await openMobileMenuIfNeeded(page);
    await page.getByRole('button', { name: route.name, exact: true }).click();
    await expectUsablePage(page);
    await route.assert();
    await saveScreenScreenshot(page, testInfo, route.screenshot);
  }
});

test('credential opens phpMyAdmin through the signon flow', async ({ page }, testInfo) => {
  test.skip(!isRealApp(), 'phpMyAdmin signon flow requires the full Docker Compose stack.');

  await page.goto(frontendPath('/'));

  await page
    .getByRole('button', { name: /Open phpMyAdmin using Development Readonly/ })
    .click();

  const pmaBase = pmaPath('/');
  await page.waitForURL((url) => url.pathname.startsWith(pmaBase), { timeout: 30_000 });
  await expectUsablePage(page, { checkHorizontalOverflow: false });
  await expect(page.locator('body')).toContainText(/phpMyAdmin/i, { timeout: 15_000 });

  await saveScreenScreenshot(page, testInfo, 'phpmyadmin');
});

async function mockGatewayApi(page: Page) {
  await page.route('**/_api/v1/me', async (route) => {
    await fulfillJson(route, {
      user: 'alice@example.com',
      groups: ['db-users', 'db-admins'],
      isAdmin: true,
    });
  });

  await page.route('**/_api/v1/available-credentials', async (route) => {
    await fulfillJson(route, {
      items: [mockCredential()],
    });
  });

  await page.route('**/_api/v1/admin/credentials', async (route) => {
    await fulfillJson(route, {
      items: [mockCredential()],
    });
  });

  await page.route('**/_api/v1/admin/mappings', async (route) => {
    await fulfillJson(route, {
      items: [
        {
          id: 'mapping-1',
          subjectType: 'group',
          subject: 'db-admins',
          credentialId: 'sampledb-readonly',
        },
      ],
    });
  });

  await page.route('**/_api/v1/admin/audit-events/filter-options', async (route) => {
    await fulfillJson(route, {
      actions: ['credential.available.list', 'audit.view'],
      targetTypes: ['credential', 'audit_event'],
      actorSuggestions: ['alice@example.com'],
    });
  });

  await page.route('**/_api/v1/admin/audit-events?*', async (route) => {
    await fulfillJson(route, {
      items: [
        {
          id: 'audit-1',
          timestamp: '2026-04-30 10:00:00 JST',
          actor: 'alice@example.com',
          action: 'audit.view',
          targetType: 'audit_event',
          targetId: '',
          result: 'success',
          remoteAddress: '127.0.0.1',
          message: 'Audit log view',
          metadata: {},
        },
      ],
      page: 1,
      pageSize: 10,
      totalItems: 1,
      totalPages: 1,
    });
  });
}

function mockCredential() {
  return {
    id: 'sampledb-readonly',
    name: 'Sample database',
    description: 'Read-only access for smoke tests',
    dbHost: 'mariadb',
    dbPort: 3306,
    dbUser: 'readonly',
    enabled: true,
  };
}

async function fulfillJson(route: Route, body: unknown) {
  await route.fulfill({
    contentType: 'application/json',
    body: JSON.stringify(body),
  });
}

function isRealApp() {
  return process.env.E2E_REAL_APP === 'true';
}

function frontendPath(relativePath: string) {
  return joinBasePath(process.env.E2E_FRONTEND_PATH ?? '/', relativePath);
}

function pmaPath(relativePath: string) {
  const frontendBase = normalizeBasePath(process.env.E2E_FRONTEND_PATH ?? '/');
  const pmaBase = frontendBase.endsWith('/_gateway/')
    ? `${frontendBase.slice(0, -'_gateway/'.length)}_pma/`
    : '/_pma/';
  return joinBasePath(pmaBase, relativePath);
}

function joinBasePath(basePath: string, relativePath: string) {
  const normalizedBase = normalizeBasePath(basePath);
  const normalizedRelative = relativePath.replace(/^\/+/, '');
  return `${normalizedBase}${normalizedRelative}`;
}

function normalizeBasePath(basePath: string) {
  return basePath.endsWith('/') ? basePath : `${basePath}/`;
}

function credentialHeading(page: Page) {
  return page.getByRole('heading', {
    name: isRealApp()
      ? /Development (Root|Readonly|Admin)/
      : 'Sample database',
  }).first();
}

async function openMobileMenuIfNeeded(page: Page) {
  const openMenu = page.getByRole('button', { name: 'Open menu' });
  if (await openMenu.isVisible().catch(() => false)) {
    await openMenu.click();
  }
}

async function expectUsablePage(
  page: Page,
  options: { checkHorizontalOverflow?: boolean } = {},
) {
  await expect(page.locator('body')).toBeVisible();
  await expect(page.locator('body')).not.toBeEmpty();

  if (options.checkHorizontalOverflow === false) {
    return;
  }

  const overflow = await page.evaluate(() => {
    const root = document.documentElement;
    return root.scrollWidth - root.clientWidth;
  });
  expect(overflow).toBeLessThanOrEqual(4);
}

async function saveScreenScreenshot(page: Page, testInfo: TestInfo, screenName: string) {
  const screenshotDir = process.env.E2E_SCREENSHOT_DIR ?? 'test-results/screenshots';
  await mkdir(screenshotDir, { recursive: true });

  const projectName = sanitizeFileName(testInfo.project.name);
  await page.screenshot({
    path: path.join(screenshotDir, `${projectName}-${screenName}.png`),
    fullPage: true,
  });
}

function sanitizeFileName(value: string) {
  return value.replace(/[^a-z0-9_-]+/gi, '-').replace(/^-|-$/g, '');
}
