import clsx from 'clsx';
import Link from '@docusaurus/Link';
import useDocusaurusContext from '@docusaurus/useDocusaurusContext';
import Layout from '@theme/Layout';
import Heading from '@theme/Heading';
import { useEffect, useState } from 'react';

import styles from './index.module.css';

function HomepageHeader() {
  return (
    <header className={styles.hero}>
      <div className={styles.heroBackground}>
        <div className={styles.orb1}></div>
        <div className={styles.orb2}></div>
        <div className={styles.orb3}></div>
        <div className={styles.gridOverlay}></div>
      </div>
      <div className={styles.heroContent}>
        <div className={styles.badge}>
          <span className={styles.badgeDot}></span>
          MCP Orchestration Layer
        </div>
        <h1 className={styles.heroTitle}>
          <span className={styles.heroTitleLine}>Install 17 MCPs.</span>
          <span className={styles.heroTitleAccent}>Claude won't even notice.</span>
        </h1>
        <p className={styles.heroSubtitle}>
          Connect unlimited MCP servers to Claude with zero context overhead.
          On-demand tool loading. Skills with no schema bloat. One plugin to orchestrate them all.
        </p>
        <div className={styles.heroCTA}>
          <Link className={styles.primaryButton} to="/docs/intro">
            Get Started
            <span className={styles.buttonArrow}>→</span>
          </Link>
          <Link className={styles.secondaryButton} to="/docs/examples/math-calculations">
            See Examples
          </Link>
        </div>
        <div className={styles.installHint}>
          <code>pip install slop-mcp</code>
          <span className={styles.separator}>or</span>
          <code>npx slop-mcp</code>
        </div>
      </div>
    </header>
  );
}

function StatsSection() {
  const stats = [
    { number: '17+', label: 'MCPs simultaneously', sublabel: 'with room for more' },
    { number: '0', label: 'Context tokens wasted', sublabel: 'lazy loading FTW' },
    { number: '98%', label: 'Context savings', sublabel: 'vs traditional setup' },
  ];

  return (
    <section className={styles.stats}>
      <div className={styles.statsContainer}>
        {stats.map((stat, idx) => (
          <div key={idx} className={styles.statCard}>
            <div className={styles.statNumber}>{stat.number}</div>
            <div className={styles.statLabel}>{stat.label}</div>
            <div className={styles.statSublabel}>{stat.sublabel}</div>
          </div>
        ))}
      </div>
    </section>
  );
}

function TerminalDemo() {
  return (
    <section className={styles.terminalSection}>
      <div className={styles.terminalContainer}>
        <div className={styles.terminalHeader}>
          <span className={styles.terminalTitle}>How it works</span>
        </div>
        <div className={styles.terminalGrid}>
          <div className={styles.terminalCard}>
            <div className={styles.terminalWindow}>
              <div className={styles.terminalTitleBar}>
                <div className={styles.trafficLights}>
                  <span className={styles.trafficRed}></span>
                  <span className={styles.trafficYellow}></span>
                  <span className={styles.trafficGreen}></span>
                </div>
                <span className={styles.terminalName}>terminal — slop-mcp</span>
              </div>
              <div className={styles.terminalBody}>
                <div className={styles.terminalLine}>
                  <span className={styles.prompt}>$</span>
                  <span className={styles.command}>slop-mcp mcp add figma -t streamable https://mcp.figma.com</span>
                </div>
                <div className={styles.terminalLine}>
                  <span className={styles.outputSuccess}>✓ Added MCP: figma (streamable)</span>
                </div>
                <div className={styles.terminalLine}>&nbsp;</div>
                <div className={styles.terminalLine}>
                  <span className={styles.prompt}>$</span>
                  <span className={styles.command}>slop-mcp mcp auth login figma</span>
                </div>
                <div className={styles.terminalLine}>
                  <span className={styles.outputDim}>Opening browser for OAuth...</span>
                </div>
                <div className={styles.terminalLine}>
                  <span className={styles.outputSuccess}>✓ Authenticated! Connection re-established.</span>
                </div>
              </div>
            </div>
            <div className={styles.terminalCaption}>
              <span className={styles.captionNumber}>01</span>
              Add MCPs with one command. OAuth just works.
            </div>
          </div>

          <div className={styles.terminalCard}>
            <div className={styles.terminalWindow}>
              <div className={styles.terminalTitleBar}>
                <div className={styles.trafficLights}>
                  <span className={styles.trafficRed}></span>
                  <span className={styles.trafficYellow}></span>
                  <span className={styles.trafficGreen}></span>
                </div>
                <span className={styles.terminalName}>claude code</span>
              </div>
              <div className={styles.terminalBody}>
                <div className={styles.terminalLine}>
                  <span className={styles.outputDim}>{'>'}</span>
                  <span className={styles.command}> search_tools query="design file"</span>
                </div>
                <div className={styles.terminalLine}>&nbsp;</div>
                <div className={styles.terminalLine}>
                  <span className={styles.output}>Found 2 tools:</span>
                </div>
                <div className={styles.terminalLine}>
                  <span className={styles.outputAccent}>  figma:</span>
                  <span className={styles.output}>get_file, export_assets</span>
                </div>
                <div className={styles.terminalLine}>&nbsp;</div>
                <div className={styles.terminalLine}>
                  <span className={styles.outputDim}># Only 2 schemas loaded</span>
                </div>
                <div className={styles.terminalLine}>
                  <span className={styles.outputDim}># Not 200+ from all your MCPs</span>
                </div>
              </div>
            </div>
            <div className={styles.terminalCaption}>
              <span className={styles.captionNumber}>02</span>
              Search loads only what you need. Context stays clean.
            </div>
          </div>
        </div>
      </div>
    </section>
  );
}

