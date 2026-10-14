import type { StorybookConfig } from '@storybook/html-vite';

const config: StorybookConfig = {
    stories: ['../stories/**/*.stories.ts'],
    addons: ['@storybook/addon-a11y'],
    framework: {
        name: '@storybook/html-vite',
        options: {},
    },
    staticDirs: [{ from: '../dist/extension', to: '/' }],
};

export default config;
