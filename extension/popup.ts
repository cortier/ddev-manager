import { groupTasks, filterTasks, type State, type Service } from './model';
const $ = <T extends HTMLElement>(id: string) => document.getElementById(id) as T;
const search = $<HTMLInputElement>('search');
const runningOnly = $<HTMLInputElement>('running-only');
const tasks = $('tasks');
let state: State | undefined;
let menu: string | undefined;
const menus = new Map<string, Service[] | string>();
const paths: Record<string, string> = {
    start: 'M5 3l10 7-10 7Z',
    restart: 'M16 7A7 7 0 1 0 17 12M16 2v5h-5',
    stop: 'M5 5h10v10H5Z',
    services: 'M10 4v.01M10 10v.01M10 16v.01',
};
function button(label: string, icon: string, fn: () => void, key: string) {
    const b = document.createElement('button');
    b.className = 'icon-button';
    b.title = label;
    b.setAttribute('aria-label', label);
    b.dataset.key = key;
    const svg = document.createElementNS('http://www.w3.org/2000/svg', 'svg');
    svg.setAttribute('viewBox', '0 0 20 20');
    svg.setAttribute('aria-hidden', 'true');
    const p = document.createElementNS(svg.namespaceURI, 'path');
    p.setAttribute('d', paths[icon]);
    if (icon === 'services') {
        p.setAttribute('stroke-width', '3');
    }
    svg.append(p);
    b.append(svg);
    b.onclick = fn;
    return b;
}
function text(tag: string, value: string, className = '') {
    const e = document.createElement(tag);
    e.textContent = value;
    e.className = className;
    return e;
}
function error(e: unknown) {
    $('notice').textContent = e instanceof Error ? e.message : String(e);
}
async function action(ids: string[], kind: string, serviceId?: string) {
    try {
        await browser.runtime.sendMessage({ type: 'action', ids, action: kind, serviceId });
    } catch (e) {
        error(e);
    }
}
async function toggleServices(id: string) {
    menu = menu === id ? undefined : id;
    render();
    if (menu && !menus.has(id)) {
        try {
            menus.set(id, await browser.runtime.sendMessage({ type: 'services', id }));
        } catch (e) {
            menus.set(id, e instanceof Error ? e.message : String(e));
        }
        render();
    }
}
function render() {
    if (!state) return;
    const focused = (document.activeElement as HTMLElement)?.dataset.key;
    const scroll = tasks.scrollTop;
    tasks.replaceChildren();
    const all = groupTasks(state.surfaces, state.settings.overrides);
    const statusFiltered = runningOnly.checked
        ? all
              .filter((task) => task.surfaces.some((surface) => surface.status === 'running'))
              .map((task) => ({
                  ...task,
                  surfaces: task.surfaces.filter((surface) => surface.status === 'running'),
              }))
        : all;
    const visible = filterTasks(statusFiltered, search.value);
    $('notice').textContent =
        state.error ||
        (!state.cleanupReady && state.connected
            ? 'Automatic removal is unavailable. Open Settings to check the cleanup service.'
            : '');
    $('summary').textContent = state.loading
        ? 'Finding projects…'
        : `${all.length} tasks · ${state.surfaces.length} surfaces`;
    if (!visible.length)
        tasks.append(
            text(
                'p',
                state.loading
                    ? 'Finding DDEV projects…'
                    : search.value || runningOnly.checked
                      ? 'No tasks match your filters.'
                      : state.connected
                        ? 'No registered DDEV projects. New projects will appear automatically.'
                        : 'Install the companion. Projects will appear automatically once connected.',
                'empty',
            ),
        );
    for (const task of visible) {
        const section = text('section', '', 'task');
        const heading = text('div', '', 'task-heading');
        const title = text('h2', task.name);
        title.title = task.branch || task.name;
        heading.append(title);
        const actions = text('div', '', 'actions');
        const ids = all.find((item) => item.id === task.id)?.surfaces.map((surface) => surface.id) || [];
        const busy = ids.some((id) => state?.operations[id]?.state === 'busy');
        for (const kind of ['start', 'restart', 'stop']) {
            const label = `${kind[0].toUpperCase() + kind.slice(1)} all surfaces in ${task.name}`;
            const b = button(label, kind, () => void action(ids, kind), `${task.id}:${kind}`);
            b.disabled = busy;
            actions.append(b);
        }
        heading.append(actions);
        section.append(heading);
        if (task.branch) section.append(text('p', task.branch, 'branch'));
        for (const s of task.surfaces) {
            const operation = state.operations[s.id];
            const running = operation?.state === 'busy';
            const row = text('div', '', 'surface-row');
            const open = document.createElement('button');
            open.className = 'surface-open';
            open.dataset.key = `${s.id}:open`;
            open.title = `Open ${s.name}\n${s.root}`;
            open.setAttribute('aria-label', `Open ${s.label}, ${s.status}`);
            open.disabled = running || !!s.warning;
            const dot = text('span', '', `status-dot ${s.status}`);
            dot.setAttribute('aria-hidden', 'true');
            open.append(dot, text('span', s.label, 'surface-label'));
            open.onclick = () => void action([s.id], 'open');
            row.append(open);
            const kind = s.status === 'running' ? 'restart' : 'start';
            for (const k of [kind, 'stop']) {
                const b = button(
                    `${k[0].toUpperCase() + k.slice(1)} ${s.name}`,
                    k,
                    () => void action([s.id], k),
                    `${s.id}:${k}`,
                );
                b.disabled = running || !!s.warning || (k === 'stop' && s.status === 'stopped');
                row.append(b);
            }
            const serviceButton = button(
                `Services for ${s.name}`,
                'services',
                () => void toggleServices(s.id),
                `${s.id}:services`,
            );
            serviceButton.setAttribute('aria-expanded', String(menu === s.id));
            serviceButton.disabled = !!s.warning;
            row.append(serviceButton);
            section.append(row);
            if (menu === s.id) {
                const list = text('div', '', 'service-menu');
                const ide = document.createElement('button');
                ide.textContent = 'Open in IDE';
                ide.dataset.key = `${s.id}:ide`;
                ide.disabled = running;
                ide.onclick = () => void action([s.id], 'ide');
                list.append(ide);
                const divider = document.createElement('hr');
                divider.className = 'service-divider';
                list.append(divider);
                const items = menus.get(s.id);
                if (!items) list.append(text('p', 'Finding services…'));
                else if (typeof items === 'string') list.append(text('p', items));
                else if (!items.length) list.append(text('p', 'No browser services found.'));
                else
                    for (const item of items) {
                        const b = document.createElement('button');
                        b.textContent = item.name;
                        b.title = item.url;
                        b.dataset.key = `${s.id}:service:${item.id}`;
                        b.disabled = running;
                        b.onclick = () => void action([s.id], 'open', item.id);
                        list.append(b);
                    }
                section.append(list);
            }
        }
        tasks.append(section);
    }
    tasks.scrollTop = scroll;
    if (focused)
        for (const el of tasks.querySelectorAll<HTMLElement>('[data-key]'))
            if (el.dataset.key === focused) {
                el.focus({ preventScroll: true });
                break;
            }
}
search.oninput = render;
runningOnly.onchange = () => {
    void browser.storage.local.set({ runningOnly: runningOnly.checked }).catch(error);
    render();
};
$('settings').onclick = () => void browser.runtime.openOptionsPage();
document.addEventListener('keydown', (e) => {
    if (e.key === 'Escape' && menu) {
        menu = undefined;
        render();
        e.preventDefault();
    }
});
browser.runtime.onMessage.addListener((m: any) => {
    if (m.type === 'state') {
        if (m.state.updatedAt !== state?.updatedAt) {
            menus.clear();
            menu = undefined;
        }
        state = m.state;
        render();
    }
});
void Promise.all([browser.runtime.sendMessage({ type: 'getState' }), browser.storage.local.get('runningOnly')])
    .then(([s, saved]) => {
        runningOnly.checked = saved.runningOnly === true;
        state = s;
        render();
    })
    .catch(error);
