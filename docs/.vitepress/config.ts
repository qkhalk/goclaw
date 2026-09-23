import { defineConfig } from 'vitepress'

// GitHub Pages serves this site at https://qkhalk.github.io/goclaw/.
// Override the base path with the BASE env var (e.g. BASE=/ for a custom domain).
const base = process.env.BASE || '/goclaw/'

// The docs/ dir also hosts GoClaw's pre-existing internal markdown library
// (00-*.md … 31-*.md, journals/, runbooks/, superpowers/, ad-hoc notes).
// Only the curated site pages under en/ and vi/ (plus the root index
// redirect) are built; the legacy library stays in the repo as plain files.
const srcExclude = [
  '[0-9][0-9]-*.md',
  'agent-*.md',
  'browser-*.md',
  'codebase-*.md',
  'credential-*.md',
  'deployment-*.md',
  'git-*.md',
  'google-*.md',
  'migrations-*.md',
  'model-*.md',
  'packages-*.md',
  'paseo-*.md',
  'project-*.md',
  'telegram-*.md',
  'tts-*.md',
  'webhooks.md',
  'journals/**',
  'runbooks/**',
  'superpowers/**',
  '**/README.md'
]

// Locale layout: English lives under /en/, Vietnamese under /vi/.
// There is no root locale — the root index.md is a redirect stub to ./en/.
// Both locales have full page parity: every sidebar entry has a translated
// page under both /en/ and /vi/.
const goclawRepo = 'https://github.com/qkhalk/goclaw'

function langSwitcher(current: 'en' | 'vi') {
  return {
    text: current === 'en' ? 'English' : 'Tiếng Việt',
    items: [
      { text: 'English', link: '/en/', activeMatch: '^/en/' },
      { text: 'Tiếng Việt', link: '/vi/', activeMatch: '^/vi/' }
    ]
  }
}

const pagePaths: { group: string; pages: { text: string; link: string }[] }[] = [
  {
    group: 'Getting Started',
    pages: [
      { text: 'Installation', link: 'getting-started/install' },
      { text: 'Configuration', link: 'getting-started/configuration' }
    ]
  },
  { group: 'Overview', pages: [{ text: 'Architecture', link: 'architecture' }] },
  {
    group: 'Core Features',
    pages: [
      { text: 'Agents', link: 'features/agents' },
      { text: 'Memory & Knowledge', link: 'features/memory' },
      { text: 'Orchestration & Teams', link: 'features/orchestration' },
      { text: 'Skills & Skill Market', link: 'features/skills' },
      { text: 'Tools, Browser & MCP', link: 'features/tools' },
      { text: 'Creative Studio', link: 'features/tools-studio' }
    ]
  },
  {
    group: 'Channels',
    pages: [
      { text: 'Overview', link: 'channels/overview' },
      { text: 'Telegram', link: 'channels/telegram' }
    ]
  },
  {
    group: 'Providers',
    pages: [{ text: 'Providers & Models', link: 'providers/overview' }]
  },
  {
    group: 'API',
    pages: [
      { text: 'HTTP API', link: 'api/http' },
      { text: 'WebSocket RPC', link: 'api/websocket' }
    ]
  },
  {
    group: 'Deployment',
    pages: [
      { text: 'Self-Hosting Guide', link: 'self-hosting' },
      { text: 'Desktop (Lite Edition)', link: 'desktop' }
    ]
  },
  { group: 'Help', pages: [{ text: 'Troubleshooting', link: 'troubleshooting' }] }
]

const enSidebar = pagePaths.map((g) => ({
  text: g.group,
  items: g.pages.map((pg) => ({ text: pg.text, link: `/en/${pg.link}` }))
}))

