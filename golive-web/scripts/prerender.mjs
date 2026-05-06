import { mkdir, readFile, writeFile } from 'node:fs/promises';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const here = path.dirname(fileURLToPath(import.meta.url));
const projectRoot = path.resolve(here, '..');
const distDir = path.join(projectRoot, 'dist');
const templatePath = path.join(distDir, 'index.html');
const defaultSiteOrigin = 'http://154.36.185.85';
const siteOrigin = normalizeOrigin(
  process.env.VITE_PUBLIC_ORIGIN ?? process.env.SITE_ORIGIN ?? defaultSiteOrigin,
);

const routes = [
  {
    path: '/',
    title: 'GoLive - Live streaming community',
    description:
      'Discover live rooms, replays, creator channels, appointments, chat, and community moments on GoLive.',
    content: renderHome(),
  },
  {
    path: '/search',
    title: 'Search GoLive live rooms and creators',
    description:
      'Search GoLive for live rooms, creators, replays, appointments, and community posts.',
    content: renderSearch(),
  },
  {
    path: '/privacy',
    title: 'Privacy Policy - GoLive',
    description:
      'Learn how GoLive collects, uses, shares, and protects account, security, and service information.',
    content: renderLegal('Privacy Policy', [
      [
        'Information we collect',
        'GoLive collects account details, profile information, login method, invite code usage, and security records needed to provide and protect the service.',
      ],
      [
        'How we use information',
        'We use information to provide login, live rooms, chat, subscriptions, wallet features, creator tools, moderation, safety, and service reliability.',
      ],
      [
        'Sharing and retention',
        'We do not sell personal information. Data is shared only with service components, infrastructure providers, or legal and safety processes when needed.',
      ],
    ]),
  },
  {
    path: '/terms',
    title: 'Terms of Service - GoLive',
    description:
      'Read the GoLive terms for using live streaming, chat, creator tools, coins, and community features.',
    content: renderLegal('Terms of Service', [
      [
        'Using GoLive',
        'GoLive provides live streaming, chat, creator, wallet, and community features. You are responsible for account activity and password security.',
      ],
      [
        'Community and content',
        'Do not upload, stream, or send illegal, abusive, infringing, deceptive, or harmful content.',
      ],
      [
        'Coins and creator tools',
        'Coins, gifts, live permissions, and creator tools may be changed, limited, or reviewed to prevent fraud, abuse, or operational risk.',
      ],
    ]),
  },
];

const template = await readFile(templatePath, 'utf8');

for (const route of routes) {
  const html = renderRoute(template, route);
  const outputPath =
    route.path === '/'
      ? path.join(distDir, 'index.html')
      : path.join(distDir, route.path.slice(1), 'index.html');

  await mkdir(path.dirname(outputPath), { recursive: true });
  await writeFile(outputPath, html);
}

await writeFile(path.join(distDir, 'robots.txt'), renderRobots());
await writeFile(path.join(distDir, 'sitemap.xml'), renderSitemap());

console.log(`prerendered ${routes.length} static route(s)`);

