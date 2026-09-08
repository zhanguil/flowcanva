import { test, expect, type Page, type Locator } from '@playwright/test'

async function drop(page: Page, target: Locator, names: string[], x: number, y: number) {
  await target.evaluate((element, payload) => {
    const transfer = new DataTransfer()
    const canvas = document.createElement('canvas')
    canvas.width = 300; canvas.height = 400
    const ctx = canvas.getContext('2d')!
    ctx.fillStyle = '#af9678'; ctx.fillRect(0, 0, 300, 400)
    for (const name of payload.names) {
      const type = name.endsWith('.jpg') ? 'image/jpeg' : name.endsWith('.webp') ? 'image/webp' : 'image/png'
      const bytes = Uint8Array.from(atob(canvas.toDataURL(type).split(',')[1]), c => c.charCodeAt(0))
      transfer.items.add(new File([bytes], name, { type }))
    }
    for (const type of ['dragover', 'drop']) element.dispatchEvent(new DragEvent(type, {
      bubbles: true, cancelable: true, dataTransfer: transfer, clientX: payload.x, clientY: payload.y,
    }))
  }, { names, x, y })
}

async function connect(page: Page, from: string, to: string) {
  const source = await page.locator(`[data-node-id="${from}"][data-connect-handle="right"]`).boundingBox()
  const target = await page.locator(`[data-node-id="${to}"][data-connect-handle="left"]`).boundingBox()
  expect(source).toBeTruthy(); expect(target).toBeTruthy()
  const saved = page.waitForResponse(r => r.url().includes('/edges') && r.request().method() === 'POST')
  await page.mouse.move(source!.x + source!.width / 2, source!.y + source!.height / 2)
  await page.mouse.down()
  await page.mouse.move(target!.x + target!.width / 2, target!.y + target!.height / 2, { steps: 12 })
  await page.mouse.up()
  expect((await saved).status()).toBe(201)
}

test('drop → multi-reference → 3:4 → generated reference → 1:1 uses exact panel payload', async ({ page, request }) => {
  await page.setViewportSize({ width: 1920, height: 1080 })
  const cv = await (await request.post('/api/canvases', { data: { name: 'reference-workflow' } })).json()
  const createGeneration = async (x: number, y: number) => (await (await request.post(`/api/canvases/${cv.id}/nodes`, {
    data: { node_type: 'image', x, y, width: 400, height: 88, content: JSON.stringify({ prompt: '家具电商场景图' }), config: '{}' },
  })).json())
  const first = await createGeneration(520, 100)
  const second = await createGeneration(880, 700)
  const snapshot = async () => (await (await request.get(`/api/canvases/${cv.id}`)).json())
  let uploads = 0
  page.on('request', r => { if (r.method() === 'POST' && r.url().includes('/assets/upload')) uploads++ })
  await page.goto(`/canvas/#canvas=${cv.id}`)
  await page.getByTestId('dock-close').click()
  await drop(page, page.getByTestId('canvas-drop-zone'), ['家具.png'], 260, 200)
  await expect(page.locator('.canvas-node[data-node-type="asset"]')).toHaveCount(1)
  const original = (await snapshot()).nodes.find((n: any) => n.node_type === 'asset')
  // Dropping over an existing image must reach the canvas importer only once.
  await drop(page, page.locator(`[data-node-id="${original.id}"].canvas-node img`), ['材质.jpg', '风格.webp'], 240, 580)
  await expect(page.locator('.canvas-node[data-node-type="asset"]')).toHaveCount(3)
  const references = (await snapshot()).nodes.filter((n: any) => n.node_type === 'asset')
  expect(uploads).toBe(3)
  expect(JSON.parse(references.find((n: any) => n.id === original.id).content).url).toBe(JSON.parse(original.content).url)
  await connect(page, original.id, first.id)
  const extra = references.find((n: any) => JSON.parse(n.content).name === '材质.jpg')
  await connect(page, extra.id, first.id)
  const select = async (id: string) => page.locator(`[data-node-id="${id}"].canvas-node`).click({ position: { x: 100, y: 40 } })
  await select(first.id)
  await expect(page.getByTestId('reference-card')).toHaveCount(2)
  await expect(page.getByTestId('reference-card').first()).toContainText('用户上传')
  await page.getByTestId('ratio-3:4').click()
  await expect(page.getByTestId('ratio-3:4')).toHaveAttribute('aria-pressed', 'true')
  await page.getByTestId('reference-card').filter({ hasText: '材质.jpg' }).getByRole('button', { name: /删除/ }).click()
  await expect(page.getByTestId('reference-card')).toHaveCount(1)
  await select(second.id); await select(first.id)
  await expect(page.getByTestId('reference-card')).toHaveCount(1)
  await expect(page.getByTestId('ratio-3:4')).toHaveAttribute('aria-pressed', 'true')
  const generate = async (expectedURLs: string[], ratio: string) => {
    const response = page.waitForResponse(r => r.url().includes('/api/images/generate') && r.request().method() === 'POST')
    await page.getByTestId('generate-image').click()
    const result = await response
    expect(result.request().postDataJSON()).toMatchObject({ reference_images: expectedURLs, reference_mode: 'explicit', aspect_ratio: ratio })
    expect(result.status()).toBe(200)
    return (await result.json()).data[0]
  }
  const generated = await generate([JSON.parse(original.content).url], '3:4')
  expect(generated.width / generated.height).toBeCloseTo(3 / 4)
  await expect(page.locator('.canvas-node[data-node-type="asset"]')).toHaveCount(4)
  const output = (await snapshot()).nodes.find((n: any) => JSON.parse(n.content || '{}').asset_id === generated.id)
  const outputImage = page.locator(`[data-node-id="${output.id}"].canvas-node img`)
  await expect.poll(async () => outputImage.evaluate(img => {
    const rect = img.getBoundingClientRect(); return rect.width / rect.height
  })).toBeCloseTo(3 / 4, 1)
  await connect(page, output.id, second.id)
  await select(second.id)
  await expect(page.getByTestId('reference-card')).toHaveCount(1)
  await expect(page.getByTestId('reference-card')).toContainText('AI生成')
  await page.getByTestId('ratio-1:1').click()
  const chained = await generate([generated.url], '1:1')
  expect(chained.width).toBe(chained.height)
  await expect(page.locator('.canvas-node[data-node-type="asset"]')).toHaveCount(5)
  expect(uploads).toBe(3)
  const debug = (await (await request.get('/api/dev/generation-debug')).json()).data
  expect(debug.reference_image_count).toBe(1)
  expect(debug.resolved_image_assets).toContain(generated.id)

  // The generated selection survives a reload and the dock cannot cover Generate.
  await page.reload()
  await select(second.id)
  await expect(page.getByTestId('reference-card')).toContainText('AI生成')
  await page.setViewportSize({ width: 1366, height: 768 })
  await expect(page.getByTestId('generate-image')).toBeInViewport()
  await page.getByTestId('generate-image').click({ trial: true })
  await page.screenshot({ path: 'test-results/reference-workflow.png' })
})

