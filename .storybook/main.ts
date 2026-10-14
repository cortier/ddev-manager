import type { StorybookConfig } from '@storybook/html-vite';

const config: StorybookConfig = {
    stories: ['../stories/**/*.stories.ts'],
    addons: ['@storybook/addon-a11y'],
    framework: {
        name: '@storybook/html-vite',
        options: {},
    },
    staticDirs: [{ from: '../dist/extension', to: '/' }],
    viteFinal: (viteConfig) => ({
        ...viteConfig,
        cacheDir: '.ddev/.storybook-vite-cache',
        server: {
            ...viteConfig.server,
            allowedHosts: ['ddev-manager.ddev.site', 'localhost', '127.0.0.1'],
        },
    }),
};

export default config;
