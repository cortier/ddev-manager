import { execFileSync } from 'node:child_process';
execFileSync(
    'npx',
    ['web-ext', 'build', '--source-dir', 'dist/extension', '--artifacts-dir', 'dist', '--overwrite-dest'],
    { stdio: 'inherit' },
);
