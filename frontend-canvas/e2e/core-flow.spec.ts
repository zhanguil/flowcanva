import { expect, test, type APIRequestContext, type Page } from '@playwright/test'

interface TestCanvas {
  canvas_id: string
  nodes: {
    product_a: string
    generation_b: string
    generation_c: string
    reference_d: string
  }
}

async function createTestCanvas(request: APIRequestContext): Promise<TestCanvas> {
  const response = await request.post('/api/dev/test-canvas')
  expect(response.ok()).toBeTruthy()
  return response.json()
}

async function openTestCanvas(page: Page, testCanvas: TestCanvas) {
  await page.goto(`/canvas/#canvas=${encodeURIComponent(testCanvas.canvas_id)}`)
  await expect(page.locator(`[data-node-id="${testCanvas.nodes.generation_b}"].canvas-node`)).toBeVisible()
}

async function connectNodes(page: Page, sourceNodeID: string, targetNodeID: string) {
  const source = page.locator(`[data-node-id="${sourceNodeID}"][data-connect-handle="right"]`)
  const target = page.locator(`[data-node-id="${targetNodeID}"][data-connect-handle="left"]`)
  await expect(source).toBeVisible()
  await expect(target).toBeVisible()
  const sourceBox = await source.boundingBox()
  const targetBox = await target.boundingBox()
  expect(sourceBox).not.toBeNull()
  expect(targetBox).not.toBeNull()

  const edgeResponse = page.waitForResponse(response =>
    response.url().includes('/edges') && response.request().method() === 'POST' && response.status() === 201,
  )
  await page.mouse.move(sourceBox!.x + sourceBox!.width / 2, sourceBox!.y + sourceBox!.height / 2)
  await page.mouse.down()
  await page.mouse.move(targetBox!.x + targetBox!.width / 2, targetBox!.y + targetBox!.height / 2, { steps: 12 })
  await page.mouse.up()
  await edgeResponse
}

test('core data flow: create, connect, mock generate, chain generated output', async ({ page, request }) => {
  await page.setViewportSize({ width: 1920, height: 1080 })
  const testCanvas = await createTestCanvas(request)
  await openTestCanvas(page, testCanvas)

  await page.getByTestId('dock-close').click()

  const createNodeResponse = page.waitForResponse(response =>
    response.url().includes(`/api/canvases/${testCanvas.canvas_id}/nodes`) && response.request().method() === 'POST',
  )
  await page.getByTestId('add-node-image').click()
  const createdNode = await (await createNodeResponse).json()
  await expect(page.locator(`[data-node-id="${createdNode.id}"].canvas-node`)).toBeVisible()

  await connectNodes(page, createdNode.id, testCanvas.nodes.generation_c)

  await page.locator(`[data-node-id="${testCanvas.nodes.generation_b}"].canvas-node`).click({ position: { x: 100, y: 80 } })
  await expect(page.getByTestId('image-node-panel')).toBeVisible()
  const generationResponse = page.waitForResponse(response =>
    response.url().includes('/api/images/generate') && response.request().method() === 'POST',
  )
  await page.getByTestId('generate-image').click()
  const generated = await generationResponse
  expect(generated.status()).toBe(200)
  const generatedBody = await generated.json()
  expect(generatedBody.data).toHaveLength(1)
  const generatedAssetID = generatedBody.data[0].id

  await expect(page.locator('[data-node-type="asset"].canvas-node')).toHaveCount(3)
  const canvasResponse = await request.get(`/api/canvases/${testCanvas.canvas_id}`)
  const canvas = await canvasResponse.json()
  const outputNode = canvas.nodes.find((node: any) => {
    try { return JSON.parse(node.content || '{}').asset_id === generatedAssetID } catch { return false }
  })
  expect(outputNode).toBeTruthy()
  await connectNodes(page, outputNode.id, testCanvas.nodes.generation_c)

  await page.locator(`[data-node-id="${testCanvas.nodes.generation_c}"].canvas-node`).click({ position: { x: 120, y: 90 } })
  await expect(page.getByTestId('image-node-panel')).toBeVisible()
  const chainedResponse = page.waitForResponse(response =>
    response.url().includes('/api/images/generate') && response.request().method() === 'POST',
  )
  await page.getByTestId('generate-image').click()
  expect((await chainedResponse).status()).toBe(200)

  const debugResponse = await request.get('/api/dev/generation-debug')
  const debug = (await debugResponse.json()).data
  expect(debug.node_id).toBe(testCanvas.nodes.generation_c)
  expect(debug.resolved_input_nodes).toEqual(expect.arrayContaining([
    testCanvas.nodes.generation_b,
    testCanvas.nodes.reference_d,
    outputNode.id,
  ]))
  expect(debug.resolved_image_assets).toContain(generatedAssetID)
  expect(debug.reference_image_count).toBeGreaterThanOrEqual(2)
})

test('RightDock stays single, visible and closable across target viewports and zooms', async ({ page, request }) => {
  const testCanvas = await createTestCanvas(request)
  await openTestCanvas(page, testCanvas)

  const viewports = [
    { width: 1920, height: 1080 },
    { width: 1600, height: 900 },
    { width: 1440, height: 900 },
    { width: 1366, height: 768 },
  ]
  const zooms = [0.8, 1, 1.25]

  for (const viewport of viewports) {
    await page.setViewportSize(viewport)
    for (const zoom of zooms) {
      await page.evaluate(value => { document.documentElement.style.zoom = String(value) }, zoom)

      const assistantButton = page.getByTestId('open-assistant')
      if (!(await page.getByTestId('assistant-panel').isVisible())) await assistantButton.click()
      await expect(page.getByTestId('assistant-panel')).toBeVisible()
      await expect(page.getByPlaceholder('输入产品分析、比较或提示词要求…')).toBeVisible()

      await page.getByTestId('open-project').click()
      await expect(page.getByTestId('project-panel')).toBeVisible()
      await expect(page.getByTestId('assistant-panel')).toHaveCount(0)
      await expect(page.getByTestId('right-dock')).toHaveCount(1)

      const box = await page.getByTestId('right-dock').boundingBox()
      expect(box).not.toBeNull()
      expect(box!.x).toBeGreaterThanOrEqual(0)
      expect(box!.y).toBeGreaterThanOrEqual(0)
      expect(box!.x + box!.width).toBeLessThanOrEqual(viewport.width + 1)
      expect(box!.y + box!.height).toBeLessThanOrEqual(viewport.height + 1)

      await page.getByTestId('dock-tab-layers').click()
      await expect(page.getByTestId('layers-panel')).toBeVisible()
      await page.getByTestId('dock-close').click()
      await expect(page.getByTestId('right-dock')).toHaveCount(0)
    }
  }

  await page.evaluate(() => { document.documentElement.style.zoom = '1' })
  await page.getByTestId('open-project').click()
  await page.getByTestId('open-assets').click()
  await expect(page.getByTestId('asset-manager')).toBeVisible()
  await expect(page.getByTestId('right-dock')).toHaveCount(0)
})
