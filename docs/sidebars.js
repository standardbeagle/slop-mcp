/** @type {import('@docusaurus/plugin-content-docs').SidebarsConfig} */
const sidebars = {
  docsSidebar: [
    'intro',
    {
      type: 'category',
      label: 'Getting Started',
      items: [
        'getting-started/installation',
        'getting-started/quick-start',
        'getting-started/configuration',
      ],
    },
    {
      type: 'category',
      label: 'Core Concepts',
      items: [
        'concepts/why-slop-mcp',
        'concepts/context-efficiency',
        'concepts/skills',
        'concepts/oauth',
        'concepts/monitoring',
        'concepts/customization',
      ],
    },
    {
      type: 'category',
      label: 'Examples',
      items: [
        'examples/math-calculations',
        'examples/mcp-templates',
        'examples/multi-mcp-orchestration',
      ],
    },
    {
      type: 'category',
      label: 'Reference',
      items: [
        'reference/cli',
        'reference/tools',
        'reference/slop-language',
        'reference/kdl-config',
      ],
    },
  ],
};

export default sidebars;
