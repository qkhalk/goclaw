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
// VitePress does not fall back per-page between locales, so for pages that
// are not translated yet, the vi sidebar/nav links point straight to the
// English page.
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

const enNav = [
  { text: 'Getting Started', link: '/en/getting-started/install' },
  { text: 'Features', link: '/en/features/agents' },
  { text: 'API', link: '/en/api/http' },
  { text: 'Self-Hosting', link: '/en/self-hosting' },
  { text: 'Troubleshooting', link: '/en/troubleshooting' },
  langSwitcher('en')
]

const enSidebar = [
  {
    text: 'Getting Started',
    items: [
      { text: 'Installation', link: '/en/getting-started/install' },
      { text: 'Configuration', link: '/en/getting-started/configuration' }
    ]
  },
  {
    text: 'Overview',
    items: [{ text: 'Architecture', link: '/en/architecture' }]
  },
  {
    text: 'Features',
    items: [
      { text: 'Agents & Subagents', link: '/en/features/agents' },
      { text: 'Skills & Skill Market', link: '/en/features/skills' },
      { text: 'Creative Tools (Studio)', link: '/en/features/tools-studio' }
    ]
  },
  {
    text: 'Channels',
    items: [{ text: 'Telegram', link: '/en/channels/telegram' }]
  },
  {
    text: 'API',
    items: [{ text: 'HTTP API', link: '/en/api/http' }]
  },
  {
    text: 'Deployment',
    items: [{ text: 'Self-Hosting Guide', link: '/en/self-hosting' }]
  },
  {
    text: 'Help',
    items: [{ text: 'Troubleshooting', link: '/en/troubleshooting' }]
  }
]

// Vietnamese pages exist for: index, install, skills. Everything else falls
// back to the English page (linked directly).
const viNav = [
  { text: 'Bắt đầu', link: '/vi/getting-started/install' },
  { text: 'Tính năng', link: '/en/features/agents' },
  { text: 'API', link: '/en/api/http' },
  { text: 'Tự host', link: '/en/self-hosting' },
  { text: 'Xử lý sự cố', link: '/en/troubleshooting' },
  langSwitcher('vi')
]

const viSidebar = [
  {
    text: 'Bắt đầu',
    items: [
      { text: 'Cài đặt', link: '/vi/getting-started/install' },
      { text: 'Cấu hình (EN)', link: '/en/getting-started/configuration' }
    ]
  },
  {
    text: 'Tổng quan',
    items: [{ text: 'Kiến trúc (EN)', link: '/en/architecture' }]
  },
  {
    text: 'Tính năng',
    items: [
      { text: 'Agent & Subagent (EN)', link: '/en/features/agents' },
      { text: 'Skill & Chợ Skill', link: '/vi/features/skills' },
      { text: 'Studio công cụ sáng tạo (EN)', link: '/en/features/tools-studio' }
    ]
  },
  {
    text: 'Kênh nhắn tin',
    items: [{ text: 'Telegram (EN)', link: '/en/channels/telegram' }]
  },
  {
    text: 'API',
    items: [{ text: 'HTTP API (EN)', link: '/en/api/http' }]
  },
  {
    text: 'Triển khai',
    items: [{ text: 'Hướng dẫn tự host (EN)', link: '/en/self-hosting' }]
  },
  {
    text: 'Trợ giúp',
    items: [{ text: 'Xử lý sự cố (EN)', link: '/en/troubleshooting' }]
  }
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
    ['link', { rel: 'icon', type: 'image/svg+xml', href: `${base}logo.svg` }],
    ['link', { rel: 'preconnect', href: 'https://fonts.googleapis.com' }],
    [
      'link',
      { rel: 'preconnect', href: 'https://fonts.gstatic.com', crossorigin: '' }
    ],
    [
      'link',
      {
        rel: 'stylesheet',
        href: 'https://fonts.googleapis.com/css2?family=IBM+Plex+Sans:wght@400;500;600;700&family=JetBrains+Mono:wght@400;500;600&display=swap'
      }
    ]
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
