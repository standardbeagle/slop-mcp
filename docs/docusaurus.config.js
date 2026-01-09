// @ts-check
import {themes as prismThemes} from 'prism-react-renderer';

/** @type {import('@docusaurus/types').Config} */
const config = {
  title: 'SLOP MCP',
  tagline: 'Install 17 MCPs. Claude won\'t even notice.',
  favicon: 'img/favicon.ico',

  url: 'https://standardbeagle.github.io',
  baseUrl: '/slop-mcp/',

  organizationName: 'standardbeagle',
  projectName: 'slop-mcp',

  onBrokenLinks: 'throw',
  onBrokenMarkdownLinks: 'warn',

  i18n: {
    defaultLocale: 'en',
    locales: ['en'],
  },

  presets: [
    [
      'classic',
      /** @type {import('@docusaurus/preset-classic').Options} */
      ({
        docs: {
          sidebarPath: './sidebars.js',
          editUrl: 'https://github.com/standardbeagle/slop-mcp/tree/main/docs/',
        },
        blog: false,
        theme: {
          customCss: './src/css/custom.css',
        },
      }),
    ],
  ],

  themeConfig:
    /** @type {import('@docusaurus/preset-classic').ThemeConfig} */
    ({
      image: 'img/slop-mcp-social-card.png',
      navbar: {
        title: 'SLOP MCP',
        logo: {
          alt: 'SLOP MCP Logo',
          src: 'img/logo.svg',
        },
        items: [
          {
            type: 'docSidebar',
            sidebarId: 'docsSidebar',
            position: 'left',
            label: 'Docs',
          },
          {
            href: 'https://github.com/standardbeagle/slop-mcp',
            label: 'GitHub',
            position: 'right',
          },
        ],
      },
      footer: {
        style: 'dark',
        links: [
          {
            title: 'Docs',
            items: [
              {
                label: 'Getting Started',
                to: '/docs/intro',
              },
              {
                label: 'Examples',
                to: '/docs/examples/math-calculations',
              },
            ],
          },
          {
            title: 'Community',
            items: [
              {
                label: 'GitHub Discussions',
                href: 'https://github.com/standardbeagle/slop-mcp/discussions',
              },
              {
                label: 'Issues',
                href: 'https://github.com/standardbeagle/slop-mcp/issues',
              },
            ],
          },
          {
            title: 'More',
            items: [
              {
                label: 'GitHub',
                href: 'https://github.com/standardbeagle/slop-mcp',
              },
              {
                label: 'PyPI',
                href: 'https://pypi.org/project/slop-mcp/',
              },
              {
                label: 'npm',
                href: 'https://www.npmjs.com/package/slop-mcp',
              },
            ],
          },
        ],
        copyright: `Copyright ${new Date().getFullYear()} Standard Beagle. Built with Docusaurus.`,
      },
      prism: {
        theme: prismThemes.github,
        darkTheme: prismThemes.dracula,
        additionalLanguages: ['bash', 'json', 'toml'],
      },
    }),
};

export default config;
