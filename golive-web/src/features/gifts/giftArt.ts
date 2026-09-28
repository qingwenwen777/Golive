import type { LucideIcon } from 'lucide-react';
import {
  CakeSlice,
  Castle,
  Crown,
  Donut,
  Flower2,
  Gem,
  Lightbulb,
  Rainbow,
  Rocket,
  Sailboat,
  Soup,
  Star,
  WandSparkles,
} from 'lucide-react';
import { Comet, RingedPlanet, Ufo } from './giftGlyphs';

export interface GiftArtSpec {
  Icon: LucideIcon;
  from: string;
  to: string;
  ink: string;
  tier: 0 | 1 | 2 | 3;
}

const WHITE = '#ffffff';
const GOLD = '#fcd34d';

// The built-in catalogue (gift-service seed). Basic and premium gifts are
// bright tiles; luxury gifts are dark tiles with gold glyphs.
export const GIFT_ART: Record<string, GiftArtSpec> = {
  flower: { Icon: Flower2, from: '#f472b6', to: '#be185d', ink: WHITE, tier: 0 },
  donut: { Icon: Donut, from: '#fb923c', to: '#c2410c', ink: WHITE, tier: 0 },
  cake: { Icon: CakeSlice, from: '#fb7185', to: '#be123c', ink: WHITE, tier: 0 },
  ramen: { Icon: Soup, from: '#fbbf24', to: '#b45309', ink: WHITE, tier: 0 },
  rocket: { Icon: Rocket, from: '#38bdf8', to: '#0369a1', ink: WHITE, tier: 1 },
  starlight: { Icon: Star, from: '#facc15', to: '#a16207', ink: WHITE, tier: 1 },
  fan_light: { Icon: Lightbulb, from: '#e879f9', to: '#a21caf', ink: WHITE, tier: 2 },
  crown: { Icon: Crown, from: '#fbbf24', to: '#92400e', ink: WHITE, tier: 2 },
  gem: { Icon: Gem, from: '#22d3ee', to: '#0e7490', ink: WHITE, tier: 2 },
  aurora: { Icon: Rainbow, from: '#818cf8', to: '#0f766e', ink: WHITE, tier: 2 },
  yacht: { Icon: Sailboat, from: '#1d4ed8', to: '#172554', ink: GOLD, tier: 3 },
  castle: { Icon: Castle, from: '#4f46e5', to: '#1e1b4b', ink: GOLD, tier: 3 },
  meteor: { Icon: Comet, from: '#ea580c', to: '#450a0a', ink: GOLD, tier: 3 },
  galaxy_ship: { Icon: Ufo, from: '#7e22ce', to: '#2e1065', ink: GOLD, tier: 3 },
  royal_crown: { Icon: Crown, from: '#be123c', to: '#4c0519', ink: GOLD, tier: 3 },
  nebula_ring: { Icon: RingedPlanet, from: '#c026d3', to: '#3b0764', ink: GOLD, tier: 3 },
  eternal_scepter: { Icon: WandSparkles, from: '#334155', to: '#020617', ink: GOLD, tier: 3 },
};

// Chat broadcasts carry a gift's name and emoji but not its id.
const GIFT_BY_EMOJI: Record<string, string> = {
  '\u{1F33C}': 'flower',
  '\u{1F369}': 'donut',
  '\u{1F370}': 'cake',
  '\u{1F35C}': 'ramen',
  '\u{1F680}': 'rocket',
  '\u{1F31F}': 'starlight',
  '\u{1F4A1}': 'fan_light',
  '\u{1F451}': 'crown',
  '\u{1F48E}': 'gem',
  '\u{1F308}': 'aurora',
  '\u{1F6E5}': 'yacht',
  '\u{1F3F0}': 'castle',
  '☄': 'meteor',
  '\u{1F6F8}': 'galaxy_ship',
  '\u{1F48D}': 'royal_crown', // what the Royal Crown used to be
  '\u{1FA90}': 'nebula_ring',
  '✨': 'eternal_scepter',
};

export interface GiftRef {
  id?: string;
  name?: string;
  icon?: string;
}

/**
 * Which built-in gift this is, by id, then English name (the Royal Crown and
 * Crown share an emoji), then emoji. Undefined for gifts added later.
 */
export function builtInGiftKey(gift: GiftRef): string | undefined {
  if (gift.id && GIFT_ART[gift.id]) return gift.id;
  const byName = gift.name?.trim().toLowerCase().replace(/\s+/g, '_');
  if (byName && GIFT_ART[byName]) return byName;
  const emoji = gift.icon?.replace(/️/g, '').trim();
  return emoji ? GIFT_BY_EMOJI[emoji] : undefined;
}
