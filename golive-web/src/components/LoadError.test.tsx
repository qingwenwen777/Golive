// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { AxiosError, AxiosHeaders } from 'axios';
import { afterEach, describe, expect, it, vi } from 'vitest';

import { isNotFoundError } from '@/lib/httpError';
import { LoadError } from './LoadError';

vi.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (key: string, options?: { defaultValue?: string }) => options?.defaultValue ?? key,
  }),
}));

function httpFailure(status: number) {
  return new AxiosError('Request failed', 'ERR_BAD_RESPONSE', undefined, undefined, {
    status,
    statusText: '',
    data: {},
    headers: {},
    config: { headers: new AxiosHeaders() },
  });
}

const offline = new AxiosError('Network Error', 'ERR_NETWORK');

afterEach(cleanup);

describe('isNotFoundError', () => {
  it('is true only when the server answered 404', () => {
    expect(isNotFoundError(httpFailure(404))).toBe(true);
    expect(isNotFoundError(httpFailure(500))).toBe(false);
    expect(isNotFoundError(offline)).toBe(false);
    expect(isNotFoundError(new Error('boom'))).toBe(false);
  });
});

describe('LoadError', () => {
  it('says the section failed and retries on click', () => {
    const onRetry = vi.fn();
    render(<LoadError error={offline} onRetry={onRetry} />);
    expect(screen.getByText("Couldn't load this section")).toBeTruthy();
    expect(screen.getByText('Check your connection and try again.')).toBeTruthy();
    fireEvent.click(screen.getByRole('button', { name: 'Retry' }));
    expect(onRetry).toHaveBeenCalledTimes(1);
  });

  it('blames the server, not the connection, when the server answered', () => {
    render(<LoadError error={httpFailure(503)} onRetry={() => {}} />);
    expect(
      screen.getByText('Something went wrong on our end. Try again in a moment.'),
    ).toBeTruthy();
  });

  it('ignores clicks while a retry is in flight but keeps the button focusable', () => {
    const onRetry = vi.fn();
    render(<LoadError onRetry={onRetry} retrying />);
    const button = screen.getByRole('button', { name: 'Retrying…' });
    expect(button.getAttribute('aria-disabled')).toBe('true');
    expect(button.hasAttribute('disabled')).toBe(false);
    fireEvent.click(button);
    expect(onRetry).not.toHaveBeenCalled();
  });

  it('announces a whole-page failure', () => {
    render(<LoadError variant="page" title="Couldn't load this live room" onRetry={() => {}} />);
    expect(screen.getByRole('alert').textContent).toContain("Couldn't load this live room");
  });
});
