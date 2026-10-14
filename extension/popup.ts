import { groupTasks, filterTasks, type State, type Service } from './model';
const $ = <T extends HTMLElement>(id: string) => document.getElementById(id) as T;
const search = $<HTMLInputElement>('search');
const runningOnly = $<HTMLInputElement>('running-only');
const tasks = $('tasks');
$('version').textContent = `v${browser.runtime.getManifest().version}`;
let state: State | undefined;
let menu: string | undefined;
const menus = new Map<string, Service[] | string>();
const icons: Record<string, { viewBox?: string; paths: string[]; acorn?: boolean }> = {
    start: {
        paths: [
            'M2.059 3.43c0-1.536 1.659-2.498 2.992-1.738l8 4.571c1.344.768 1.344 2.704 0 3.472l-8 4.571c-1.333.762-2.992-.2-2.992-1.736V3.43Z',
        ],
        acorn: true,
    },
    restart: {
        viewBox: '0 0 20 20',
        paths: [
            'M14.957 5.514a.875.875 0 0 1 1.218.222A7.46 7.46 0 0 1 17.5 9.99a7.5 7.5 0 0 1-7.5 7.492h-.025V19.5a.5.5 0 0 1-.849.356l-3-2.932a.5.5 0 0 1-.005-.71l3-3.009a.5.5 0 0 1 .854.353v2.175H10a5.747 5.747 0 0 0 5.75-5.743 5.7 5.7 0 0 0-1.015-3.26.875.875 0 0 1 .222-1.216Z',
            'M10 .5a.5.5 0 0 1 .855-.352l3 3.009a.5.5 0 0 1-.006.709l-3 2.933A.5.5 0 0 1 10 6.442v-2.197A5.747 5.747 0 0 0 4.25 9.99c0 1.217.377 2.343 1.021 3.271a.875.875 0 0 1-1.437.996A7.46 7.46 0 0 1 2.5 9.99 7.5 7.5 0 0 1 10 2.497V.5Z',
        ],
        acorn: true,
    },
    stop: { viewBox: '0 0 20 20', paths: ['M5 5h10v10H5Z'] },
    delete: {
        paths: [
            'M6.25 6.5a.75.75 0 0 1 .75.75v4.5a.75.75 0 0 1-1.5 0v-4.5a.75.75 0 0 1 .75-.75ZM9.75 6.5a.75.75 0 0 1 .75.75v4.5a.75.75 0 0 1-1.5 0v-4.5a.75.75 0 0 1 .75-.75Z',
            'M8 0c.624 0 1.205.037 1.735.095A2 2 0 0 1 11.5 2.083v1.071c.911.08 1.87.195 2.869.356a.75.75 0 1 1-.238 1.48L14 4.971v8.071a2.75 2.75 0 0 1-2.415 2.731A28.9 28.9 0 0 1 8 16c-1.054 0-2.271-.059-3.615-.23A2.75 2.75 0 0 1 2 13.041V4.97l-.131.02a.75.75 0 1 1-.238-1.48A31.1 31.1 0 0 1 4.5 3.153V2.085A2 2 0 0 1 6.292.091 16 16 0 0 1 8 0Zm0 4.5c-1.637 0-3.145.105-4.5.265v8.277c0 .63.467 1.162 1.074 1.239A27.5 27.5 0 0 0 8 14.5c1.238 0 2.376-.086 3.397-.215a1.25 1.25 0 0 0 1.103-1.243V4.764A39 39 0 0 0 8 4.5Zm0-3c-.459 0-.98.022-1.548.083A.5.5 0 0 0 6 2.085v.967A40 40 0 0 1 8 3c.63 0 1.297.016 2 .051v-.968a.5.5 0 0 0-.427-.497A14.6 14.6 0 0 0 8 1.5Z',
        ],
        acorn: true,
    },
    services: {
        paths: [
            'M13.25 11a.75.75 0 0 1 0 1.5H2.75a.75.75 0 0 1 0-1.5h10.5ZM13.25 7a.75.75 0 0 1 0 1.5H2.75a.75.75 0 0 1 0-1.5h10.5ZM13.25 3a.75.75 0 0 1 0 1.5H2.75a.75.75 0 0 1 0-1.5h10.5Z',
        ],
        acorn: true,
    },
};
const serviceIcons: Record<string, string> = {
    buggregator: 'service-icons/buggregator.png',
    storybook: 'service-icons/storybook.svg',
    'webhook-site': 'service-icons/webhook-site.ico',
};
const externalLinkPaths = [
    'M8 2.5c-1.603 0-3.081.099-4.414.253A1.25 1.25 0 0 0 2.5 3.994v8.009c0 .635.48 1.174 1.109 1.246A38.5 38.5 0 0 0 8 13.5c1.603 0 3.081-.099 4.414-.253A1.25 1.25 0 0 0 13.5 12.006V8H15v4.006a2.75 2.75 0 0 1-2.414 2.731A40 40 0 0 1 8 15c-1.354 0-2.888-.068-4.562-.261A2.75 2.75 0 0 1 1 12.003V3.994a2.75 2.75 0 0 1 2.414-2.731A40 40 0 0 1 8 1v1.5Z',
    'M14.519 1c.265 0 .48.215.48.48v3.639a.48.48 0 0 1-.819.339l-1.288-1.289-4.611 4.611a.75.75 0 1 1-1.061-1.061l4.61-4.611-1.289-1.289A.48.48 0 0 1 10.88 1h3.639Z',
];
function svgIcon(paths: string[], viewBox = '0 0 16 16') {
    const svg = document.createElementNS('http://www.w3.org/2000/svg', 'svg');
    svg.setAttribute('viewBox', viewBox);
    svg.setAttribute('aria-hidden', 'true');
    for (const d of paths) {
        const path = document.createElementNS(svg.namespaceURI, 'path');
        path.setAttribute('d', d);
        svg.append(path);
    }
    return svg;
}
function button(label: string, icon: string, fn: () => void, key: string) {
    const b = document.createElement('button');
    b.className = 'icon-button';
    b.title = label;
    b.setAttribute('aria-label', label);
    b.dataset.key = key;
    const definition = icons[icon];
    const svg = svgIcon(definition.paths, definition.viewBox);
    if (definition.acorn) svg.classList.add('acorn-icon');
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
function selectMenuAction(ids: string[], kind: string, serviceId?: string) {
    menu = undefined;
    render();
    void action(ids, kind, serviceId);
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
        for (const kind of ['start', 'restart', 'stop'] as const) {
            const eligible =
                kind === 'start'
                    ? ids.filter((id) =>
                          all
                              .find((item) => item.id === task.id)
                              ?.surfaces.some((s) => s.id === id && s.status === 'stopped'),
                      )
                    : kind === 'restart'
                      ? ids.filter((id) =>
                            all
                                .find((item) => item.id === task.id)
                                ?.surfaces.some((s) => s.id === id && s.status === 'running'),
                        )
                      : ids;
            const verb =
                kind === 'start'
                    ? 'Start all stopped surfaces in'
                    : kind === 'restart'
                      ? 'Restart all running surfaces in'
                      : 'Stop all surfaces in';
            const b = button(`${verb} ${task.name}`, kind, () => void action(eligible, kind), `${task.id}:${kind}`);
            b.disabled = busy || !eligible.length;
            actions.append(b);
        }
        const deleteTask = button(
            `Delete task ${task.name}`,
            'delete',
            () => {
                if (confirm(`Delete task ${task.name} and all its surfaces and branches?`)) void action(ids, 'delete');
            },
            `${task.id}:delete`,
        );
        deleteTask.disabled = busy;
        actions.append(deleteTask);
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
            for (const k of [kind, 'stop'] as const) {
                const b = button(
                    `${k[0].toUpperCase() + k.slice(1)} ${s.name}`,
                    k,
                    () => void action([s.id], k),
                    `${s.id}:${k}`,
                );
                b.disabled = running || !!s.warning || (k === 'stop' && s.status === 'stopped');
                row.append(b);
            }
            const deleteSurface = button(
                `Delete ${s.name}`,
                'delete',
                () => {
                    if (confirm(`Delete ${s.name} and its branch?`)) void action([s.id], 'delete');
                },
                `${s.id}:delete`,
            );
            deleteSurface.disabled = running || !!s.warning;
            row.append(deleteSurface);
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
                ide.className = 'service-menu-item';
                ide.append(svgIcon(externalLinkPaths), text('span', 'Open in IDE'));
                ide.dataset.key = `${s.id}:ide`;
                ide.disabled = running;
                ide.onclick = () => selectMenuAction([s.id], 'ide');
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
                        b.className = 'service-menu-item';
                        const icon = document.createElement('img');
                        icon.className = 'service-icon';
                        icon.src = serviceIcons[item.id] || 'service-icons/generic.svg';
                        icon.alt = '';
                        icon.setAttribute('aria-hidden', 'true');
                        b.append(icon, text('span', item.name));
                        b.dataset.key = `${s.id}:service:${item.id}`;
                        b.disabled = running;
                        b.onclick = () => selectMenuAction([s.id], 'open', item.id);
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
