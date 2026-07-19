// Generates .design-sync/.cache/utility-corpus.txt — a Tailwind @source scan
// file listing common layout/typography/color utilities so the DS stylesheet
// shipped to claude.ai/design carries vocabulary beyond the exact classes the
// app happens to use (authored previews and the design agent's own layout
// glue depend on it). Wired into buildCmd before the tailwindcss CLI run.
import { mkdirSync, writeFileSync } from 'node:fs';
import { join, dirname } from 'node:path';
import { fileURLToPath } from 'node:url';

const out = [];
const push = (...cs) => out.push(...cs);
const cross = (prefixes, values) => prefixes.flatMap((p) => values.map((v) => `${p}-${v}`));

const SPACING = ['0', '0.5', '1', '1.5', '2', '2.5', '3', '3.5', '4', '5', '6', '7', '8', '9', '10', '11', '12', '14', '16', '20', '24'];
const SIZES = [...SPACING, '28', '32', '36', '40', '44', '48', '52', '56', '60', '64', '72', '80', '96',
  'full', 'fit', 'auto', 'min', 'max', 'px', '1/2', '1/3', '2/3', '1/4', '3/4', '1/5', '2/5', '3/5', '4/5'];
const TOKENS = ['background', 'foreground', 'card', 'card-foreground', 'popover', 'popover-foreground',
  'primary', 'primary-foreground', 'secondary', 'secondary-foreground', 'muted', 'muted-foreground',
  'accent', 'accent-foreground', 'destructive', 'border', 'input', 'ring', 'income', 'expense',
  'econumo-yellow', 'econumo-yellow-text', 'econumo-magenta', 'econumo-magenta-dark',
  'econumo-magenta-light', 'econumo-purple', 'econumo-card', 'econumo-hover',
  'sidebar', 'sidebar-foreground', 'sidebar-primary', 'sidebar-accent', 'sidebar-border',
  'chart-1', 'chart-2', 'chart-3', 'chart-4', 'chart-5', 'white', 'black', 'transparent', 'current'];

push('flex', 'inline-flex', 'grid', 'inline-grid', 'block', 'inline-block', 'inline', 'hidden', 'contents');
push('flex-row', 'flex-row-reverse', 'flex-col', 'flex-col-reverse', 'flex-wrap', 'flex-nowrap',
  'flex-1', 'flex-auto', 'flex-none', 'grow', 'grow-0', 'shrink', 'shrink-0');
push(...cross(['items'], ['start', 'center', 'end', 'baseline', 'stretch']));
push(...cross(['justify'], ['start', 'center', 'end', 'between', 'around', 'evenly', 'stretch']));
push(...cross(['self'], ['auto', 'start', 'center', 'end', 'stretch']));
push(...cross(['content'], ['start', 'center', 'end', 'between']));
push(...cross(['grid-cols'], ['1', '2', '3', '4', '5', '6', '7', '8', '9', '10', '11', '12', 'none']));
push(...cross(['col-span'], ['1', '2', '3', '4', '5', '6', 'full']));
push(...cross(['grid-rows'], ['1', '2', '3', '4', '5', '6']));
push(...cross(['row-span'], ['1', '2', '3', 'full']));
push(...cross(['gap', 'gap-x', 'gap-y'], SPACING));
push(...cross(['space-x', 'space-y'], SPACING.slice(0, 13)));
push(...cross(['p', 'px', 'py', 'pt', 'pr', 'pb', 'pl'], SPACING));
push(...cross(['m', 'mx', 'my', 'mt', 'mr', 'mb', 'ml'], [...SPACING, 'auto']));
push(...cross(['w'], [...SIZES, 'screen']));
push(...cross(['h'], [...SIZES, 'screen', 'svh', 'dvh']));
push(...cross(['size'], SPACING));
push(...cross(['min-w'], ['0', 'full', 'fit', 'min', 'max']));
push(...cross(['min-h'], ['0', 'full', 'screen', 'svh', 'dvh', 'fit']));
push(...cross(['max-w'], ['none', 'xs', 'sm', 'md', 'lg', 'xl', '2xl', '3xl', '4xl', '5xl', '6xl', '7xl', 'prose', ...SIZES]));
push(...cross(['max-h'], ['screen', 'svh', 'dvh', ...SIZES]));
push(...cross(['text'], ['xs', 'sm', 'base', 'lg', 'xl', '2xl', '3xl', '4xl', '5xl', '6xl',
  'left', 'center', 'right', 'justify']));
push(...cross(['font'], ['thin', 'extralight', 'light', 'normal', 'medium', 'semibold', 'bold', 'extrabold', 'sans', 'heading']));
push(...cross(['leading'], ['none', 'tight', 'snug', 'normal', 'relaxed', 'loose']));
push(...cross(['tracking'], ['tighter', 'tight', 'normal', 'wide', 'wider', 'widest']));
push('truncate', 'line-clamp-1', 'line-clamp-2', 'line-clamp-3', 'line-clamp-4',
  'whitespace-nowrap', 'whitespace-pre-line', 'break-words', 'break-all',
  'italic', 'not-italic', 'underline', 'no-underline', 'line-through',
  'uppercase', 'lowercase', 'capitalize', 'normal-case', 'tabular-nums', 'antialiased');
push(...cross(['bg', 'text', 'border'], TOKENS));
push(...cross(['bg'], TOKENS.flatMap((t) => ['primary', 'secondary', 'muted', 'accent', 'destructive', 'foreground', 'card'].includes(t)
  ? ['10', '20', '30', '40', '50', '60', '70', '80', '90'].map((o) => `${t}/${o}`) : [])));
