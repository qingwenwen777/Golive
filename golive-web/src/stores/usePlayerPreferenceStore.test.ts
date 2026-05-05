// @vitest-environment jsdom
import { beforeEach, describe, expect, it } from 'vitest';

import { usePlayerPreferenceStore } from './usePlayerPreferenceStore';

describe('usePlayerPreferenceStore', () => {
  beforeEach(() => {
    window.localStorage.clear();
    usePlayerPreferenceStore.setState({ muted: false, volume: 1 });
  });

  it('updates audio settings independently', () => {
    usePlayerPreferenceStore.getState().setAudio({ muted: true });
    expect(usePlayerPreferenceStore.getState()).toMatchObject({ muted: true, volume: 1 });

    usePlayerPreferenceStore.getState().setAudio({ volume: 0.35 });
    expect(usePlayerPreferenceStore.getState()).toMatchObject({ muted: true, volume: 0.35 });
  });

  it('clamps volume into the playable range', () => {
    usePlayerPreferenceStore.getState().setAudio({ volume: -1 });
    expect(usePlayerPreferenceStore.getState().volume).toBe(0);

    usePlayerPreferenceStore.getState().setAudio({ volume: 2 });
    expect(usePlayerPreferenceStore.getState().volume).toBe(1);

    usePlayerPreferenceStore.getState().setAudio({ volume: Number.NaN });
    expect(usePlayerPreferenceStore.getState().volume).toBe(1);
  });
});
