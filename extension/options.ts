const el = <T extends HTMLElement>(id: string) => document.getElementById(id) as T;
async function check() {
    try {
        const d = await browser.runtime.sendMessage({ type: 'diagnostics' });
        el('diagnostics').textContent =
            `Companion connected · ${d.platform}\nAutomatic removal: ${d.cleanupReady ? 'ready' : d.cleanupError || 'unavailable'}`;
        if (!el<HTMLInputElement>('ddev').value) el<HTMLInputElement>('ddev').value = d.config.ddevPath;
        if (!el<HTMLInputElement>('git').value) el<HTMLInputElement>('git').value = d.config.gitPath;
        if (!el<HTMLInputElement>('ide').value) el<HTMLInputElement>('ide').value = d.config.idePath || '';
    } catch (e) {
        el('diagnostics').textContent =
            `Companion unavailable. Install it, or run ddev-manager doctor.\n${e instanceof Error ? e.message : String(e)}`;
    }
}
el('check').onclick = () => void check();
el<HTMLFormElement>('form').onsubmit = async (e) => {
    e.preventDefault();
    try {
        const overrides = JSON.parse(el<HTMLTextAreaElement>('overrides').value || '{}');
        await browser.runtime.sendMessage({
            type: 'saveSettings',
            settings: {
                overrides,
                ddevPath: el<HTMLInputElement>('ddev').value.trim(),
                gitPath: el<HTMLInputElement>('git').value.trim(),
                idePath: el<HTMLInputElement>('ide').value.trim(),
            },
        });
        el('result').textContent = 'Settings saved.';
        await check();
    } catch (e) {
        el('result').textContent = e instanceof Error ? e.message : String(e);
    }
};
void browser.storage.local.get('settings').then((s) => {
    el<HTMLTextAreaElement>('overrides').value = JSON.stringify(s.settings?.overrides || {}, null, 2);
});
void check();
