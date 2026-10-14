import type { StorybookConfig } from '@storybook/html-vite';

const config: StorybookConfig = {
    stories: ['../stories/**/*.stories.ts'],
    addons: ['@storybook/addon-a11y'],
    core: {
        disableWhatsNewNotifications: true,
    },
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
            hmr: process.env.DDEV_HOSTNAME
                ? {
                      protocol: 'wss',
                      clientPort: 6007,
                  }
                : viteConfig.server?.hmr,
        },
    }),
};

export default config;