const viLabels: Record<string, string> = {
  'Getting Started': 'Bắt đầu',
  Installation: 'Cài đặt',
  Configuration: 'Cấu hình',
  Overview: 'Tổng quan',
  Architecture: 'Kiến trúc',
  'Core Features': 'Tính năng chính',
  Agents: 'Agent',
  'Memory & Knowledge': 'Bộ nhớ & Tri thức',
  'Orchestration & Teams': 'Điều phối & Nhóm',
  'Skills & Skill Market': 'Skill & Chợ Skill',
  'Tools, Browser & MCP': 'Công cụ, Browser & MCP',
  'Creative Studio': 'Studio sáng tạo',
  Channels: 'Kênh nhắn tin',
  Telegram: 'Telegram',
  Providers: 'Nhà cung cấp',
  'Providers & Models': 'Provider & Model',
  API: 'API',
  'HTTP API': 'HTTP API',
  'WebSocket RPC': 'WebSocket RPC',
  Deployment: 'Triển khai',
  'Self-Hosting Guide': 'Hướng dẫn tự host',
  'Desktop (Lite Edition)': 'Desktop (bản Lite)',
  Help: 'Trợ giúp',
  Troubleshooting: 'Xử lý sự cố'
}

const viSidebar = pagePaths.map((g) => ({
  text: viLabels[g.group] ?? g.group,
  items: g.pages.map((pg) => ({
    text: viLabels[pg.text] ?? pg.text,
    link: `/vi/${pg.link}`
  }))
}))

const enNav = [
  { text: 'Getting Started', link: '/en/getting-started/install' },
  { text: 'Features', link: '/en/features/agents' },
  { text: 'Channels', link: '/en/channels/overview' },
  { text: 'API', link: '/en/api/http' },
  { text: 'Deployment', link: '/en/self-hosting' },
  langSwitcher('en')
]

const viNav = [
  { text: 'Bắt đầu', link: '/vi/getting-started/install' },
  { text: 'Tính năng', link: '/vi/features/agents' },
  { text: 'Kênh nhắn tin', link: '/vi/channels/overview' },
  { text: 'API', link: '/vi/api/http' },
  { text: 'Triển khai', link: '/vi/self-hosting' },
  langSwitcher('vi')
]

export default defineConfig({
  ignoreDeadLinks: true,
  base,
  title: 'GoClaw Docs',
  description:
    'Documentation for GoClaw — a multi-tenant AI agent platform: 40+ LLM providers, 10 channels, agents that orchestrate and create slides, videos and art.',

  // Dark / light toggle (both modes available, follows system by default).
  appearance: true,

  head: [
    // themeConfig.logo/hero get the base auto-prefixed; raw <head> links do
    // not — without base here the favicon 404s on GitHub Pages.
    // Fonts are self-hosted (theme/fonts.css) — no third-party request.
    ['link', { rel: 'icon', type: 'image/svg+xml', href: `${base}logo.svg` }]
  ],

  // The repo README is for GitHub, not a docs page.
  srcExclude,

  locales: {
    en: {
      label: 'English',
      lang: 'en',
      link: '/en/',
      themeConfig: {
        nav: enNav,
        sidebar: enSidebar
      }
    },
    vi: {
      label: 'Tiếng Việt',
      lang: 'vi',
      link: '/vi/',
      themeConfig: {
        nav: viNav,
        sidebar: viSidebar
      }
    }
  },

  themeConfig: {
    // Official GoClaw mascot (same artwork the web app loader uses).
    logo: '/goclaw-icon.svg',

    socialLinks: [{ icon: 'github', link: goclawRepo }],

    // Local search (minisearch-based, no external service).
    search: {
      provider: 'local',
      options: {
        locales: {
          vi: {
            translations: {
              button: {
                buttonText: 'Tìm kiếm',
                buttonAriaLabel: 'Tìm trong tài liệu'
              },
              modal: {
                noResultsText: 'Không tìm thấy kết quả',
                resetButtonTitle: 'Xóa truy vấn',
                footer: {
                  selectText: 'Chọn',
                  navigateText: 'Di chuyển',
                  closeText: 'Đóng'
                }
              }
            }
          }
        }
      }
    },

    outline: { level: [2, 3], label: 'On this page' },

    docFooter: {
      prev: 'Previous',
      next: 'Next'
    },

    footer: {
      message: 'Released under the CC BY-NC 4.0 License.',
      copyright: 'Copyright © GoClaw contributors'
    }
  }
})
