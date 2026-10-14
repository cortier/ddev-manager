import type { Meta, StoryObj } from '@storybook/html-vite';
import popupHTML from '../extension/popup.html?raw';
import type { State } from '../extension/model';
import { extensionFrame } from './extension-frame';

const surfaces = [
    {
        id: 'inventory-api',
        name: 'inventory-api',
        root: '/projects/inventory-api',
        branch: 'feat/inventory-sync',
        repository: '/projects/inventory',
        status: 'running',
        url: 'https://inventory-api.ddev.site',
    },
    {
        id: 'inventory-app',
        name: 'inventory-app',
        root: '/projects/inventory-app',
        branch: 'feat/inventory-sync',
        repository: '/projects/inventory',
        status: 'running',
        url: 'https://inventory-app.ddev.site',
    },
    {
        id: 'checkout-api',
        name: 'checkout-api',
        root: '/projects/checkout-api',
        branch: 'fix/payment-retry',
        repository: '/projects/checkout',
        status: 'stopped',
        url: 'https://checkout-api.ddev.site',
    },
];

const state: State = {
    surfaces,
    settings: { overrides: {} },
    operations: {},
    connected: true,
    cleanupReady: true,
    loading: false,
    updatedAt: 1,
};

const services = {
    'inventory-api': [
        { id: 'buggregator', name: 'Buggregator', url: 'https://inventory-api.ddev.site:8777' },
        { id: 'storybook', name: 'Storybook', url: 'https://inventory-api.ddev.site:6007' },
        { id: 'webhook-site', name: 'Webhook.site', url: 'https://inventory-api.ddev.site:8084' },
    ],
};

const meta = {
    title: 'Extension/Popup',
    parameters: { viewport: { defaultViewport: 'responsive' } },
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

export const Projects: Story = {
    render: () => extensionFrame(popupHTML, { state, services }, 'popup'),
};

export const ServicesOpen: Story = {
    render: () =>
        extensionFrame(
            popupHTML,
            { state, services },
            'popup',
            `<script>setTimeout(() => document.querySelector('[aria-label="Services for inventory-api"]')?.click(), 50)</script>`,
        ),
};

export const IdeOnly: Story = {
    render: () =>
        extensionFrame(
            popupHTML,
            { state, services: { 'inventory-api': [] } },
            'popup',
            `<script>setTimeout(() => document.querySelector('[aria-label="Services for inventory-api"]')?.click(), 50)</script>`,
        ),
};

export const Busy: Story = {
    render: () =>
        extensionFrame(
            popupHTML,
            {
                state: {
                    ...state,
                    operations: {
                        'inventory-api': { surfaceId: 'inventory-api', label: 'Restarting', state: 'busy' },
                    },
                },
                services,
            },
            'popup',
        ),
};

export const Empty: Story = {
    render: () => extensionFrame(popupHTML, { state: { ...state, surfaces: [] } }, 'popup'),
};

export const CompanionUnavailable: Story = {
    render: () =>
        extensionFrame(
            popupHTML,
            {
                state: {
                    ...state,
                    surfaces: [],
                    connected: false,
                    cleanupReady: false,
                    error: 'DDEV Manager companion is not connected.',
                },
            },
            'popup',
        ),
};
