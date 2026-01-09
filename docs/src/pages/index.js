import clsx from 'clsx';
import Link from '@docusaurus/Link';
import useDocusaurusContext from '@docusaurus/useDocusaurusContext';
import Layout from '@theme/Layout';
import Heading from '@theme/Heading';

import styles from './index.module.css';

function HomepageHeader() {
  const {siteConfig} = useDocusaurusContext();
  return (
    <header className={clsx('hero hero--primary', styles.heroBanner)}>
      <div className="container">
        <Heading as="h1" className="hero__title">
          {siteConfig.title}
        </Heading>
        <p className="hero__subtitle">{siteConfig.tagline}</p>
        <div className={styles.buttons}>
          <Link
            className="button button--secondary button--lg"
            to="/docs/intro">
            Get Started in 5 Minutes
          </Link>
        </div>
      </div>
    </header>
  );
}

function StatsSection() {
  return (
    <section className={styles.stats}>
      <div className="container">
        <div className={styles.statsGrid}>
          <div className={styles.statItem}>
            <div className={styles.statNumber}>17+</div>
            <div className={styles.statLabel}>MCPs at once</div>
          </div>
          <div className={styles.statItem}>
            <div className={styles.statNumber}>0</div>
            <div className={styles.statLabel}>Context tokens wasted</div>
          </div>
          <div className={styles.statItem}>
            <div className={styles.statNumber}>1</div>
            <div className={styles.statLabel}>Plugin to rule them all</div>
          </div>
        </div>
      </div>
    </section>
  );
}

const FeatureList = [
  {
    title: 'Scale Without Limits',
    emoji: '🚀',
    description: (
      <>
        Install 17 MCPs, or 50, or 100. SLOP MCP loads tool metadata on-demand,
        not upfront. Your context window stays clean while your capabilities explode.
      </>
    ),
  },
  {
    title: 'Skills = Zero Context Overhead',
    emoji: '⚡',
    description: (
      <>
        Create skills that call MCPs directly. No tool schemas flooding your context.
        Just invoke <code>/my-skill</code> and let SLOP handle the orchestration.
      </>
    ),
  },
  {
    title: 'OAuth That Just Works',
    emoji: '🔐',
    description: (
      <>
        Login to MCPs like Figma, Linear, or Dart with a single command.
        Auto-reconnect after auth means your tools are ready instantly.
      </>
    ),
  },
  {
    title: 'One Config, All MCPs',
    emoji: '📦',
    description: (
      <>
        Define your MCPs in KDL config files. Local, project, or user scope.
        SLOP aggregates them all into a unified interface.
      </>
    ),
  },
  {
    title: 'Search, Don\'t Scroll',
    emoji: '🔍',
    description: (
      <>
        With <code>search_tools</code>, find the exact tool you need across all
        your MCPs. Fuzzy matching included. Only matching tools hit your context.
      </>
    ),
  },
  {
    title: 'Marketplace Ready',
    emoji: '🛒',
    description: (
      <>
        Use the SLOP MCP plugin from the standardbeagle-tools marketplace.
        Pre-configured skills, instant setup, community patterns.
      </>
    ),
  },
];

function Feature({emoji, title, description}) {
  return (
    <div className={clsx('col col--4')}>
      <div className={styles.featureCard}>
        <div className={styles.featureEmoji}>{emoji}</div>
        <Heading as="h3">{title}</Heading>
        <p>{description}</p>
      </div>
    </div>
  );
}

function HomepageFeatures() {
  return (
    <section className={styles.features}>
      <div className="container">
        <div className="row">
          {FeatureList.map((props, idx) => (
            <Feature key={idx} {...props} />
          ))}
        </div>
      </div>
    </section>
  );
}

function CodeExample() {
  return (
    <section className={styles.codeSection}>
      <div className="container">
        <div className="row">
          <div className="col col--6">
            <Heading as="h2">Add MCPs in Seconds</Heading>
            <div className={styles.terminal}>
              <div><span className={styles.prompt}>$</span> slop-mcp mcp add math-mcp npx @anthropic/math-mcp</div>
              <div className={styles.output}>Added MCP: math-mcp (stdio)</div>
              <div>&nbsp;</div>
              <div><span className={styles.prompt}>$</span> slop-mcp mcp add figma -t streamable https://mcp.figma.com</div>
              <div className={styles.output}>Added MCP: figma (streamable)</div>
              <div>&nbsp;</div>
              <div><span className={styles.prompt}>$</span> slop-mcp mcp auth login figma</div>
              <div className={styles.output}>Opening browser for OAuth...</div>
              <div className={styles.output}>Authenticated! Connection re-established.</div>
            </div>
          </div>
          <div className="col col--6">
            <Heading as="h2">Search Across All MCPs</Heading>
            <div className={styles.terminal}>
              <div><span className={styles.prompt}>$</span> # In Claude Code, search tools:</div>
              <div>&nbsp;</div>
              <div className={styles.output}>search_tools query="calculate"</div>
              <div>&nbsp;</div>
              <div className={styles.output}>Found 3 tools:</div>
              <div className={styles.output}>  - math-mcp: calculate</div>
              <div className={styles.output}>  - math-mcp: evaluate_expression</div>
              <div className={styles.output}>  - stats-mcp: calculate_mean</div>
              <div>&nbsp;</div>
              <div className={styles.output}># Only these 3 schemas hit your context</div>
              <div className={styles.output}># Not all 200+ tools from 17 MCPs!</div>
            </div>
          </div>
        </div>
      </div>
    </section>
  );
}

function CTASection() {
  return (
    <section className={styles.cta}>
      <div className="container">
        <Heading as="h2">Ready to Supercharge Your AI Workflow?</Heading>
        <p>Install via pip, npm, or download the binary directly.</p>
        <div className={styles.installOptions}>
          <code>pip install slop-mcp</code>
          <code>npx slop-mcp</code>
          <code>brew install standardbeagle/tap/slop-mcp</code>
        </div>
        <div className={styles.buttons}>
          <Link
            className="button button--primary button--lg"
            to="/docs/getting-started/installation">
            Installation Guide
          </Link>
          <Link
            className="button button--secondary button--lg"
            to="/docs/examples/math-calculations">
            See Examples
          </Link>
        </div>
      </div>
    </section>
  );
}

export default function Home() {
  const {siteConfig} = useDocusaurusContext();
  return (
    <Layout
      title={`${siteConfig.title} - MCP Orchestration for Claude`}
      description="Install 17 MCPs and Claude won't even notice. SLOP MCP provides context-efficient MCP orchestration with on-demand tool loading and skills.">
      <HomepageHeader />
      <main>
        <StatsSection />
        <HomepageFeatures />
        <CodeExample />
        <CTASection />
      </main>
    </Layout>
  );
}
