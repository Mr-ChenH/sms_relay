import { BrowserQRCodeReader } from '@zxing/browser'
import { DecodeHintType } from '@zxing/library'

const MAX_IMAGE_SIDE = 3200
const MAX_IMAGE_PIXELS = 12_000_000

type BarcodeDetectorConstructor = new (options?: { formats?: string[] }) => {
  detect: (source: CanvasImageSource) => Promise<Array<{ rawValue: string }>>
}

declare global {
  interface Window {
    BarcodeDetector?: BarcodeDetectorConstructor
  }
}

function fittedSize(width: number, height: number) {
  const sideScale = Math.min(1, MAX_IMAGE_SIDE / Math.max(width, height))
  const pixelScale = Math.min(1, Math.sqrt(MAX_IMAGE_PIXELS / (width * height)))
  const scale = Math.min(sideScale, pixelScale)
  return {
    width: Math.max(1, Math.round(width * scale)),
    height: Math.max(1, Math.round(height * scale))
  }
}

function canvasFromSource(source: CanvasImageSource, width: number, height: number) {
  const size = fittedSize(width, height)
  const canvas = document.createElement('canvas')
  canvas.width = size.width
  canvas.height = size.height
  const context = canvas.getContext('2d', { willReadFrequently: true })
  if (!context) throw new Error('canvas-unavailable')
  context.fillStyle = '#fff'
  context.fillRect(0, 0, canvas.width, canvas.height)
  context.drawImage(source, 0, 0, canvas.width, canvas.height)
  return canvas
}

async function loadImage(file: File): Promise<{ source: CanvasImageSource; width: number; height: number; close: () => void }> {
  if ('createImageBitmap' in window) {
    try {
      const bitmap = await createImageBitmap(file, { imageOrientation: 'from-image' })
      return { source: bitmap, width: bitmap.width, height: bitmap.height, close: () => bitmap.close() }
    } catch {
      // Some browser decoders reject formats that an HTML image can still display.
    }
  }

  const url = URL.createObjectURL(file)
  const image = new Image()
  image.decoding = 'async'
  image.src = url
  try {
    await image.decode()
  } catch {
    URL.revokeObjectURL(url)
    throw new Error('image-load-failed')
  }
  return {
    source: image,
    width: image.naturalWidth,
    height: image.naturalHeight,
    close: () => URL.revokeObjectURL(url)
  }
}

function rotatedCanvas(source: HTMLCanvasElement, degrees: number) {
  if (degrees === 0) return source
  const swapSides = degrees === 90 || degrees === 270
  const canvas = document.createElement('canvas')
  canvas.width = swapSides ? source.height : source.width
  canvas.height = swapSides ? source.width : source.height
  const context = canvas.getContext('2d')
  if (!context) throw new Error('canvas-unavailable')
  context.translate(canvas.width / 2, canvas.height / 2)
  context.rotate((degrees * Math.PI) / 180)
  context.drawImage(source, -source.width / 2, -source.height / 2)
  return canvas
}

function enhancedCanvas(source: HTMLCanvasElement, inverted: boolean) {
  const canvas = document.createElement('canvas')
  canvas.width = source.width
  canvas.height = source.height
  const context = canvas.getContext('2d', { willReadFrequently: true })
  if (!context) throw new Error('canvas-unavailable')
  context.drawImage(source, 0, 0)
  const image = context.getImageData(0, 0, canvas.width, canvas.height)
  const data = image.data
  for (let index = 0; index < data.length; index += 4) {
    const luminance = (data[index] * 299 + data[index + 1] * 587 + data[index + 2] * 114) / 1000
    const contrasted = luminance < 128 ? Math.max(0, luminance * 0.72) : Math.min(255, 255 - (255 - luminance) * 0.72)
    const value = inverted ? 255 - contrasted : contrasted
    data[index] = value
    data[index + 1] = value
    data[index + 2] = value
  }
  context.putImageData(image, 0, 0)
  return canvas
}

async function detectNative(source: CanvasImageSource) {
  if (!window.BarcodeDetector) return ''
  try {
    const codes = await new window.BarcodeDetector({ formats: ['qr_code'] }).detect(source)
    return codes.find((code) => code.rawValue)?.rawValue || ''
  } catch {
    return ''
  }
}

function decodeZxing(reader: BrowserQRCodeReader, canvas: HTMLCanvasElement) {
  try {
    return reader.decodeFromCanvas(canvas).getText()
  } catch {
    return ''
  }
}

export async function decodeQrFile(file: File) {
  const image = await loadImage(file)

  try {
    const nativeResult = await detectNative(image.source)
    if (nativeResult) return nativeResult

    const base = canvasFromSource(image.source, image.width, image.height)
    const hints = new Map<DecodeHintType, unknown>([[DecodeHintType.TRY_HARDER, true]])
    const reader = new BrowserQRCodeReader(hints)

    for (const degrees of [0, 90, 180, 270]) {
      const rotated = rotatedCanvas(base, degrees)
      const nativeRotatedResult = await detectNative(rotated)
      if (nativeRotatedResult) return nativeRotatedResult

      const zxingResult = decodeZxing(reader, rotated)
      if (zxingResult) return zxingResult

      for (const inverted of [false, true]) {
        const enhanced = enhancedCanvas(rotated, inverted)
        const enhancedResult = decodeZxing(reader, enhanced)
        if (enhancedResult) return enhancedResult
      }

      await new Promise<void>((resolve) => requestAnimationFrame(() => resolve()))
    }
    return ''
  } finally {
    image.close()
  }
}
