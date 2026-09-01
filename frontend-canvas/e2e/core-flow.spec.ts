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

interface CanvasSnapshot {
  id: string
  nodes: Array<{ id: string; node_type: string; x: number; y: number; width: number; height: number; content: string }>
  edges: Array<{ id: string; source_node_id: string; target_node_id: string }>
}

const tinyPng = 'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII='

async function createEmptyCanvas(request: APIRequestContext) {
  const response = await request.post('/api/canvases', { data: { name: `drop-${Date.now()}` } })
  expect(response.status()).toBe(201)
  return response.json() as Promise<{ id: string }>
}

async function canvasSnapshot(request: APIRequestContext, canvasID: string): Promise<CanvasSnapshot> {
  const response = await request.get(`/api/canvases/${canvasID}`)
  expect(response.ok()).toBeTruthy()
  const snapshot = await response.json()
  return { ...snapshot, nodes: snapshot.nodes || [], edges: snapshot.edges || [] }
}

async function dropImages(page: Page, targetTestID: string, files: Array<{ name: string; type?: string }>, x = 600, y = 420) {
  await page.getByTestId(targetTestID).evaluate((target, payload) => {
    const transfer = new DataTransfer()
    for (const item of payload.files) {
      const binary = atob(payload.base64)
      const bytes = Uint8Array.from(binary, character => character.charCodeAt(0))
      transfer.items.add(new File([bytes], item.name, { type: item.type || 'image/png' }))
    }
    target.dispatchEvent(new DragEvent('dragover', { bubbles: true, cancelable: true, clientX: payload.x, clientY: payload.y, dataTransfer: transfer }))
    target.dispatchEvent(new DragEvent('drop', { bubbles: true, cancelable: true, clientX: payload.x, clientY: payload.y, dataTransfer: transfer }))
  }, { files, base64: tinyPng, x, y })
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

  await page.locator(`[data-node-id="${testCanvas.nodes.generation_b}"].canvas-node`).click({ position: { x: 100, y: 60 } })
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

  await page.locator(`[data-node-id="${testCanvas.nodes.generation_c}"].canvas-node`).click({ position: { x: 120, y: 60 } })
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

test('image output handle exposes a clear continue-generation action and creates a connected Generation node', async ({ page, request }) => {
  await page.setViewportSize({ width: 1600, height: 1000 })
  const testCanvas = await createTestCanvas(request)
  await openTestCanvas(page, testCanvas)
  await page.getByTestId('dock-close').click()

  const handle = page.locator(`[data-node-id="${testCanvas.nodes.product_a}"][data-connect-handle="right"]`)
  await expect(handle).toBeVisible()
  const box = await handle.boundingBox()
  expect(box).not.toBeNull()
  await page.mouse.move(box!.x + box!.width / 2, box!.y + box!.height / 2)
  await page.mouse.down()
  await page.mouse.move(420, 760, { steps: 10 })
  await page.mouse.up()

  const continueButton = page.getByTestId('connect-create-image')
  await expect(continueButton).toBeVisible()
  await expect(continueButton).toHaveText('继续生图')
  const nodeResponse = page.waitForResponse(response =>
    response.url().includes(`/api/canvases/${testCanvas.canvas_id}/nodes`) && response.request().method() === 'POST',
  )
  const edgeResponse = page.waitForResponse(response =>
    response.url().includes(`/api/canvases/${testCanvas.canvas_id}/edges`) && response.request().method() === 'POST',
  )
  await continueButton.click()
  const created = await (await nodeResponse).json()
  expect(created.node_type).toBe('image')
  const edge = await (await edgeResponse).json()
  expect(edge.source_node_id).toBe(testCanvas.nodes.product_a)
  expect(edge.target_node_id).toBe(created.id)
  await expect(page.locator(`[data-node-id="${created.id}"][data-node-type="image"]`)).toBeVisible()
})

test('Tests 1-2: reference input stays unchanged and every generation creates an independent output node', async ({ page, request }) => {
  await page.setViewportSize({ width: 1920, height: 1080 })
  const testCanvas = await createTestCanvas(request)
  await openTestCanvas(page, testCanvas)
  await page.getByTestId('dock-close').click()

  const before = await canvasSnapshot(request, testCanvas.canvas_id)
  const originalReference = before.nodes.find(node => node.id === testCanvas.nodes.product_a)
  expect(originalReference).toBeTruthy()

  const generationNode = page.locator(`[data-node-id="${testCanvas.nodes.generation_b}"].canvas-node`)
  const generationBox = await generationNode.boundingBox()
  expect(generationBox).not.toBeNull()
  expect(generationBox!.height).toBeLessThanOrEqual(90)
  await expect(generationNode.getByTestId('generation-node-control')).toBeVisible()
  await generationNode.click({ position: { x: 100, y: 60 } })
  await expect(page.getByTestId('image-node-panel')).toBeVisible()
  await expect(page.locator(`[data-node-id="${testCanvas.nodes.generation_b}"] img`)).toHaveCount(0)
  await expect(page.getByTitle('放大预览')).toHaveCount(0)

  for (let generation = 1; generation <= 3; generation++) {
    const response = page.waitForResponse(item => item.url().includes('/api/images/generate') && item.request().method() === 'POST')
    await page.getByTestId('generate-image').click()
    expect((await response).status()).toBe(200)
    await expect.poll(async () => {
      const snapshot = await canvasSnapshot(request, testCanvas.canvas_id)
      return snapshot.nodes.filter(node => {
        try { return JSON.parse(node.content || '{}').parent_generation_node_id === testCanvas.nodes.generation_b } catch { return false }
      }).length
    }).toBe(generation)

    const snapshot = await canvasSnapshot(request, testCanvas.canvas_id)
    expect(snapshot.nodes.find(node => node.id === testCanvas.nodes.product_a)?.content).toBe(originalReference!.content)
  }

  const after = await canvasSnapshot(request, testCanvas.canvas_id)
  const outputs = after.nodes.filter(node => {
    try { return JSON.parse(node.content || '{}').parent_generation_node_id === testCanvas.nodes.generation_b } catch { return false }
  })
  expect(outputs).toHaveLength(3)
  expect(outputs.every(node => node.node_type === 'asset' && node.id !== testCanvas.nodes.product_a)).toBeTruthy()
  await expect(page.locator(`[data-node-id="${testCanvas.nodes.product_a}"] img`)).toHaveCount(1)
})

test('Tests 3-5: Windows-style single, multiple and transformed canvas drops create correctly placed image nodes', async ({ page, request }) => {
  await page.setViewportSize({ width: 1600, height: 1000 })
  const canvas = await createEmptyCanvas(request)
  await page.goto(`/canvas/#canvas=${encodeURIComponent(canvas.id)}`)
  await expect(page.getByTestId('canvas-drop-zone')).toBeVisible()
  await page.getByTestId('dock-close').click()

  await test.step('Test 3: one dropped image creates one Image Node', async () => {
    await dropImages(page, 'canvas-drop-zone', [{ name: 'single.png' }], 520, 360)
    await expect.poll(async () => (await canvasSnapshot(request, canvas.id)).nodes.filter(node => node.node_type === 'asset').length).toBe(1)
  })

  await test.step('Test 4: five dropped images create five staggered nodes', async () => {
    const before = await canvasSnapshot(request, canvas.id)
    await dropImages(page, 'canvas-drop-zone', Array.from({ length: 5 }, (_, index) => ({ name: `multi-${index + 1}.png` })), 420, 260)
    await expect.poll(async () => (await canvasSnapshot(request, canvas.id)).nodes.filter(node => node.node_type === 'asset').length).toBe(6)
    const after = await canvasSnapshot(request, canvas.id)
    const newNodes = after.nodes.filter(node => !before.nodes.some(old => old.id === node.id))
    expect(newNodes).toHaveLength(5)
    expect(new Set(newNodes.map(node => `${node.x},${node.y}`)).size).toBe(5)
  })

  await test.step('Test 5: pan and zoom are applied by screenToCanvasPoint', async () => {
    await page.mouse.move(700, 500)
    await page.mouse.wheel(180, 90)
    await page.keyboard.down('Control')
    await page.mouse.wheel(0, -100)
    await page.keyboard.up('Control')
    const matrix = await page.getByTestId('canvas-world').evaluate(element => {
      const value = new DOMMatrix(getComputedStyle(element).transform)
      return { scale: value.a, x: value.e, y: value.f }
    })
    expect(matrix.scale).not.toBe(1)
    const drop = { x: 760, y: 540 }
    const expected = { x: (drop.x - matrix.x) / matrix.scale - 160, y: (drop.y - matrix.y) / matrix.scale - 150 }
    const before = await canvasSnapshot(request, canvas.id)
    await dropImages(page, 'canvas-drop-zone', [{ name: 'transformed.webp', type: 'image/webp' }], drop.x, drop.y)
    await expect.poll(async () => (await canvasSnapshot(request, canvas.id)).nodes.length).toBe(before.nodes.length + 1)
    const after = await canvasSnapshot(request, canvas.id)
    const created = after.nodes.find(node => !before.nodes.some(old => old.id === node.id))!
    expect(created.x).toBeCloseTo(expected.x, 1)
    expect(created.y).toBeCloseTo(expected.y, 1)
  })
})

test('Tests 6-8: selected and local images populate explicit Assistant context without auto-send; Minimap is absent', async ({ page, request }) => {
  await page.setViewportSize({ width: 1600, height: 1000 })
  const canvas = await createEmptyCanvas(request)
  await page.goto(`/canvas/#canvas=${encodeURIComponent(canvas.id)}`)
  await page.getByTestId('dock-close').click()
  await dropImages(page, 'canvas-drop-zone', [{ name: 'product.png' }, { name: 'scene.jpg', type: 'image/jpeg' }], 450, 300)
  await expect.poll(async () => (await canvasSnapshot(request, canvas.id)).nodes.length).toBe(2)
  const snapshot = await canvasSnapshot(request, canvas.id)

  await page.locator(`[data-node-id="${snapshot.nodes[0].id}"].canvas-node`).dispatchEvent('pointerdown', { pointerId: 1, clientX: 450, clientY: 300 })
  await page.locator(`[data-node-id="${snapshot.nodes[0].id}"].canvas-node`).dispatchEvent('pointerup', { pointerId: 1, clientX: 450, clientY: 300 })
  await page.locator(`[data-node-id="${snapshot.nodes[1].id}"].canvas-node`).dispatchEvent('pointerdown', { pointerId: 2, clientX: 810, clientY: 300, ctrlKey: true })
  await page.locator(`[data-node-id="${snapshot.nodes[1].id}"].canvas-node`).dispatchEvent('pointerup', { pointerId: 2, clientX: 810, clientY: 300, ctrlKey: true })
  await page.getByTestId('open-assistant').click()
  await expect(page.getByTestId('assistant-panel')).toBeVisible()

  await test.step('Test 6: add two selected canvas images', async () => {
    await page.getByTestId('add-selected-images').click()
    await expect(page.getByTestId('visual-context-item')).toHaveCount(2)
  })

  await test.step('Test 7: local drop adds context but sends no chat request', async () => {
    let chatRequests = 0
    page.on('request', req => { if (req.url().includes('/api/assistant/chat')) chatRequests++ })
    await dropImages(page, 'assistant-visual-context', [{ name: 'local-context.webp', type: 'image/webp' }], 1300, 260)
    await expect(page.getByTestId('visual-context-item')).toHaveCount(3)
    expect(chatRequests).toBe(0)
  })

  await test.step('Test 8: Assistant input is visible and Minimap UI is not rendered', async () => {
    await expect(page.getByPlaceholder('输入产品分析、比较或提示词要求…')).toBeVisible()
    await expect(page.getByTestId('minimap')).toHaveCount(0)
    await expect(page.locator('[data-tip="小地图"]')).toHaveCount(0)
  })
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
      await expect(page.getByTestId('assistant-panel')).not.toBeVisible()
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
      await expect(page.getByTestId('right-dock')).not.toBeVisible()
    }
  }

  await page.evaluate(() => { document.documentElement.style.zoom = '1' })
  await page.getByTestId('open-project').click()
  await page.getByTestId('open-assets').click()
  await expect(page.getByTestId('asset-manager')).toBeVisible()
  await expect(page.getByTestId('right-dock')).not.toBeVisible()
})
