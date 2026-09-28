import { createLucideIcon } from 'lucide-react';

// Gift glyphs lucide doesn't have, drawn on its 24px grid so they match the
// rest of the set.

export const Comet = createLucideIcon('Comet', [
  ['circle', { cx: '7', cy: '17', r: '3.5', key: 'head' }],
  ['path', { d: 'M9.5 14.5 20 4', key: 'tail' }],
  ['path', { d: 'M7 13.5 13 7.5', key: 'tail-upper' }],
  ['path', { d: 'M10.5 17 16.5 11', key: 'tail-lower' }],
]);

export const Ufo = createLucideIcon('Ufo', [
  ['path', { d: 'M7.5 11a4.5 4.5 0 0 1 9 0', key: 'dome' }],
  ['ellipse', { cx: '12', cy: '13.5', rx: '10', ry: '3.5', key: 'saucer' }],
  ['path', { d: 'm8 20-1 2', key: 'beam-left' }],
  ['path', { d: 'M12 20v2', key: 'beam-mid' }],
  ['path', { d: 'm16 20 1 2', key: 'beam-right' }],
]);

export const RingedPlanet = createLucideIcon('RingedPlanet', [
  ['circle', { cx: '12', cy: '12', r: '5.5', key: 'planet' }],
  [
    'path',
    {
      d: 'M17.3 9.7c2.7-.9 4.6-.8 4.9.2.5 1.6-3.6 4.5-9.2 6.5S2.3 18.9 1.8 17.3c-.3-1 1.2-2.4 3.7-3.8',
      key: 'ring',
    },
  ],
]);
