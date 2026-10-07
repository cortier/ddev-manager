import { beforeEach, afterEach, it, expect, vi } from 'vitest';
let listener: (m: any) => any;
let nativeListener: (m: any) => void;
let disconnect: () => void;
let stored: Record<string, any>;
let calls: any[];
let opened: string[];
let published: any[];
let failSurface: string | undefined;
let poll: () => void;
const surfaces = [
    {
        id: 'api',
        name: 'api',
        branch: 'feat/work',
        repository: 'product-api',
        root: '/api',
        status: 'running',
        url: 'https://api.test',
    },
    {
        id: 'app',
        name: 'app',
        branch: 'feat/work',
        repository: 'product-app',
        root: '/app',
        status: 'running',
        url: 'https://app.test',
    },
    {
        id: 'api-two',
        name: 'api-two',
        branch: 'feat/other',
        repository: 'other-api',
        root: '/api-two',
        status: 'running',
        url: 'https://api-two.test',
    },
];
beforeEach(async () => {
    vi.resetModules();
    stored = {};
    calls = [];
    opened = [];
    published = [];
    failSurface = undefined;
    vi.stubGlobal('setInterval', (callback: () => void) => {
        poll = callback;
        return 1;
    });
    const runtime = {
        connectNative: () => ({
            postMessage: (m: any) => {
                calls.push(m);
                queueMicrotask(() => {
                    let result: any = {};
                    if (m.method === 'discover') result = structuredClone(surfaces);
                    if (m.method === 'register') result = { url: 'http://127.0.0.1:43111/uninstall#token' };
                    if (m.method === 'services')
                        result = [
                            {
                                id: 'buggregator',
                                name: 'Buggregator',
                                url: `https://${m.surfaceId}.test:8777`,
                            },
                        ];
                    if (m.method === 'action') result = { url: 'https://api.test' };
                    nativeListener({
                        version: 1,
                        id: m.id,
                        ...(m.method === 'action' && m.surfaceId === failSurface
                            ? { error: { message: 'Fixture failure' } }
                            : { result }),
                    });
                });
            },
            onMessage: { addListener: (fn: any) => (nativeListener = fn) },
            onDisconnect: { addListener: (fn: any) => (disconnect = fn) },
        }),
        onMessage: { addListener: (fn: any) => (listener = fn) },
        sendMessage: async (m: any) => {
            published.push(structuredClone(m));
        },
        setUninstallURL: vi.fn(async () => {}),
    };
    vi.stubGlobal('browser', {
        runtime,
        storage: { local: { get: async () => stored, set: async (v: any) => Object.assign(stored, v) } },
        tabs: {
            create: async ({ url }: any) => {
                opened.push(url);
            },
        },
    });
    await import('../extension/background');
    await vi.waitFor(() => expect(published.at(-1)?.state.loading).toBe(false));
});
afterEach(() => vi.unstubAllGlobals());
it('registers a stable profile and opens through Firefox', async () => {
    expect(stored.profile.id).toHaveLength(64);
    expect(stored.profile.proof).toHaveLength(64);
    const identity = stored.profile.id;
    await listener({ type: 'action', ids: ['api'], action: 'open' });
    await vi.waitFor(() => expect(opened).toEqual(['https://api.test/']));
    await listener({ type: 'refresh' });
    expect(stored.profile.id).toBe(identity);
});
it('sends IDE actions through the native companion', async () => {
    await listener({ type: 'action', ids: ['api'], action: 'ide' });
    await vi.waitFor(() => {
        expect(calls.some((call) => call.method === 'action' && call.action === 'ide')).toBe(true);
        expect(published.at(-1)?.state.operations.api?.state).toBe('success');
        expect(published.at(-1)?.state.loading).toBe(false);
    });
    expect(opened).toEqual([]);
});
it('loads and caches services separately for every worktree', async () => {
    await vi.waitFor(() => expect(calls.filter((call) => call.method === 'services')).toHaveLength(3));
    expect(calls.filter((call) => call.method === 'services').map((call) => call.surfaceId)).toEqual([
        'api',
        'app',
        'api-two',
    ]);
    const before = calls.length;
    const result = await listener({ type: 'services', id: 'api-two' });
    expect(calls).toHaveLength(before);
    expect(result).toEqual([{ id: 'buggregator', name: 'Buggregator' }]);
});
it('resolves a service through the companion every time it is opened', async () => {
    await listener({
        type: 'action',
        ids: ['api'],
        action: 'open',
        serviceId: 'buggregator',
    });
    await vi.waitFor(() => expect(opened).toHaveLength(1));
    await listener({
        type: 'action',
        ids: ['api'],
        action: 'open',
        serviceId: 'buggregator',
    });
    await vi.waitFor(() => expect(opened).toHaveLength(2));
    expect(calls.filter((call) => call.method === 'action' && call.serviceId === 'buggregator')).toHaveLength(2);
});
it('starts a stopped surface before opening its service', async () => {
    surfaces[0].status = 'stopped';
    await listener({ type: 'refresh' });
    await listener({
        type: 'action',
        ids: ['api'],
        action: 'open',
        serviceId: 'buggregator',
    });
    await vi.waitFor(() =>
        expect(calls.some((call) => call.method === 'action' && call.serviceId === 'buggregator')).toBe(true),
    );
    surfaces[0].status = 'running';
    await listener({ type: 'refresh' });
});
it('continues batches after failure and preserves results with no popup', async () => {
    failSurface = 'api';
    await listener({ type: 'action', ids: ['api', 'app'], action: 'stop' });
    await vi.waitFor(() => expect(published.at(-1)?.state.operations.app?.state).toBe('success'));
    expect(published.at(-1).state.operations.api.state).toBe('error');
    expect(calls.filter((c) => c.method === 'action').map((c) => c.surfaceId)).toEqual(['api', 'app']);
});
it('publishes the completed lifecycle status before discovery refreshes', async () => {
    await listener({ type: 'action', ids: ['api'], action: 'stop' });
    await vi.waitFor(() =>
        expect(
            published.some(
                (message) =>
                    message.state.operations.api?.state === 'success' &&
                    message.state.surfaces.find((surface: any) => surface.id === 'api')?.status === 'stopped',
            ),
        ).toBe(true),
    );
});
it('rejects duplicate in-flight actions', async () => {
    const a = listener({ type: 'action', ids: ['api'], action: 'restart' });
    const b = listener({ type: 'action', ids: ['api'], action: 'restart' });
    await a;
    await expect(b).rejects.toThrow('already running');
    await vi.waitFor(() => expect(published.at(-1)?.state.operations.api?.state).toBe('success'));
});
it('marks native disconnection and reconnects on refresh', async () => {
    disconnect();
    expect(published.at(-1).state.connected).toBe(false);
    await listener({ type: 'refresh' });
    expect(published.at(-1).state.connected).toBe(true);
});
it('silently polls and publishes project changes', async () => {
    const before = published.length;
    surfaces[0].status = 'stopped';
    poll();
    await vi.waitFor(() =>
        expect(published.slice(before).some((message) => message.state.surfaces[0].status === 'stopped')).toBe(true),
    );
    expect(published.slice(before).some((message) => message.state.loading)).toBe(false);
    surfaces[0].status = 'running';
});
