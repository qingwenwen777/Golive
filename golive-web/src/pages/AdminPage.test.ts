import { describe, expect, it } from 'vitest';

import { getModuleFromPath, isAdminPath, reportActionsForTarget } from './AdminPage';

describe('admin route parsing', () => {
  it('does not treat non-admin routes as admin modules during route transitions', () => {
    expect(isAdminPath('/')).toBe(false);
    expect(getModuleFromPath('/')).toBeUndefined();
    expect(getModuleFromPath('/search')).toBeUndefined();
  });

  it('maps admin paths to modules', () => {
    expect(isAdminPath('/admin/content')).toBe(true);
    expect(getModuleFromPath('/admin')).toBe('dashboard');
    expect(getModuleFromPath('/admin/applications')).toBe('creators');
    expect(getModuleFromPath('/admin/content')).toBe('content');
  });
});

describe('report actions', () => {
  it('offers penalties only for reports whose target the server verified', () => {
    expect(reportActionsForTarget('post')).toContain('ban_user');
    expect(reportActionsForTarget('room', true)).toContain('force_end_live');
    for (const targetType of ['post', 'room', 'channel', 'danmu']) {
      expect(reportActionsForTarget(targetType, false)).toEqual(['dismiss']);
    }
  });
});
