// @vitest-environment jsdom
import { cleanup, render, screen, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import { GoogleIdentityButton } from './googleIdentity';

describe('GoogleIdentityButton', () => {
  beforeEach(() => {
    vi.stubEnv('VITE_GOOGLE_CLIENT_ID', 'test-client-id');
    window.google = {
      accounts: {
        id: {
          initialize: vi.fn(),
          renderButton: vi.fn((element: HTMLElement) => {
            element.appendChild(document.createElement('iframe'));
          }),
        },
      },
    };
  });

  afterEach(() => {
    cleanup();
    vi.unstubAllEnvs();
    delete window.google;
  });

  it('keeps the Google iframe constrained to the button shell when ready', async () => {
    render(
      <GoogleIdentityButton
        fallbackLabel="Continue with Google"
        onCredential={vi.fn()}
      />,
    );

    await waitFor(() => expect(screen.getByLabelText('Continue with Google')).toBeTruthy());

    const shell = document.querySelector('.gl-google-button-shell');
    expect(shell).toBeTruthy();
    expect(shell?.className).toContain('gl-google-button-shell');
    expect(shell?.className).toContain('is-ready');
    expect(document.querySelector('.gl-google-button-shellis-ready')).toBeNull();
  });
});