function renderRoute(templateHtml, route) {
  let html = stripManagedHead(templateHtml);
  const title = escapeHtml(route.title);
  const description = escapeAttr(route.description);
  const url = canonicalUrl(route.path);

  html = html.replace(/\s*<meta\s+name=["']description["'][^>]*>\s*/gi, '\n');
  html = html.replace(/<html([^>]*)\sdata-prerendered=["']true["']([^>]*)>/, '<html$1$2>');
  html = html.replace(/<html([^>]*)>/, '<html$1 data-prerendered="true">');
  html = html.replace(/<title>.*?<\/title>/s, `<title>${title}</title>`);
  html = html.replace(
    '</head>',
    `    <!-- golive:ssg-head:start -->
    <meta name="description" content="${description}" />
    <meta name="robots" content="index,follow" />
    <meta property="og:type" content="website" />
    <meta property="og:site_name" content="GoLive" />
    <meta property="og:title" content="${title}" />
    <meta property="og:description" content="${description}" />
    <meta name="twitter:card" content="summary" />
    <meta name="twitter:title" content="${title}" />
    <meta name="twitter:description" content="${description}" />
${url ? `    <link rel="canonical" href="${escapeAttr(url)}" />\n` : ''}    <script type="application/ld+json">${renderJsonLd(route, url)}</script>
    <!-- golive:ssg-head:end -->
  </head>`,
  );

  const root = `<div id="root" data-prerendered="true">
      <!-- golive:ssg-body:start -->
${route.content}
      <!-- golive:ssg-body:end -->
    </div>`;

  return html.replace(
    /<div id="root"><\/div>|<div id="root"[^>]*>\s*<!-- golive:ssg-body:start -->[\s\S]*?<!-- golive:ssg-body:end -->\s*<\/div>/,
    root,
  );
}

function renderHome() {
  return renderShell(`
      <section class="gl-prerender-hero">
        <p class="gl-prerender-kicker">Live now, replay later</p>
        <h1>GoLive</h1>
        <p>Find live rooms, upcoming creator appointments, replays, and community updates before the app finishes loading.</p>
        <div class="gl-prerender-actions">
          <a href="/search">Search live rooms</a>
          <a href="/subscriptions">View subscriptions</a>
        </div>
      </section>
      <section class="gl-prerender-section" aria-labelledby="ssg-live-heading">
        <div class="gl-prerender-section-head">
          <h2 id="ssg-live-heading">Recommended live</h2>
          <span>Fresh rooms, creators, and categories</span>
        </div>
        <div class="gl-prerender-grid">
          ${renderCard('Live rooms', 'Browse streams by category, creator, or search keyword.', '/search')}
          ${renderCard('Upcoming appointments', 'Reserve upcoming broadcasts and return when they go live.', '/')}
          ${renderCard('Hot replays', 'Catch recent live moments after the stream has ended.', '/search')}
        </div>
      </section>
      <section class="gl-prerender-section" aria-labelledby="ssg-community-heading">
        <div class="gl-prerender-section-head">
          <h2 id="ssg-community-heading">Creator community</h2>
          <span>Channels, posts, chat, gifts, and fan groups</span>
        </div>
        <div class="gl-prerender-grid">
          ${renderCard('Creator channels', 'Follow creators and keep track of new live sessions.', '/search')}
          ${renderCard('Chat and messages', 'Join real-time room chat and private community threads.', '/messages')}
          ${renderCard('Coins and gifts', 'Support creators with wallet and gift features.', '/coins')}
        </div>
      </section>`);
}

function renderSearch() {
  return renderShell(`
      <section class="gl-prerender-hero is-compact">
        <p class="gl-prerender-kicker">Search</p>
        <h1>Search GoLive</h1>
        <p>Look for creators, live streams, replays, appointments, and posts.</p>
        <form class="gl-prerender-search" action="/search" method="get">
          <input name="q" type="search" placeholder="Search live rooms and creators" />
          <button type="submit">Search</button>
        </form>
      </section>
      <section class="gl-prerender-section">
        <div class="gl-prerender-section-head">
          <h2>Explore by type</h2>
          <span>Results load instantly once the app connects</span>
        </div>
        <div class="gl-prerender-grid">
          ${renderCard('Creators', 'Find channels and recent creator activity.', '/search')}
          ${renderCard('Live', 'Open rooms that are streaming right now.', '/search')}
          ${renderCard('Replays', 'Discover past broadcasts and highlights.', '/search')}
        </div>
      </section>`);
}

function renderLegal(title, sections) {
  return renderShell(`
      <section class="gl-prerender-hero is-compact">
        <p class="gl-prerender-kicker">GoLive</p>
        <h1>${escapeHtml(title)}</h1>
        <p>Core policy information is available in the initial HTML and the React app provides the full localized page after load.</p>
      </section>
      <section class="gl-prerender-section">
        <div class="gl-prerender-legal">
          ${sections
            .map(
              ([heading, body]) => `<article>
            <h2>${escapeHtml(heading)}</h2>
            <p>${escapeHtml(body)}</p>
          </article>`,
            )
            .join('\n')}
        </div>
      </section>`);
}

function renderShell(main) {
  return `      <div class="gl-prerender-shell">
        <header class="gl-prerender-topbar">
          <a class="gl-prerender-brand" href="/">
            <img src="/golive-logo.svg" alt="" />
            <span>GoLive</span>
          </a>
          <nav aria-label="Primary">
            <a href="/">Home</a>
            <a href="/search">Search</a>
            <a href="/coins">Coins</a>
          </nav>
        </header>
        <div class="gl-prerender-layout">
          <aside class="gl-prerender-sidebar" aria-label="Sections">
            <a href="/">Home</a>
            <a href="/subscriptions">Subscriptions</a>
            <a href="/history">History</a>
            <a href="/watch-later">Watch later</a>
          </aside>
          <main class="gl-prerender-main">
${main}
          </main>
        </div>
      </div>`;
}

function renderCard(title, body, href) {
  return `<article class="gl-prerender-card">
            <a href="${escapeAttr(href)}">
              <span>${escapeHtml(title.slice(0, 1))}</span>
              <strong>${escapeHtml(title)}</strong>
              <p>${escapeHtml(body)}</p>
            </a>
          </article>`;
}

function stripManagedHead(html) {
  return html.replace(
    /\s*<!-- golive:ssg-head:start -->[\s\S]*?<!-- golive:ssg-head:end -->/g,
    '',
  );
}

function renderJsonLd(route, url) {
  const data = {
    '@context': 'https://schema.org',
    '@type': route.path === '/' ? 'WebSite' : 'WebPage',
    name: route.title,
    description: route.description,
    ...(url ? { url } : {}),
    ...(route.path === '/' && url
      ? {
          potentialAction: {
            '@type': 'SearchAction',
            target: `${siteOrigin}/search?q={search_term_string}`,
            'query-input': 'required name=search_term_string',
          },
        }
      : {}),
  };

  return JSON.stringify(data).replace(/</g, '\\u003c');
}

function renderRobots() {
  const sitemap = siteOrigin ? `\nSitemap: ${siteOrigin}/sitemap.xml` : '';
  return `User-agent: *
Allow: /${sitemap}
`;
}

function renderSitemap() {
  const urls = routes
    .map(
      (route) => `  <url>
    <loc>${escapeHtml(siteOrigin + route.path)}</loc>
  </url>`,
    )
    .join('\n');

  return `<?xml version="1.0" encoding="UTF-8"?>
<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
${urls}
</urlset>
`;
}

function canonicalUrl(routePath) {
  if (!siteOrigin) return '';
  return siteOrigin + routePath;
}

function normalizeOrigin(value) {
  const trimmed = value.trim();
  if (!trimmed) return '';
  return trimmed.replace(/\/+$/, '');
}

function escapeHtml(value) {
  return String(value)
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;');
}

function escapeAttr(value) {
  return escapeHtml(value).replace(/"/g, '&quot;');
}
