import type { Meta, StoryObj } from '@storybook/html-vite';
import optionsHTML from '../extension/options.html?raw';
import { extensionFrame } from './extension-frame';

const meta = { title: 'Extension/Settings' } satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

const diagnostics = {
    platform: 'linux',
    cleanupReady: true,
    cleanupError: '',
    environmentIDE: 'code',
    config: {
        ddevPath: '/usr/local/bin/ddev',
        gitPath: '/usr/bin/git',
        idePath: '',
    },
};

export const Connected: Story = {
    render: () =>
        extensionFrame(
            optionsHTML,
            {
                diagnostics,
                settings: {
                    overrides: {
                        'inventory-api': { product: 'Inventory', surface: 'API' },
                    },
                },
            },
            'options',
        ),
};

export const CompanionUnavailable: Story = {
    render: () =>
        extensionFrame(
            optionsHTML,
            {
                diagnosticsError: 'Native host has exited.',
                settings: { overrides: {} },
            },
            'options',
        ),
};
