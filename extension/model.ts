export interface Surface {
    id: string;
    name: string;
    root: string;
    branch: string;
    repository: string;
    status: string;
    url: string;
    warning?: string;
}
export interface Service {
    id: string;
    name: string;
    url: string;
    status?: string;
}
export interface Override {
    product?: string;
    surface?: string;
}
export interface Settings {
    overrides: Record<string, Override>;
    ddevPath?: string;
    gitPath?: string;
}
export interface Task {
    id: string;
    name: string;
    branch: string;
    surfaces: (Surface & { label: string })[];
}
export interface Operation {
    surfaceId: string;
    label: string;
    state: 'busy' | 'success' | 'error';
    message?: string;
}
export interface State {
    surfaces: Surface[];
    settings: Settings;
    operations: Record<string, Operation>;
    loading: boolean;
    error?: string;
    connected: boolean;
    cleanupReady: boolean;
    cleanupError?: string;
    updatedAt?: number;
}
const bases = new Set(['main', 'master', 'develop', 'development', 'staging']);
const suffixes = /-(api|app|angular|admin|shop|docs|checklist|laravel)$/i;
export function groupTasks(surfaces: Surface[], overrides: Settings['overrides'] = {}): Task[] {
    const groups = new Map<string, Task>();
    for (const s of surfaces) {
        const repo = s.repository || s.name;
        const suffix = repo.match(suffixes)?.[1]?.toLowerCase();
        const override = overrides[s.id] || {};
        const product = override.product?.trim() || repo.replace(suffixes, '');
        const label =
            override.surface?.trim() ||
            (suffix === 'laravel'
                ? 'API'
                : suffix === 'api'
                  ? 'API'
                  : suffix
                    ? suffix[0].toUpperCase() + suffix.slice(1)
                    : s.name);
        const base = bases.has(s.branch);
        const id = !s.branch ? `project:${s.id}` : base ? `product:${product}` : `branch:${s.branch}`;
        const name = !s.branch ? s.name : base ? product : s.branch.replace(/^[^/]+\//, '');
        const group = groups.get(id) || { id, name, branch: base ? '' : s.branch, surfaces: [] };
        group.surfaces.push({ ...s, label });
        groups.set(id, group);
    }
    for (const task of groups.values()) {
        const counts = new Map<string, number>();
        task.surfaces.forEach((s) => counts.set(s.label, (counts.get(s.label) || 0) + 1));
        task.surfaces.forEach((s) => {
            if ((counts.get(s.label) || 0) > 1) s.label = `${s.label} · ${s.name}`;
        });
        task.surfaces.sort((a, b) => a.label.localeCompare(b.label));
    }
    return [...groups.values()].sort(
        (a, b) =>
            Number(b.surfaces.some((s) => s.status === 'running')) -
                Number(a.surfaces.some((s) => s.status === 'running')) ||
            a.name.localeCompare(b.name) ||
            a.id.localeCompare(b.id),
    );
}
export function filterTasks(tasks: Task[], query: string) {
    const q = query.trim().toLowerCase();
    return tasks.filter((t) =>
        [t.name, t.branch, ...t.surfaces.flatMap((s) => [s.name, s.branch, s.label])].some((v) =>
            v.toLowerCase().includes(q),
        ),
    );
}
export function safeURL(value: string): string {
    const u = new URL(value);
    if (!['https:', 'http:'].includes(u.protocol) || u.username || u.password)
        throw new Error('The companion returned an invalid web address.');
    return u.href;
}