test('panel multi-upload waits for edges and a generated batch supports individual removal', async ({ page, request }) => {
  await page.setViewportSize({ width: 1920, height: 1080 })
  const cv = await (await request.post('/api/canvases', { data: { name: 'panel-upload' } })).json()
  const create = async (x: number, y: number) => (await (await request.post(`/api/canvases/${cv.id}/nodes`, {
    data: { node_type: 'image', x, y, width: 400, height: 88, content: JSON.stringify({ prompt: '家具场景' }), config: '{}' },
  })).json())
  const first = await create(520, 100)
  const second = await create(880, 700)
  await page.goto(`/canvas/#canvas=${cv.id}`)
  await page.getByTestId('dock-close').click()
  await page.locator(`[data-node-id="${first.id}"].canvas-node`).click()
  const chooser = page.waitForEvent('filechooser')
  await page.getByRole('button', { name: '添加参考图' }).click()
  const buffer = Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=', 'base64')
  await (await chooser).setFiles(['产品.png', '场景.png'].map(name => ({ name, mimeType: 'image/png', buffer })))
  await expect(page.getByTestId('reference-card')).toHaveCount(2)
  await expect(page.getByTestId('generate-image')).toBeEnabled()
  await page.getByTestId('image-node-panel').getByLabel('生成数量').selectOption('2')
  const generated = page.waitForResponse(r => r.url().includes('/api/images/generate') && r.request().method() === 'POST')
  await page.getByTestId('generate-image').click()
  const result = await generated
  expect(result.request().postDataJSON().reference_images).toHaveLength(2)
  expect(result.status()).toBe(200)
  const outputs = (await result.json()).data
  expect(outputs).toHaveLength(2)
  await expect(page.locator('.canvas-node[data-node-type="asset"]')).toHaveCount(4)
  await connect(page, first.id, second.id)
  await page.locator(`[data-node-id="${second.id}"].canvas-node`).click({ position: { x: 100, y: 40 } })
  await expect(page.getByTestId('reference-card')).toHaveCount(2)
  await page.getByTestId('reference-card').first().getByRole('button', { name: /删除/ }).click()
  await expect(page.getByTestId('reference-card')).toHaveCount(1)
  const chained = page.waitForResponse(r => r.url().includes('/api/images/generate') && r.request().method() === 'POST')
  await page.getByTestId('generate-image').click()
  const response = await chained
  expect(response.request().postDataJSON().reference_images).toEqual([outputs[1].url])
  expect(response.status()).toBe(200)
  expect((await (await request.get('/api/dev/generation-debug')).json()).data.reference_image_count).toBe(1)
})
