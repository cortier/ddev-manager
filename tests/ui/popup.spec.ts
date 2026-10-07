import { test, expect } from '@playwright/test';
const surfaces = [
    {
        id: 'inventory-sync-api',
        name: 'inventory-sync-api',
        branch: 'feat/inventory-sync',
        repository: 'example-api',
        root: '/projects/api',
        status: 'running',
        url: 'https://api.test',
    },
    {
        id: 'inventory-sync-app',
        name: 'inventory-sync-app',
        branch: 'feat/inventory-sync',
        repository: 'example-app',
        root: '/projects/app',
        status: 'running',
        url: 'https://app.test',
    },
    {
        id: 'example-api',
        name: 'example-api',
        branch: 'fix/a-very-long-task-name-that-needs-to-wrap-without-hiding-actions',
        repository: 'example-api',
        root: '/projects/example',
        status: 'stopped',
        url: 'https://example.test',
    },
];
test.beforeEach(async ({ page }) => {
    await page.addInitScript(
        ({ surfaces }) => {
            const state = {
                surfaces,
                settings: { overrides: {} },
                operations: {},
                loading: false,
                connected: true,
                cleanupReady: true,
            };
            (window as any).requests = [];
            (window as any).storageWrites = [];
            (window as any).browser = {
                runtime: {
                    onMessage: { addListener: () => {} },
                    sendMessage: async (m: any) => {
                        (window as any).requests.push(m);
                        if (m.type === 'getState') return state;
                        if (m.type === 'diagnostics')
                            return {
                                platform: 'linux',
                                cleanupReady: true,
                                config: {
                                    ddevPath: '/usr/bin/ddev',
                                    gitPath: '/usr/bin/git',
                                    idePath: '/usr/bin/code',
                                },
                            };
                        if (m.type === 'services')
                            return [
                                { id: 'buggregator', name: 'Buggregator', url: 'https://api.test:8777' },
                                { id: 'webhook-site', name: 'webhook.site', url: 'https://api.test:8084' },
                            ];
                        return { ok: true };
                    },
                    openOptionsPage: async () => {
                        (window as any).optionsOpened = true;
                    },
                },
                storage: {
                    local: {
                        get: async () => ({ settings: { overrides: {} }, runningOnly: false }),
                        set: async (value: any) => {
                            (window as any).storageWrites.push(value);
                        },
                    },
                },
            };
        },
        { surfaces },
    );
});
for (const colorScheme of ['dark', 'light'] as const) {
    test(`${colorScheme} popup layout and services`, async ({ page }) => {
        await page.emulateMedia({ colorScheme });
        await page.goto('/popup.html');
        await expect(page.getByRole('button', { name: 'Refresh projects' })).toHaveCount(0);
        await expect(page.getByRole('heading', { name: 'inventory-sync', exact: true })).toBeVisible();
        await expect(page.locator('body')).toHaveCSS('width', '360px');
        await expect(page.locator('.surface-status')).toHaveCount(0);
        await expect(page.getByRole('button', { name: 'Open API, running' })).toHaveAttribute(
            'title',
            'Open inventory-sync-api\n/projects/api',
        );
        const overflow = await page.evaluate(() => document.body.scrollWidth > 360);
        expect(overflow).toBe(false);
        await page.getByRole('button', { name: 'Services for inventory-sync-api' }).click();
        await expect(page.getByRole('button', { name: 'Buggregator', exact: true })).toBeVisible();
        await expect(page.getByRole('separator')).toBeVisible();
        const ideBottom = await page
            .getByRole('button', { name: 'Open in IDE', exact: true })
            .evaluate((element) => element.getBoundingClientRect().bottom);
        const dividerTop = await page.getByRole('separator').evaluate((element) => element.getBoundingClientRect().top);
        const serviceTop = await page
            .getByRole('button', { name: 'Buggregator', exact: true })
            .evaluate((element) => element.getBoundingClientRect().top);
        expect(ideBottom).toBeLessThan(dividerTop);
        expect(dividerTop).toBeLessThan(serviceTop);
        const taskButtons = await Promise.all(
            [
                'Start all surfaces in inventory-sync',
                'Restart all surfaces in inventory-sync',
                'Stop all surfaces in inventory-sync',
            ].map((name) => page.getByRole('button', { name, exact: true }).boundingBox()),
        );
        const surfaceButtons = await Promise.all(
            ['Restart inventory-sync-api', 'Stop inventory-sync-api', 'Services for inventory-sync-api'].map((name) =>
                page.getByRole('button', { name, exact: true }).boundingBox(),
            ),
        );
        expect(surfaceButtons.map((box) => box?.x)).toEqual(taskButtons.map((box) => box?.x));
        const searchBox = await page.getByRole('searchbox').boundingBox();
        const surface = await page.locator('.surface-row').first().boundingBox();
        const services = await page.locator('.service-menu').boundingBox();
        expect({ x: surface?.x, width: surface?.width }).toEqual({ x: searchBox?.x, width: searchBox?.width });
        expect({ x: services?.x, width: services?.width }).toEqual({ x: searchBox?.x, width: searchBox?.width });
        await page.getByRole('button', { name: 'Open in IDE', exact: true }).click();
        expect(await page.evaluate(() => (window as any).requests.at(-1))).toMatchObject({
            type: 'action',
            action: 'ide',
            ids: ['inventory-sync-api'],
        });
        await expect(page.locator('.service-menu')).toHaveCount(0);
        await page.getByRole('button', { name: 'Services for inventory-sync-api' }).click();
        await page.getByRole('button', { name: 'Buggregator', exact: true }).click();
        expect(await page.evaluate(() => (window as any).requests.at(-1))).toMatchObject({
            type: 'action',
            action: 'open',
            ids: ['inventory-sync-api'],
            serviceId: 'buggregator',
            serviceUrl: 'https://api.test:8777',
        });
        await expect(page.locator('.service-menu')).toHaveCount(0);
        await page.screenshot({ path: `dist/popup-${colorScheme}.png` });
    });
}
test('search keeps siblings and no-match state is readable', async ({ page }) => {
    await page.goto('/popup.html');
    await page.getByRole('searchbox').fill('inventory-sync-api');
    await expect(page.getByRole('button', { name: 'Open App, running' })).toBeVisible();
    await expect(page.getByRole('heading', { name: /a-very-long/ })).toHaveCount(0);
    await page.getByRole('searchbox').fill('unknown');
    await expect(page.getByText('No tasks match your filters.')).toBeVisible();
});
test('running filter hides stopped surfaces and saves its selection', async ({ page }) => {
    await page.goto('/popup.html');
    const filter = page.getByRole('checkbox', { name: 'Only show running projects' });
    await filter.check();
    await expect(page.getByRole('heading', { name: /a-very-long/ })).toHaveCount(0);
    await expect(page.getByRole('button', { name: 'Open API, running' })).toBeVisible();
    await expect.poll(() => page.evaluate(() => (window as any).storageWrites.at(-1))).toEqual({ runningOnly: true });
});
test('task commands carry all surfaces and keyboard focus is visible', async ({ page }) => {
    await page.goto('/popup.html');
    const button = page.getByRole('button', { name: 'Stop all surfaces in inventory-sync' });
    await button.focus();
    await expect(button).toBeFocused();
    await page.keyboard.press('Enter');
    expect(await page.evaluate(() => (window as any).requests.at(-1))).toMatchObject({
        type: 'action',
        action: 'stop',
        ids: ['inventory-sync-api', 'inventory-sync-app'],
    });
});
test('settings cog opens the Firefox options page', async ({ page }) => {
    await page.goto('/popup.html');
    await expect(page.locator('footer').getByRole('button')).toHaveCount(0);
    await page.getByRole('button', { name: 'Settings' }).click();
    await expect.poll(() => page.evaluate(() => (window as any).optionsOpened)).toBe(true);
});
test('settings shows the effective IDE and saves an explicit override', async ({ page }) => {
    await page.goto('/options.html');
    await expect(page.getByLabel('IDE executable')).toHaveValue('/usr/bin/code');
    await page.getByLabel('IDE executable').fill('/opt/idea/bin/idea');
    await page.getByRole('button', { name: 'Save settings' }).click();
    await expect
        .poll(() => page.evaluate(() => (window as any).requests.findLast((m: any) => m.type === 'saveSettings')))
        .toMatchObject({ settings: { idePath: '/opt/idea/bin/idea' } });
});
test('settings prefills IDE from the environment when no path is configured', async ({ page }) => {
    await page.addInitScript(() => {
        const original = (window as any).browser.runtime.sendMessage;
        (window as any).browser.runtime.sendMessage = async (m: any) =>
            m.type === 'diagnostics'
                ? {
                      platform: 'linux',
                      cleanupReady: true,
                      config: { ddevPath: '/usr/bin/ddev', gitPath: '/usr/bin/git' },
                      environmentIDE: '/usr/local/bin/code',
                  }
                : original(m);
    });
    await page.goto('/options.html');
    await expect(page.getByLabel('IDE executable')).toHaveValue('/usr/local/bin/code');
});
