export const imageRatioOptions = [
  { value: '1:1', label: '1:1', primary: true },
  { value: '3:4', label: '3:4', primary: true },
  ...['自适应', '4:3', '3:2', '2:3', '16:9', '9:16', '5:4', '4:5', '21:9'].map(value => ({ value, label: value, primary: false })),
]

/** Scale both dimensions together; the header is outside the image viewport. */
export function imageNodeSize(width: number, height: number) {
  if (!(width > 0 && height > 0)) return { w: 320, h: 354 }
  const scale = Math.min(320 / width, 420 / height)
  return { w: Math.max(1, Math.round(width * scale)), h: Math.max(1, Math.round(height * scale)) + 34 }
}
