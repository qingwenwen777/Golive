// @vitest-environment jsdom
import { type ReactNode } from 'react';
import { AxiosError, AxiosHeaders } from 'axios';
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, beforeAll, beforeEach, describe, expect, it, vi } from 'vitest';

import i18n from '@/i18n';
import { AuthPanel } from './AuthPanel';

const api = vi.hoisted(() => {
  const mutation = () => ({ mutate: vi.fn(), reset: vi.fn(), isPending: false });
  return {
    fetchCaptcha: vi.fn(),
    login: mutation(),
    sendEmailCode: mutation(),
    register: mutation(),
    resetPassword: mutation(),
    googleLogin: mutation(),
    googleRegister: mutation(),
    googleLinkExisting: mutation(),
  };
});

vi.mock('@/api/auth', () => ({
  fetchCaptcha: api.fetchCaptcha,
  useLoginMutation: () => api.login,
  useSendEmailCodeMutation: () => api.sendEmailCode,
  useRegisterMutation: () => api.register,
  useResetPasswordMutation: () => api.resetPassword,
  useGoogleLoginMutation: () => api.googleLogin,
  useGoogleRegisterMutation: () => api.googleRegister,
  useGoogleLinkExistingMutation: () => api.googleLinkExisting,
}));

vi.mock('@/lib/googleIdentity', () => ({
  GoogleIdentityButton: () => null,
  decodeGoogleCredentialEmail: () => '',
  isGoogleConfigured: () => false,
}));

vi.mock('@/components/ui/dialog', () => ({
  Dialog: ({ open, children }: { open: boolean; children: ReactNode }) =>
    open ? <div>{children}</div> : null,
  DialogContent: ({ children }: { children: ReactNode }) => <div>{children}</div>,
  DialogTitle: ({ children }: { children: ReactNode }) => <h2>{children}</h2>,
  DialogDescription: ({ children }: { children: ReactNode }) => <p>{children}</p>,
}));

const USE_EMAIL_CODE = "Can't read the image? Get a code by email instead";

let captchaCount = 0;

async function renderSignIn() {
  render(<AuthPanel />);
  await screen.findByAltText('Captcha image');
  fireEvent.change(screen.getByLabelText('Username'), { target: { value: 'streamer' } });
  fireEvent.change(screen.getByLabelText('Password'), { target: { value: 'secret123' } });
}

function switchToEmailCode() {
  fireEvent.click(screen.getByRole('button', { name: USE_EMAIL_CODE }));
  return screen.getByRole('textbox', { name: 'Email' });
}

function submit() {
  fireEvent.click(screen.getByRole('button', { name: 'Sign in' }));
}

describe('AuthPanel sign-in', () => {
  beforeAll(async () => {
    await vi.waitFor(() => expect(i18n.isInitialized).toBe(true));
    await i18n.changeLanguage('en-US');
  });

  beforeEach(() => {
    for (const mutation of [api.login, api.sendEmailCode, api.register]) {
      mutation.mutate.mockReset();
    }
    captchaCount = 0;
    api.fetchCaptcha.mockReset();
    api.fetchCaptcha.mockImplementation(async () => {
      captchaCount += 1;
      return {
        id: `captcha-${captchaCount}`,
        image: 'data:image/svg+xml;base64,PHN2Zy8+',
        expiresIn: 300,
      };
    });
  });

  afterEach(() => {
    cleanup();
  });

  it('describes the captcha to screen readers', async () => {
    await renderSignIn();

    // The refresh button sits outside the label, so it no longer leaks into
    // the input's name.
    expect(
      screen.getByRole('textbox', {
        name: 'Captcha',
        description: 'Type the 6 characters in the image',
      }),
    ).toBeTruthy();
    expect(screen.getByRole('button', { name: 'Get a new captcha image' })).toBeTruthy();
    expect(screen.getByAltText('Captcha image')).toBeTruthy();
  });

  it('signs in with an emailed code instead of the captcha', async () => {
    await renderSignIn();

    const email = switchToEmailCode();
    expect(document.activeElement).toBe(email);
    expect(screen.queryByRole('textbox', { name: 'Captcha' })).toBeNull();
    expect(
      screen.getByRole('textbox', {
        name: 'Email',
        description: "We'll send a code to the email on your account.",
      }),
    ).toBe(email);
    fireEvent.change(email, { target: { value: 'streamer@example.com' } });

    fireEvent.click(screen.getByRole('button', { name: 'Send code' }));
    expect(api.sendEmailCode.mutate).toHaveBeenCalledWith(
      { purpose: 'login', username: 'streamer', email: 'streamer@example.com' },
      expect.anything(),
    );

    fireEvent.change(screen.getByRole('textbox', { name: 'Email code' }), {
      target: { value: ' 123456 ' },
    });
    submit();
    expect(api.login.mutate).toHaveBeenCalledTimes(1);
    expect(api.login.mutate.mock.calls[0][0]).toStrictEqual({
      username: 'streamer',
      password: 'secret123',
      email: 'streamer@example.com',
      emailCode: '123456',
    });
  });

  it('asks for a new code after a wrong password, since the server used it up', async () => {
    api.login.mutate.mockImplementation(
      (_payload: unknown, callbacks: { onError: (err: Error) => void }) => {
        callbacks.onError(
          new AxiosError('Unauthorized', 'ERR_BAD_REQUEST', undefined, undefined, {
            status: 401,
            statusText: 'Unauthorized',
            data: { message: 'Invalid username or password' },
            headers: {},
            config: { headers: new AxiosHeaders() },
          }),
        );
      },
    );
    await renderSignIn();
    fireEvent.change(switchToEmailCode(), { target: { value: 'streamer@example.com' } });
    const code = screen.getByRole('textbox', { name: 'Email code' }) as HTMLInputElement;
    fireEvent.change(code, { target: { value: '123456' } });

    submit();

    expect(screen.getByRole('alert').textContent).toBe(
      'Invalid username or password. Each code works only once, so send a new one to try again.',
    );
    expect(code.value).toBe('');
    expect(api.fetchCaptcha).toHaveBeenCalledTimes(1);
  });

  it('goes back to the image captcha with a fresh image', async () => {
    await renderSignIn();
    switchToEmailCode();

    fireEvent.click(screen.getByRole('button', { name: 'Use the image captcha instead' }));

    const captcha = screen.getByRole('textbox', { name: 'Captcha' });
    expect(document.activeElement).toBe(captcha);
    await waitFor(() => expect(api.fetchCaptcha).toHaveBeenCalledTimes(2));
    await waitFor(() =>
      expect(
        (screen.getByRole('button', { name: 'Get a new captcha image' }) as HTMLButtonElement)
          .disabled,
      ).toBe(false),
    );
    fireEvent.change(captcha, { target: { value: 'abc234' } });
    submit();
    expect(api.login.mutate.mock.calls[0][0]).toStrictEqual({
      username: 'streamer',
      password: 'secret123',
      captchaId: 'captcha-2',
      captchaCode: 'ABC234',
    });
  });

  it('keeps the captcha for registration', async () => {
    await renderSignIn();

    fireEvent.click(screen.getByRole('tab', { name: 'Register' }));

    await waitFor(() => expect(api.fetchCaptcha).toHaveBeenCalledTimes(2));
    expect(screen.getByRole('textbox', { name: 'Captcha' })).toBeTruthy();
    expect(screen.queryByRole('button', { name: USE_EMAIL_CODE })).toBeNull();
  });
});