push(...cross(['text'], ['primary', 'destructive', 'muted-foreground', 'foreground'].flatMap((t) =>
  ['50', '60', '70', '80', '90'].map((o) => `${t}/${o}`))));
push('border', 'border-0', 'border-2', 'border-4', 'border-t', 'border-r', 'border-b', 'border-l',
  'border-x', 'border-y', 'border-dashed', 'border-dotted', 'border-none');
push('rounded', ...cross(['rounded'], ['none', 'sm', 'md', 'lg', 'xl', '2xl', '3xl', 'full']),
  ...cross(['rounded-t', 'rounded-b', 'rounded-l', 'rounded-r'], ['lg', 'xl', '2xl', 'full', 'none']));
push(...cross(['shadow'], ['2xs', 'xs', 'sm', 'md', 'lg', 'xl', '2xl', 'none']));
push('ring', 'ring-1', 'ring-2', 'ring-ring', 'ring-border', 'outline-none', 'outline-hidden');
push('divide-y', 'divide-x', 'divide-border');
push('relative', 'absolute', 'fixed', 'sticky', 'static', 'inset-0', 'inset-x-0', 'inset-y-0');
push(...cross(['top', 'right', 'bottom', 'left'], ['0', '1', '2', '3', '4', '6', '8', 'auto', '1/2', 'full']));
push(...cross(['z'], ['0', '10', '20', '30', '40', '50', 'auto']));
push(...cross(['overflow', 'overflow-x', 'overflow-y'], ['auto', 'hidden', 'scroll', 'visible']));
push('container', 'mx-auto', 'object-cover', 'object-contain', 'object-center',
  'aspect-square', 'aspect-video', 'aspect-auto');
push('cursor-pointer', 'cursor-default', 'cursor-not-allowed', 'select-none', 'select-text',
  'pointer-events-none', 'pointer-events-auto', 'sr-only', 'not-sr-only', 'scrollbar-none', 'scrollbar-slim');
push('transition', 'transition-all', 'transition-colors', 'transition-opacity', 'transition-transform',
  ...cross(['duration'], ['75', '100', '150', '200', '300', '500']),
  ...cross(['opacity'], ['0', '5', '10', '20', '25', '30', '40', '50', '60', '70', '75', '80', '90', '95', '100']),
  ...cross(['scale'], ['0', '50', '75', '90', '95', '100', '105', '110']),
  ...cross(['rotate'], ['0', '45', '90', '180']),
  ...cross(['translate-x', 'translate-y'], ['0', '1', '2', '4', '1/2', 'full']));

// Responsive/system-state variants for the highest-traffic utilities.
const RESPONSIVE_BASE = ['flex', 'hidden', 'grid', 'block', 'inline-flex', 'flex-row', 'flex-col',
  'grid-cols-1', 'grid-cols-2', 'grid-cols-3', 'grid-cols-4', 'grid-cols-6', 'grid-cols-12',
  'items-center', 'justify-between', 'justify-start', 'justify-center', 'justify-end',
  'w-auto', 'w-full', 'w-fit', 'w-1/2', 'w-1/3', 'w-2/3', 'w-64', 'w-80', 'w-96',
  'max-w-sm', 'max-w-md', 'max-w-lg', 'max-w-xl', 'max-w-2xl', 'max-w-4xl',
  'h-auto', 'h-full', 'px-4', 'px-6', 'px-8', 'py-4', 'py-6', 'py-8', 'p-4', 'p-6', 'p-8',
  'gap-2', 'gap-3', 'gap-4', 'gap-6', 'gap-8', 'mx-auto', 'mt-0', 'mb-0',
  'text-xs', 'text-sm', 'text-base', 'text-lg', 'text-xl', 'text-2xl', 'text-3xl',
  'text-left', 'text-center', 'col-span-1', 'col-span-2', 'col-span-3', 'order-first', 'order-last'];
for (const bp of ['sm', 'md', 'lg', 'xl']) push(...RESPONSIVE_BASE.map((c) => `${bp}:${c}`));
push(...['bg-muted', 'bg-accent', 'bg-primary/90', 'bg-secondary/80', 'bg-destructive/90',
  'bg-econumo-hover', 'text-foreground', 'text-primary', 'underline', 'opacity-80', 'opacity-100',
  'shadow-md', 'scale-105'].map((c) => `hover:${c}`));
push(...['ring-2', 'ring-ring', 'outline-none', 'border-ring'].map((c) => `focus-visible:${c}`));
push(...['opacity-50', 'pointer-events-none', 'cursor-not-allowed'].map((c) => `disabled:${c}`));
push(...['rotate-180', 'bg-accent', 'text-accent-foreground'].map((c) => `data-[state=open]:${c}`));
push(...['bg-background', 'bg-card', 'bg-muted', 'text-foreground', 'text-muted-foreground',
  'border-border', 'bg-input/30'].map((c) => `dark:${c}`));
push('group', 'group-hover:opacity-100', 'group-hover:underline', 'peer');

const dir = join(dirname(fileURLToPath(import.meta.url)), '.cache');
mkdirSync(dir, { recursive: true });
writeFileSync(join(dir, 'utility-corpus.txt'), [...new Set(out)].join('\n') + '\n');
console.error(`gen-utility-corpus: ${new Set(out).size} class candidates`);
