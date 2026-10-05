import { describe, it, expect } from 'vitest';
import { groupTasks, filterTasks, safeURL, surfaceType, type Surface } from '../extension/model';
const s = (name: string, branch: string, repository: string, status = 'stopped'): Surface => ({
    id: name,
    name,
    branch,
    repository,
    status,
    root: '/projects/' + name,
    url: 'https://example.test',
});
describe('task discovery model', () => {
    it('groups invitation surfaces by full branch and searches without dropping siblings', () => {
        const tasks = groupTasks([
            s('inventory-sync-api', 'feat/inventory-sync', 'example-api'),
            s('inventory-sync-app', 'feat/inventory-sync', 'example-app'),
        ]);
        expect(tasks).toHaveLength(1);
        expect(tasks[0].name).toBe('inventory-sync');
        expect(tasks[0].surfaces.map((x) => x.label)).toEqual(['API', 'App']);
        expect(filterTasks(tasks, 'API')[0].surfaces).toHaveLength(2);
        expect(filterTasks(tasks, 'no-match')).toEqual([]);
    });
    it('does not merge display collisions', () => {
        expect(groupTasks([s('a', 'feat/work', 'a-api'), s('b', 'fix/work', 'a-app')])).toHaveLength(2);
    });
    it('groups base branches by product and respects overrides', () => {
        const tasks = groupTasks([
            s('a', 'main', 'example-api'),
            s('b', 'staging', 'example-shop'),
            s('c', 'develop', 'puntomania-laravel'),
        ]);
        expect(tasks).toHaveLength(2);
        expect(tasks[0].surfaces).toHaveLength(2);
        expect(tasks[0].branch).toBe('main · staging');
        expect(tasks[1].branch).toBe('develop');
        expect(
            groupTasks([s('a', 'main', 'a-api')], { a: { product: 'Custom', surface: 'Backend' } })[0],
        ).toMatchObject({ name: 'Custom', surfaces: [{ label: 'Backend' }] });
    });
    it('infers stable surface types and respects naming overrides', () => {
        expect(surfaceType(s('one', 'main', 'product-laravel'))).toBe('API');
        expect(surfaceType(s('two', 'main', 'product-app'))).toBe('App');
        expect(surfaceType(s('two', 'main', 'product-app'), { two: { surface: 'Frontend' } })).toBe('Frontend');
    });
    it('retains detached and missing projects individually', () => {
        expect(
            groupTasks([s('a', '', 'a-api'), { ...s('b', '', 'a-app'), warning: 'Missing directory' }]),
        ).toHaveLength(2);
    });
    it('disambiguates roles and sorts running tasks first', () => {
        const tasks = groupTasks([
            s('a', 'feat/z', 'a-api', 'running'),
            s('b', 'feat/z', 'b-api'),
            s('c', 'feat/a', 'c-app'),
        ]);
        expect(tasks[0].name).toBe('z');
        expect(tasks[0].surfaces.map((x) => x.label)).toEqual(['API · a', 'API · b']);
    });
    it('rejects executable or credential-bearing URLs', () => {
        expect(() => safeURL('javascript:alert(1)')).toThrow();
        expect(() => safeURL('https://a:b@example.test')).toThrow();
        expect(safeURL('https://app.test')).toBe('https://app.test/');
    });
});
