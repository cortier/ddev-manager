type MockConfig = {
    state?: unknown;
    services?: Record<string, unknown[]>;
    settings?: unknown;
    diagnostics?: unknown;
    diagnosticsError?: string;
};

function safeJSON(value: unknown) {
    return JSON.stringify(value).replaceAll('<', '\\u003c');
}

export function extensionFrame(source: string, config: MockConfig, page: 'popup' | 'options', after = '') {
    const frame = document.createElement('iframe');
    frame.title = `DDEV Manager ${page} preview`;
    frame.style.display = 'block';
    frame.style.width = page === 'popup' ? '360px' : '760px';
    frame.style.height = page === 'popup' ? '600px' : '820px';
    frame.style.border = '0';
    frame.style.borderRadius = '8px';
    frame.style.boxShadow = '0 4px 20px rgb(0 0 0 / 14%)';

    const setup = `<script>
        (() => {
            const config = ${safeJSON(config)};
            const listeners = [];
            globalThis.browser = {
                runtime: {
                    getManifest: () => ({ version: '0.1.5' }),
                    openOptionsPage: async () => {},
                    onMessage: { addListener: (listener) => listeners.push(listener) },
                    sendMessage: async (message) => {
                        if (message.type === 'getState') return config.state;
                        if (message.type === 'services') return config.services?.[message.id] || [];
                        if (message.type === 'diagnostics') {
                            if (config.diagnosticsError) throw new Error(config.diagnosticsError);
                            return config.diagnostics;
                        }
                        if (message.type === 'saveSettings') return undefined;
                        if (message.type === 'action') return undefined;
                    },
                },
                storage: {
                    local: {
                        get: async (key) => key === 'settings'
                            ? { settings: config.settings || { overrides: {} } }
                            : { runningOnly: false },
                        set: async () => {},
                    },
                },
            };
        })();
    </script>`;
    const productionScript = `<script src="${page}.js"></script>`;
    frame.srcdoc = source.replace(productionScript, `${setup}${productionScript}${after}`);
    return frame;
}
