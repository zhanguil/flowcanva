import { test, expect, type Page } from '@playwright/test'

test('洞石砚屿电视柜: white reference → living room → 45° angle → detail retains product locks', async ({ page, request }) => {
  await page.setViewportSize({ width: 1920, height: 1080 })
  const cv = await (await request.post('/api/canvases', { data: { name: '产品血缘验收' } })).json()
  const snapshot = async () => (await (await request.get(`/api/canvases/${cv.id}`)).json())
  await page.goto(`/canvas/#canvas=${cv.id}`)
  await page.getByTestId('dock-close').click()
  let uploads = 0
  page.on('request', r => { if (r.method() === 'POST' && r.url().includes('/assets/upload')) uploads++ })
  await page.getByTestId('canvas-drop-zone').evaluate(element => {
    const canvas = document.createElement('canvas'); canvas.width = 300; canvas.height = 200
    const ctx = canvas.getContext('2d')!; ctx.fillStyle = '#fff'; ctx.fillRect(0, 0, 300, 200)
    const transfer = new DataTransfer()
    transfer.items.add(new File([Uint8Array.from(atob(canvas.toDataURL().split(',')[1]), c => c.charCodeAt(0))], '洞石砚屿-白底测试.png', { type: 'image/png' }))
    element.dispatchEvent(new DragEvent('drop', { bubbles: true, cancelable: true, dataTransfer: transfer, clientX: 260, clientY: 280 }))
  })
  await expect(page.locator('.canvas-node[data-node-type="asset"]')).toHaveCount(1)
  const white = (await snapshot()).nodes.find((n: any) => n.node_type === 'asset')
  const selectNode = async (id: string) => page.locator(`[data-node-id="${id}"].canvas-node`).click({ position: { x: 100, y: 40 } })
  await selectNode(white.id)
  await page.getByTestId('create-product-asset').click()
  await page.getByLabel('新产品名称').fill('洞石砚屿电视柜')
  await page.getByRole('button', { name: '创建产品', exact: true }).click()
  const editor = page.getByTestId('product-lock-editor')
  await expect(editor).toBeVisible()
  await editor.getByText('产品资料（尺寸 / 材质）', { exact: true }).click()
  await editor.getByLabel('产品分类').fill('电视柜')
  for (const [label, value] of [['宽度', '2000'], ['深度', '400'], ['高度', '250'], ['板厚', '20'], ['抽屉数量', '4']]) await editor.getByLabel(label, { exact: true }).fill(value)
  await editor.getByLabel('产品材质').fill('中古胡桃、罗马洞石、亮光黑')
  for (const label of ['结构', '材质', '木纹', '比例', '五金']) await editor.getByLabel(`锁定${label}`, { exact: true }).check()
  await editor.getByLabel('补充约束').fill('保持4格抽屉\n禁止改变木纹方向\n禁止改变五金位置和插座结构')
  await editor.getByRole('button', { name: '保存产品锁' }).click()
  await expect(editor).not.toBeVisible()
  await expect.poll(async () => JSON.parse((await snapshot()).nodes.find((n: any) => n.id === white.id).content).product_snapshot?.metadata.drawerCount).toBe(4)
  const product = JSON.parse((await snapshot()).nodes.find((n: any) => n.id === white.id).content).product_snapshot

  async function continueFrom(sourceId: string, type: string, page: Page) {
    await selectNode(sourceId)
    await page.getByLabel('产品生成操作').click()
    const created = page.waitForResponse(r => r.url().includes(`/api/canvases/${cv.id}/nodes`) && r.request().method() === 'POST')
    const connected = page.waitForResponse(r => r.url().includes('/edges') && r.request().method() === 'POST')
    await page.getByTestId(`continue-${type}`).click()
    const node = await (await created).json()
    await connected
    // Keep the test viewport fixed while walking a horizontally expanding canvas.
    await request.put(`/api/canvases/${cv.id}/nodes/${node.id}`, { data: { x: 500, y: 100 } })
    await page.reload()
    if (await page.getByTestId('right-dock').isVisible()) await page.getByTestId('dock-close').click()
    await selectNode(node.id)
    await expect(page.getByTestId('generation-product')).toHaveValue(product.id)
    await expect(page.getByLabel('生成类型')).toHaveValue(type)
    return node
  }
  let previousGenerationId: string | null = null
  let sourceId = white.id
  for (const [index, type] of ['scene', 'angle', 'detail'].entries()) {
    const node = await continueFrom(sourceId, type, page)
    await page.getByTestId('ratio-3:4').click()
    if (index === 1) {
      await page.getByTestId('reference-card').filter({ hasText: 'AI生成' }).getByLabel('参考角色').selectOption('composition')
    }
    const completed = page.waitForResponse(r => r.url().includes('/api/images/generate') && r.request().method() === 'POST')
    await page.getByTestId('generate-image').click()
    const response = await completed
    expect(response.status()).toBe(200)
    const body = response.request().postDataJSON()
    expect(body.generation_context.product.id).toBe(product.id)
    expect(body.generation_context.constraints).toEqual(product.constraints)
    expect(body.generation_context.constraints.lockedFields).toMatchObject({ drawerCount: 4, width: 2000, height: 250, depth: 400, panelThickness: 20 })
    expect(body.generation_context.references.some((ref: any) => ref.role === 'product')).toBe(true)
    if (index === 1) expect(body.generation_context.references.some((ref: any) => ref.role === 'composition')).toBe(true)
    expect(body.lineage.parentGenerationId).toBe(previousGenerationId)
    const asset = (await response.json()).data[0]
    expect(asset.generation.rootProductAssetId).toBe(product.id)
    expect(asset.generation.context.constraints).toEqual(product.constraints)
    previousGenerationId = asset.generation.id
    await expect.poll(async () => (await snapshot()).nodes.some((n: any) => JSON.parse(n.content || '{}').asset_id === asset.id)).toBe(true)
    const output = (await snapshot()).nodes.find((n: any) => JSON.parse(n.content || '{}').asset_id === asset.id)
    expect(JSON.parse(output.content).generation.id).toBe(asset.generation.id)
    expect(JSON.parse(output.content).product_asset_id).toBe(product.id)
    sourceId = output.id
    // Move each completed generation control out of the next control's location.
    await request.put(`/api/canvases/${cv.id}/nodes/${node.id}`, { data: { x: 450, y: 650 + index * 120 } })
    await page.reload()
    if (await page.getByTestId('right-dock').isVisible()) await page.getByTestId('dock-close').click()
  }
  expect(uploads).toBe(1)
  const stored = await page.evaluate(() => JSON.parse(localStorage.getItem('flowcanva.product-core.v1')!))
  expect(Object.values(stored.generations)).toHaveLength(3)
  expect(stored.generations[previousGenerationId!].context.constraints).toEqual(product.constraints)
  expect(stored.generations[previousGenerationId!].generationType).toBe('detail')

  // A new unconnected generation can reuse the saved product without uploading.
  const newNode = await (await request.post(`/api/canvases/${cv.id}/nodes`, { data: { node_type: 'image', x: 500, y: 100, width: 400, height: 88, content: '{}', config: '{}' } })).json()
  await page.reload()
  await selectNode(newNode.id)
  await page.getByTestId('generation-product').selectOption(product.id)
  await expect(page.getByTestId('reference-card')).toHaveCount(1)
  await page.getByTestId('generation-product-lock').click()
  await expect(page.getByTestId('product-lock-editor').getByLabel('抽屉数量')).toHaveValue('4')
  await expect(page.getByTestId('product-lock-editor').getByLabel('锁定五金')).toBeChecked()
  await page.screenshot({ path: 'test-results/product-lineage-locks.png' })
})
