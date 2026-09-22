// without-fonts: skip the default Inter bundle — fonts.css provides the site
// fonts (IBM Plex Sans / JetBrains Mono), so Inter would be dead weight.
import DefaultTheme from 'vitepress/theme-without-fonts'
import './fonts.css'
import './custom.css'

export default DefaultTheme
