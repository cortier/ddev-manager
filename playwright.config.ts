import { defineConfig } from '@playwright/test';
export default defineConfig({
    testDir: 'tests/ui',
    use: { baseURL: 'http://127.0.0.1:43118', viewport: { width: 400, height: 680 } },
    webServer: {
        command: 'node scripts/preview.mjs',
        url: 'http://127.0.0.1:43118/popup.html',
        reuseExistingServer: false,
    },
    projects: [{ name: 'firefox', use: { browserName: 'firefox' } }],
});
