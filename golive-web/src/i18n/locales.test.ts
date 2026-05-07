import { describe, expect, it } from 'vitest';

import jaCommon from './locales/ja-JP/common.json';
import jaPages from './locales/ja-JP/pages.json';
import zhCommon from './locales/zh-CN/common.json';
import zhPages from './locales/zh-CN/pages.json';

const resources = {
  'ja-JP/common': jaCommon,
  'ja-JP/pages': jaPages,
  'zh-CN/common': zhCommon,
  'zh-CN/pages': zhPages,
};

describe('locale resources', () => {
  it('does not contain question-mark replacement text in Chinese or Japanese resources', () => {
    const suspicious: string[] = [];

    for (const [resourceName, resource] of Object.entries(resources)) {
      collectQuestionMarkReplacements(resource, resourceName, suspicious);
    }

    expect(suspicious).toEqual([]);
  });
});

function collectQuestionMarkReplacements(value: unknown, path: string, results: string[]) {
  if (typeof value === 'string') {
    if (/\?{2,}/.test(value) || value === '?') {
      results.push(`${path}: ${value}`);
    }
    return;
  }

  if (!value || typeof value !== 'object') return;

  for (const [key, child] of Object.entries(value)) {
    collectQuestionMarkReplacements(child, `${path}.${key}`, results);
  }
}
