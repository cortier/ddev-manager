import { safeURL, surfaceType, type State, type Surface, type Service, type Settings } from './model';
const HOST = 'com.cortier.ddev_manager';
let state: State = {
    surfaces: [],
    settings: { overrides: {} },
    operations: {},
    loading: false,
    connected: false,
    cleanupReady: false,
};
let port: browser.runtime.Port | undefined;
const pending = new Map<
    string,
    { resolve: (v: any) => void; reject: (e: Error) => void; timer: ReturnType<typeof setTimeout> }
>();
interface ServiceTemplate {
    source: Surface;
    services: Service[];
}
const services = new Map<string, ServiceTemplate>();
const serviceLoads = new Map<string, Promise<ServiceTemplate>>();
let refreshing: Promise<void> | undefined;
let registration: Promise<void> | undefined;
let registered = false;
let queue = Promise.resolve();
const ready = browser.storage.local.get(['settings']).then((saved) => {
    if (saved.settings) state.settings = saved.settings;
});
function publish() {
    void browser.runtime.sendMessage({ type: 'state', state }).catch(() => {});
}
function connect() {
    if (port) return port;
    port = browser.runtime.connectNative(HOST);
    port.onMessage.addListener((message: any) => {
        const p = pending.get(message.id);
        if (!p) return;
        clearTimeout(p.timer);
        pending.delete(message.id);
        if (message.version !== 1) p.reject(new Error('Companion version mismatch. Update both components.'));
        else if (message.error) p.reject(new Error(message.error.message));
        else p.resolve(message.result);
    });
    port.onDisconnect.addListener(() => {
        const message =
            port?.error?.message || 'Companion disconnected. Install the companion or open Settings for diagnostics.';
        port = undefined;
        registration = undefined;
        registered = false;
        state.connected = false;
        state.cleanupReady = false;
        state.cleanupError = message;
        for (const p of pending.values()) {
            clearTimeout(p.timer);
            p.reject(new Error(message));
        }
        pending.clear();
        publish();
    });
    return port;
}
function native<T = any>(method: string, params: Record<string, unknown> = {}): Promise<T> {
    return new Promise((resolve, reject) => {
        const id = crypto.randomUUID();
        const timer = setTimeout(
            () => {
                pending.delete(id);
                reject(new Error('The companion did not respond in time. Check DDEV before retrying.'));
            },
            method === 'action' ? 21 * 60 * 1000 : 60000,
        );
        pending.set(id, { resolve, reject, timer });
        try {
            connect().postMessage({ version: 1, id, method, ...params });
        } catch (e) {
            clearTimeout(timer);
            pending.delete(id);
            reject(e);
        }
    });
}
function secret() {
    return Array.from(crypto.getRandomValues(new Uint8Array(32)), (n) => n.toString(16).padStart(2, '0')).join('');
}
async function register() {
    if (registered) return;
    if (registration) return registration;
    registration = (async () => {
        const saved = await browser.storage.local.get('profile');
        const profile = saved.profile || { id: secret(), proof: secret() };
        if (!saved.profile) await browser.storage.local.set({ profile });
        const result = await native<{ url: string }>('register', { profileId: profile.id, proof: profile.proof });
        const u = new URL(result.url);
        if (u.protocol !== 'http:' || u.hostname !== '127.0.0.1' || u.pathname !== '/uninstall')
            throw new Error('Invalid companion cleanup address.');
        await browser.runtime.setUninstallURL(u.href);
        registered = true;
        state.cleanupReady = true;
        state.cleanupError = undefined;
    })()
        .catch((e) => {
            registration = undefined;
            registered = false;
            state.cleanupReady = false;
            state.cleanupError = errorText(e);
        })
        .finally(() => {
            registration = undefined;
            publish();
        });
    return registration;
}
function errorText(e: unknown) {
    return e instanceof Error ? e.message : String(e);
}
function applyCompletedStatus(id: string, action: string) {
    const surface = state.surfaces.find((item) => item.id === id);
    if (!surface) return;
    if (action === 'stop') surface.status = 'stopped';
    else if (['start', 'restart', 'open'].includes(action)) surface.status = 'running';
}
function serviceKey(surface: Surface) {
    return surfaceType(surface, state.settings.overrides).trim().toLocaleLowerCase();
}
function loadServices(surface: Surface) {
    const key = serviceKey(surface);
    const cached = services.get(key);
    if (cached) return Promise.resolve(cached);
    const existing = serviceLoads.get(key);
    if (existing) return existing;
    const loading = native<Service[]>('services', { surfaceId: surface.id })
        .then((items) => {
            const template = { source: surface, services: items };
            services.set(key, template);
            return template;
        })
        .finally(() => serviceLoads.delete(key));
    serviceLoads.set(key, loading);
    return loading;
}
function servicesFor(template: ServiceTemplate, surface: Surface) {
    let source: URL;
    let target: URL;
    try {
        source = new URL(template.source.url);
        target = new URL(surface.url);
    } catch {
        return template.services;
    }
    return template.services.map((service) => {
        try {
            const url = new URL(service.url);
            if (url.hostname === source.hostname) url.hostname = target.hostname;
            else if (url.hostname.endsWith(`.${source.hostname}`))
                url.hostname = `${url.hostname.slice(0, -source.hostname.length)}${target.hostname}`;
            return { ...service, url: url.href };
        } catch {
            return service;
        }
    });
}
function warmServices() {
    const types = new Set<string>();
    for (const surface of state.surfaces) {
        const key = serviceKey(surface);
        if (surface.warning || types.has(key) || services.has(key)) continue;
        types.add(key);
        void loadServices(surface).catch(() => {});
    }
}
async function refresh(silent = false) {
    if (refreshing) return refreshing;
    refreshing = (async () => {
        await ready;
        const previousConnected = state.connected;
        const previousError = state.error;
        let changed = false;
        if (!silent) {
            state.loading = true;
            state.error = undefined;
            publish();
        }
        try {
            const surfaces = await native<Surface[]>('discover');
            changed = JSON.stringify(surfaces) !== JSON.stringify(state.surfaces);
            state.surfaces = surfaces;
            state.connected = true;
            state.error = undefined;
            if (changed) state.updatedAt = Date.now();
            warmServices();
            await register();
        } catch (e) {
            state.error = errorText(e);
        } finally {
            state.loading = false;
            refreshing = undefined;
            if (!silent || changed || previousConnected !== state.connected || previousError !== state.error) publish();
        }
    })();
    return refreshing;
}
function enqueue(ids: string[], action: string, serviceId?: string) {
    if (!['start', 'restart', 'stop', 'open', 'ide'].includes(action)) throw new Error('Unsupported action.');
    const known = new Set(state.surfaces.map((s) => s.id));
    const unique = [...new Set(ids)];
    if (!unique.length || unique.some((id) => !known.has(id)))
        throw new Error('Refresh the task list before retrying.');
    if (unique.some((id) => state.operations[id]?.state === 'busy'))
        throw new Error('An operation is already running for this task.');
    unique.forEach(
        (id) =>
            (state.operations[id] = {
                surfaceId: id,
                label:
                    action === 'ide'
                        ? 'Opening IDE…'
                        : action === 'open'
                          ? 'Opening…'
                          : action === 'stop'
                            ? 'Stopping…'
                            : action === 'restart'
                              ? 'Restarting…'
                              : 'Starting…',
                state: 'busy',
            }),
    );
    publish();
    queue = queue
        .then(async () => {
            for (const id of unique) {
                try {
                    const result = await native<{ url?: string }>('action', { surfaceId: id, action, serviceId });
                    if (action === 'open') {
                        if (!result.url) throw new Error('No URL returned.');
                        await browser.tabs.create({ url: safeURL(result.url) });
                    }
                    applyCompletedStatus(id, action);
                    state.operations[id] = { surfaceId: id, label: 'Done', state: 'success' };
                } catch (e) {
                    state.operations[id] = { surfaceId: id, label: 'Failed', state: 'error', message: errorText(e) };
                }
                publish();
            }
            await refresh();
        })
        .catch((e) => {
            state.error = errorText(e);
            publish();
        });
}
browser.runtime.onMessage.addListener((message: any) => {
    if (!message || typeof message.type !== 'string' || message.type === 'state') return;
    return (async () => {
        await ready;
        switch (message.type) {
            case 'getState':
                void refresh(true);
                return state;
            case 'refresh':
                await refresh();
                return state;
            case 'action':
                if (
                    message.action === 'open' &&
                    message.serviceId &&
                    typeof message.serviceUrl === 'string' &&
                    message.ids?.length === 1 &&
                    state.surfaces.find((surface) => surface.id === message.ids[0])?.status === 'running'
                ) {
                    await browser.tabs.create({ url: message.serviceUrl });
                    return { ok: true };
                }
                enqueue(message.ids, message.action, message.serviceId);
                return { ok: true };
            case 'services': {
                const surface = state.surfaces.find((item) => item.id === message.id);
                if (!surface) throw new Error('Refresh the task list before retrying.');
                return servicesFor(await loadServices(surface), surface);
            }
            case 'diagnostics':
                return native('diagnostics');
            case 'saveSettings': {
                const settings = message.settings as Settings;
                if (!settings || typeof settings.overrides !== 'object' || Array.isArray(settings.overrides))
                    throw new Error('Invalid naming overrides.');
                for (const o of Object.values(settings.overrides)) {
                    if (
                        !o ||
                        typeof o !== 'object' ||
                        (o.product !== undefined && typeof o.product !== 'string') ||
                        (o.surface !== undefined && typeof o.surface !== 'string')
                    )
                        throw new Error('Each override must contain product and/or surface text.');
                }
                await native('configure', {
                    config: {
                        ddevPath: settings.ddevPath || '',
                        gitPath: settings.gitPath || '',
                        idePath: settings.idePath || '',
                    },
                });
                state.settings = settings;
                await browser.storage.local.set({ settings });
                await refresh();
                return { ok: true };
            }
            default:
                throw new Error('Unknown extension request.');
        }
    })();
});
void ready.then(async () => {
    await refresh();
    setInterval(() => void refresh(true), 5000);
});
