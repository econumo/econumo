// Generates web/dist/types/index.d.ts — the types barrel design-sync consumes
// (web/package.json "types" points here). Re-exports every component
// declaration under dist/types/components/{,ui/} except tests. Run after
// `tsc -p tsconfig.dts.json`; wired into .design-sync/config.json buildCmd.
import { readdirSync, writeFileSync, existsSync } from 'node:fs';
import { join, dirname } from 'node:path';
import { fileURLToPath } from 'node:url';

const webDir = join(dirname(fileURLToPath(import.meta.url)), '..', 'web');
const typesRoot = join(webDir, 'dist', 'types');
const compDir = join(typesRoot, 'components');
if (!existsSync(compDir)) {
  console.error('gen-dts-barrel: web/dist/types/components missing — run tsc -p tsconfig.dts.json first');
  process.exit(1);
}
const lines = [];
for (const sub of ['', 'ui']) {
  const dir = join(compDir, sub);
  if (!existsSync(dir)) continue;
  for (const f of readdirSync(dir).sort()) {
    if (!f.endsWith('.d.ts') || f.endsWith('.test.d.ts')) continue;
    const mod = ['./components', sub, f.replace(/\.d\.ts$/, '')].filter(Boolean).join('/');
    lines.push(`export * from '${mod}';`);
  }
}
writeFileSync(join(typesRoot, 'index.d.ts'), lines.join('\n') + '\n');
console.error(`gen-dts-barrel: wrote ${lines.length} re-exports to dist/types/index.d.ts`);
