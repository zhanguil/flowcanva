import { defineConfig } from '@playwright/test'
import { existsSync } from 'node:fs'

const chromeCandidates = [
  'C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe',
  'C:\\Program Files (x86)\\Google\\Chrome\\Application\\chrome.exe',
  process.env.LOCALAPPDATA ? `${process.env.LOCALAPPDATA}\\Google\\Chrome\\Application\\chrome.exe` : '',
].filter(Boolean)
const chromePath = chromeCandidates.find(existsSync)
// Browser Edge is only a fallback executable; it is unrelated to canvas graph Edge data.
const edgePath = 'C:\\Program Files (x86)\\Microsoft\\Edge\\Application\\msedge.exe'
const browserPath = chromePath || (existsSync(edgePath) ? edgePath : '')
if (!browserPath) throw new Error('Playwright requires system Chrome or Edge')

export default defineConfig({
  testDir: './e2e',
  timeout: 60_000,
  expect: { timeout: 10_000 },
  fullyParallel: false,
  workers: 1,
  reporter: [['list']],
  use: {
    baseURL: 'http://127.0.0.1:5175',
    headless: true,
    screenshot: 'only-on-failure',
    trace: 'retain-on-failure',
  },
  projects: [{
    name: chromePath ? 'system-chrome' : 'system-edge-fallback',
    use: { launchOptions: { executablePath: browserPath } },
  }],
  webServer: [
    {
      command: 'powershell.exe -NoProfile -ExecutionPolicy Bypass -File ..\\scripts\\run-e2e-backend.ps1',
      url: 'http://127.0.0.1:6791/api/canvases?page_size=1',
      timeout: 120_000,
      reuseExistingServer: false,
    },
    {
      command: 'node ./node_modules/vite/bin/vite.js --host 127.0.0.1 --port 5175',
      url: 'http://127.0.0.1:5175/canvas/',
      timeout: 120_000,
      reuseExistingServer: false,
      env: { VITE_API_TARGET: 'http://127.0.0.1:6791', VITE_PORT: '5175' },
    },
  ],
})
