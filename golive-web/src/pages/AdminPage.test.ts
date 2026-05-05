import { describe, expect, it } from 'vitest';

import { getModuleFromPath, isAdminPath } from './AdminPage';

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
