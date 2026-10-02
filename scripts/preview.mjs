import { createServer } from 'node:http';
import { readFile } from 'node:fs/promises';
const types = { '.html': 'text/html', '.js': 'text/javascript', '.css': 'text/css', '.svg': 'image/svg+xml' };
createServer(async (req, res) => {
    try {
        const path = new URL(req.url, 'http://localhost').pathname;
        if (!/^\/[a-zA-Z0-9.-]+$/.test(path)) throw Error();
        const file = await readFile(`dist/extension${path}`);
        res.setHeader('Content-Type', types[path.slice(path.lastIndexOf('.'))] || 'application/octet-stream');
        res.end(file);
    } catch {
        res.writeHead(404);
        res.end();
    }
}).listen(43118, '127.0.0.1');
