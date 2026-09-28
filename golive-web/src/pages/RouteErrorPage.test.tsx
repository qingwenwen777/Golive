// @vitest-environment jsdom
import { cleanup, render, screen } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { createMemoryRouter, Outlet, RouterProvider } from 'react-router-dom';

import { isChunkLoadError } from '@/lib/chunkLoadError';
import RouteErrorPage from './RouteErrorPage';

vi.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (key: string, options?: { defaultValue?: string }) => options?.defaultValue ?? key,
  }),
}));

function Boom(): never {
  throw new Error('render crashed');
}

function renderAt(path: string) {
  const router = createMemoryRouter(
    [
      {
        element: (
          <div>
            <header>app shell</header>
            <Outlet />
          </div>
        ),
        errorElement: <RouteErrorPage />,
        children: [
          {
            errorElement: <RouteErrorPage />,
            children: [
              { path: '/', element: <p>home</p> },
              { path: '/boom', element: <Boom /> },
            ],
          },
        ],
      },
    ],
    { initialEntries: [path] },
  );
  return render(<RouterProvider router={router} />);
}

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

describe('RouteErrorPage', () => {
  it('shows a crash inside the app shell instead of a 404', () => {
    vi.spyOn(console, 'error').mockImplementation(() => {});
    renderAt('/boom');
    expect(screen.getByText('app shell')).toBeTruthy();
    expect(screen.getByRole('alert').textContent).toContain('This page hit an error');
    expect(screen.getByRole('button', { name: 'Reload' })).toBeTruthy();
    expect(screen.queryByText('404')).toBeNull();
  });

  it('still answers unknown URLs with the 404 page', () => {
    vi.spyOn(console, 'error').mockImplementation(() => {});
    renderAt('/nope');
    expect(screen.getByText('notFound.title')).toBeTruthy();
  });

  it('recognises failed lazy chunk loads from the major browsers', () => {
    expect(
      isChunkLoadError(new TypeError('Failed to fetch dynamically imported module: /assets/x.js')),
    ).toBe(true);
    expect(isChunkLoadError(new TypeError('error loading dynamically imported module'))).toBe(true);
    expect(isChunkLoadError(new TypeError('Importing a module script failed.'))).toBe(true);
    expect(isChunkLoadError(new Error('render crashed'))).toBe(false);
  });
});
