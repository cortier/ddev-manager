import { build } from 'esbuild';
import { mkdir, copyFile, readdir } from 'node:fs/promises';
await mkdir('dist/extension', { recursive: true });
await build({
    entryPoints: ['extension/background.ts', 'extension/popup.ts', 'extension/options.ts'],
    outdir: 'dist/extension',
    bundle: true,
    target: 'firefox140',
    format: 'iife',
    legalComments: 'none',
});
for (const file of await readdir('extension'))
    if (/\.(html|css|json|svg)$/.test(file)) await copyFile(`extension/${file}`, `dist/extension/${file}`);