const features = [
  {
    icon: '∞',
    title: 'Unlimited Scale',
    description: 'Connect 17 MCPs or 170. Your context overhead stays constant at ~400 tokens.',
  },
  {
    icon: '⚡',
    title: 'Lazy Loading',
    description: 'Tool schemas load on-demand when you search. Not upfront. Not ever, if you don\'t need them.',
  },
  {
    icon: '→',
    title: 'Skills',
    description: 'Create slash commands that invoke MCPs directly. Zero schema overhead. Just /do-the-thing.',
  },
  {
    icon: '⟳',
    title: 'Auto-Reconnect',
    description: 'OAuth flows reconnect automatically. Auth once, use forever. No manual re-registration.',
  },
  {
    icon: '◈',
    title: 'Unified Interface',
    description: 'One execute_tool pattern for every MCP. Chain database → math → slack in one workflow.',
  },
  {
    icon: '◫',
    title: 'KDL Config',
    description: 'Clean, readable config files. Project, user, or local scope. Version control friendly.',
  },
];

function FeaturesSection() {
  return (
    <section className={styles.features}>
      <div className={styles.featuresContainer}>
        <div className={styles.featuresHeader}>
          <h2 className={styles.featuresTitle}>Built for power users</h2>
          <p className={styles.featuresSubtitle}>
            Every decision optimized for minimal context, maximum capability.
          </p>
        </div>
        <div className={styles.featuresGrid}>
          {features.map((feature, idx) => (
            <div key={idx} className={styles.featureCard}>
              <div className={styles.featureIcon}>{feature.icon}</div>
              <h3 className={styles.featureTitle}>{feature.title}</h3>
              <p className={styles.featureDescription}>{feature.description}</p>
            </div>
          ))}
        </div>
      </div>
    </section>
  );
}

function CTASection() {
  return (
    <section className={styles.cta}>
      <div className={styles.ctaContainer}>
        <div className={styles.ctaContent}>
          <h2 className={styles.ctaTitle}>
            Stop burning context.<br/>Start orchestrating.
          </h2>
          <p className={styles.ctaSubtitle}>
            Available now via pip, npm, or direct download.
          </p>
          <div className={styles.ctaButtons}>
            <Link className={styles.ctaPrimary} to="/docs/getting-started/installation">
              Read the Docs
            </Link>
            <a
              className={styles.ctaSecondary}
              href="https://github.com/standardbeagle/slop-mcp"
              target="_blank"
              rel="noopener noreferrer"
            >
              View on GitHub
            </a>
          </div>
        </div>
        <div className={styles.ctaVisual}>
          <div className={styles.ctaCode}>
            <code>pip install slop-mcp</code>
          </div>
        </div>
      </div>
    </section>
  );
}

export default function Home() {
  const {siteConfig} = useDocusaurusContext();
  return (
    <Layout
      title="MCP Orchestration for Claude"
      description="Install 17 MCPs and Claude won't even notice. Context-efficient MCP orchestration with on-demand tool loading and skills.">
      <HomepageHeader />
      <main>
        <StatsSection />
        <TerminalDemo />
        <FeaturesSection />
        <CTASection />
      </main>
    </Layout>
  );
}
