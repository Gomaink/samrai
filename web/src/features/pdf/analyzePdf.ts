import { GlobalWorkerOptions, getDocument, type PDFDocumentProxy, type PDFPageProxy } from 'pdfjs-dist'
import pdfWorkerURL from 'pdfjs-dist/build/pdf.worker.min.mjs?url'
import {
  api,
  recognizePDFPage,
  uploadPDFCover,
  type OCRStatus,
  type PDFAnalysisInput,
  type PDFClassificationInput,
  type PDFPageText,
} from '../../lib/api'

GlobalWorkerOptions.workerSrc = pdfWorkerURL

export type PDFAnalysisProgress = {
  current: number
  total: number
  percent: number
  label: string
}

export type PDFTextQuality = {
  score: number
  usable: boolean
  hasContent: boolean
  reason: 'empty' | 'valid' | 'controls' | 'private_use' | 'few_letters' | 'symbols'
}

export type PDFAnalysisOptions = {
  allowOCR?: boolean
  skipCover?: boolean
}

type PDFVisualMetrics = {
  whiteRatio: number
  darkRatio: number
  midRatio: number
  edgeRatio: number
  comicScore: number
}

const maxPageCharacters = 900_000
const maxTotalCharacters = 26_000_000
const ocrCanvasWidth = 1800
const sampleCanvasWidth = 680

/**
 * Upload-time PDF preparation is intentionally shallow. It creates the cover,
 * reads metadata and samples a handful of pages. Full OCR is never started by
 * the upload dialog, so image-heavy manga becomes readable immediately.
 */
export async function analyzePDFFile(
  file: File,
  bookId: number,
  onProgress?: (progress: PDFAnalysisProgress) => void,
) {
  const data = await file.arrayBuffer()
  const task = getDocument({ data })
  const pdf = await task.promise
  try {
    return await classifyPDFDocument(pdf, bookId, onProgress)
  } finally {
    await task.destroy()
  }
}

export async function classifyPDFDocument(
  pdf: PDFDocumentProxy,
  bookId: number,
  onProgress?: (progress: PDFAnalysisProgress) => void,
) {
  const total = pdf.numPages
  const metadata = await readMetadata(pdf)
  const ocrStatus = await api.ocrStatus().catch((): OCRStatus => ({ available: false, languages: [], workers: 0 }))
  const ocrLanguage = chooseOCRLanguage(metadata.language, ocrStatus)
  onProgress?.({ current: 0, total, percent: 4, label: 'Generating cover and reading metadata' })

  const firstPage = await pdf.getPage(1)
  try {
    const cover = await renderCover(firstPage)
    await uploadPDFCover(bookId, cover.blob, cover.width, cover.height)
  } catch {
    // Classification can continue if the cover cannot be generated.
  } finally {
    firstPage.cleanup()
  }

  const indices = samplePageIndices(total)
  const samples: PDFPageText[] = []
  const visualSamples: Array<{ pageNumber: number; metrics: PDFVisualMetrics }> = []
  const nonNativePages: Array<{ pageNumber: number; quality: PDFTextQuality; metrics: PDFVisualMetrics }> = []
  let nativeUsable = 0
  let corruptNative = 0

  for (const [sampleIndex, pageNumber] of indices.entries()) {
    const page = await pdf.getPage(pageNumber + 1)
    try {
      const content = await page.getTextContent({ includeMarkedContent: false, disableNormalization: false })
      const nativeText = extractPageText(content.items)
      const nativeRaw = content.items.map((item) => isPDFTextItem(item) ? item.str : '').join('')
      const quality = assessPDFTextQuality(nativeText || nativeRaw)
      if (quality.usable && nativeText) {
        nativeUsable += 1
        samples.push({ page_number: pageNumber, text: trimPageText(nativeText), source: 'pdf', quality: quality.score })
      } else {
        if (quality.hasContent) corruptNative += 1
        const metrics = await measurePageVisuals(page)
        visualSamples.push({ pageNumber, metrics })
        nonNativePages.push({ pageNumber, quality, metrics })
      }
    } finally {
      page.cleanup()
    }
    onProgress?.({
      current: sampleIndex + 1,
      total: indices.length,
      percent: 12 + Math.round(((sampleIndex + 1) / indices.length) * 48),
      label: `Classificando amostra ${sampleIndex + 1} of ${indices.length}`,
    })
    await yieldToBrowser()
  }

  const nativeRatio = nativeUsable / Math.max(1, indices.length)
  const obviousComicVotes = visualSamples.filter((sample) => sample.metrics.comicScore >= 2).length
  const obviousComic = visualSamples.length >= 2 && obviousComicVotes >= Math.ceil(visualSamples.length * 0.6)
  let sampledOCRCharacters = 0
  let sampledOCRLines = 0
  let sampledOCRPages = 0

  // OCR at most two representative pages, and only when visual analysis cannot
  // confidently classify the document. This bounds the initial delay.
  if (nativeRatio < 0.65 && !obviousComic && ocrStatus.available && nonNativePages.length) {
    const candidates = [...nonNativePages]
      .sort((left, right) => left.metrics.comicScore - right.metrics.comicScore)
      .slice(0, 2)
    for (const [index, candidate] of candidates.entries()) {
      const page = await pdf.getPage(candidate.pageNumber + 1)
      try {
        onProgress?.({
          current: index + 1,
          total: candidates.length,
          percent: 64 + Math.round(((index + 1) / candidates.length) * 18),
          label: `Testing OCR on ${index + 1} of ${candidates.length} pages`,
        })
        const image = await renderOCRImage(page)
        const result = await recognizePDFPage(bookId, image, ocrLanguage)
        const text = normalizeText(result.text)
        const quality = assessPDFTextQuality(text)
        if (quality.usable && text) {
          sampledOCRPages += 1
          sampledOCRCharacters += Array.from(text).length
          sampledOCRLines += text.split(/\n+/).filter(Boolean).length
          samples.push({ page_number: candidate.pageNumber, text: trimPageText(text), source: 'ocr', quality: Math.max(0.72, quality.score) })
        }
      } catch {
        // A failed sample never blocks import or reading.
      } finally {
        page.cleanup()
      }
      await yieldToBrowser()
    }
  }

  const averageOCRCharacters = sampledOCRCharacters / Math.max(1, sampledOCRPages)
  const averageOCRLines = sampledOCRLines / Math.max(1, sampledOCRPages)
  let documentKind: PDFClassificationInput['document_kind']
  let textLayer: PDFClassificationInput['text_layer']

  if (nativeRatio >= 0.85) {
    documentKind = 'text'
    textLayer = 'text'
  } else if (nativeUsable > 0) {
    documentKind = 'mixed'
    textLayer = 'mixed'
  } else if (obviousComic || (sampledOCRPages > 0 && averageOCRCharacters < 320 && averageOCRLines < 10)) {
    documentKind = 'comic'
    textLayer = corruptNative > 0 ? 'corrupt' : 'scanned'
  } else if (sampledOCRPages > 0 && (averageOCRCharacters >= 320 || averageOCRLines >= 10)) {
    documentKind = 'scanned_book'
    textLayer = corruptNative > 0 ? 'corrupt' : 'scanned'
  } else {
    const textLikeVisuals = visualSamples.filter((sample) => sample.metrics.comicScore <= -1).length
    documentKind = textLikeVisuals > visualSamples.length / 2 ? 'scanned_book' : 'unknown'
    textLayer = corruptNative > 0 ? 'corrupt' : 'scanned'
  }

  const recommendedOCRMode: PDFClassificationInput['recommended_ocr_mode'] =
    documentKind === 'text' || documentKind === 'comic' ? 'off' : 'on_demand'
  const uniqueSamples = deduplicatePageSamples(samples)
  const input: PDFClassificationInput = {
    page_count: total,
    title: metadata.title,
    author: metadata.author,
    subject: metadata.subject,
    language: metadata.language,
    publication_year: metadata.publicationYear,
    text_layer: textLayer,
    document_kind: documentKind,
    recommended_ocr_mode: recommendedOCRMode,
    sampled_pages: indices.length,
    pages: uniqueSamples,
  }
  onProgress?.({ current: indices.length, total: indices.length, percent: 92, label: 'Saving quick classification' })
  await api.savePDFClassification(bookId, input)
  onProgress?.({
    current: indices.length,
    total: indices.length,
    percent: 100,
    label: documentKind === 'comic'
      ? 'Visual PDF detected · OCR off'
      : documentKind === 'text'
        ? 'Text PDF detected · preparing search when opened'
        : documentKind === 'scanned_book'
          ? 'Scanned book · on-demand OCR available'
          : 'PDF classified · on-demand OCR available',
  })
  return input
}

/** Build a complete text index. OCR is opt-in through allowOCR. */
export async function analyzePDFDocument(
  pdf: PDFDocumentProxy,
  bookId: number,
  onProgress?: (progress: PDFAnalysisProgress) => void,
  options: PDFAnalysisOptions = {},
) {
  const total = pdf.numPages
  const allowOCR = options.allowOCR === true
  onProgress?.({ current: 0, total, percent: 1, label: allowOCR ? 'Preparing full OCR' : 'Preparing text index' })
  const metadata = await readMetadata(pdf)
  const ocrStatus = allowOCR
    ? await api.ocrStatus().catch((): OCRStatus => ({ available: false, languages: [], workers: 0 }))
    : ({ available: false, languages: [], workers: 0 } as OCRStatus)
  const ocrLanguage = chooseOCRLanguage(metadata.language, ocrStatus)

  if (!options.skipCover) {
    const firstPage = await pdf.getPage(1)
    try {
      const cover = await renderCover(firstPage)
      await uploadPDFCover(bookId, cover.blob, cover.width, cover.height)
    } catch {
      // Indexing remains useful if cover generation fails.
    } finally {
      firstPage.cleanup()
    }
  }

  const pages: PDFPageText[] = []
  let nativePages = 0
  let ocrPages = 0
  let emptyPages = 0
  let corruptNativePages = 0
  let totalCharacters = 0

  for (let pageNumber = 0; pageNumber < total; pageNumber += 1) {
    const page = await pdf.getPage(pageNumber + 1)
    try {
      const content = await page.getTextContent({ includeMarkedContent: false, disableNormalization: false })
      const nativeText = extractPageText(content.items)
      const nativeRaw = content.items.map((item) => isPDFTextItem(item) ? item.str : '').join('')
      const nativeQuality = assessPDFTextQuality(nativeText || nativeRaw)
      let text = ''
      let source: PDFPageText['source'] = 'none'
      let quality = nativeQuality.score

      if (nativeQuality.usable && nativeText) {
        text = nativeText
        source = 'pdf'
        nativePages += 1
      } else {
        if (nativeQuality.hasContent) corruptNativePages += 1
        if (allowOCR && ocrStatus.available) {
          onProgress?.({
            current: pageNumber,
            total,
            percent: Math.max(2, Math.round((pageNumber / total) * 94)),
            label: `Running OCR on page ${pageNumber + 1} of ${total}`,
          })
          try {
            const image = await renderOCRImage(page)
            const result = await recognizePDFPage(bookId, image, ocrLanguage)
            const ocrText = normalizeText(result.text)
            const ocrQuality = assessPDFTextQuality(ocrText)
            if (ocrText && ocrQuality.usable) {
              text = ocrText
              source = 'ocr'
              quality = Math.max(0.72, ocrQuality.score)
              ocrPages += 1
            } else {
              emptyPages += 1
            }
          } catch {
            emptyPages += 1
          }
        } else {
          emptyPages += 1
        }
      }

      if (text.length > maxPageCharacters) text = text.slice(0, maxPageCharacters)
      if (totalCharacters + text.length > maxTotalCharacters) {
        const discardedSource = source
        text = ''
        source = 'none'
        quality = 0
        emptyPages += 1
        if (discardedSource === 'pdf' && nativePages > 0) nativePages -= 1
        if (discardedSource === 'ocr' && ocrPages > 0) ocrPages -= 1
      } else {
        totalCharacters += text.length
      }
      pages.push({ page_number: pageNumber, text, source, quality: roundQuality(quality) })

      const current = pageNumber + 1
      onProgress?.({
        current,
        total,
        percent: Math.max(2, Math.round((current / total) * 94)),
        label: source === 'ocr'
          ? `OCR complete for page ${current} of ${total}`
          : `Indexing page ${current} of ${total}`,
      })
    } finally {
      page.cleanup()
    }
    if ((pageNumber + 1) % 4 === 0) await yieldToBrowser()
  }

  const textLayer = classifyTextLayer(total, nativePages, ocrPages, emptyPages, corruptNativePages)
  onProgress?.({ current: total, total, percent: 97, label: 'Saving search index' })
  const input: PDFAnalysisInput = {
    page_count: total,
    title: metadata.title,
    author: metadata.author,
    subject: metadata.subject,
    language: metadata.language,
    publication_year: metadata.publicationYear,
    text_layer: textLayer,
    pages,
  }
  await api.savePDFAnalysis(bookId, input)
  onProgress?.({
    current: total,
    total,
    percent: 100,
    label: ocrPages > 0
      ? `PDF ready · OCR on ${ocrPages} page${ocrPages === 1 ? '' : 's'}`
      : emptyPages > 0
        ? `Partial index · ${emptyPages} page${emptyPages === 1 ? '' : 's'} without text`
        : 'Search ready',
  })
  return input
}

export async function recognizePDFPageOnDemand(
  pdf: PDFDocumentProxy,
  bookId: number,
  pageNumber: number,
  language = '',
) {
  const status = await api.ocrStatus()
  const selectedLanguage = chooseOCRLanguage(language, status)
  const page = await pdf.getPage(pageNumber + 1)
  try {
    const image = await renderOCRImage(page)
    const result = await recognizePDFPage(bookId, image, selectedLanguage)
    const text = normalizeText(result.text)
    const quality = assessPDFTextQuality(text)
    if (!text || !quality.usable) throw new Error('OCR found no searchable text on this page.')
    await api.savePDFPageText(bookId, pageNumber, { text: trimPageText(text), source: 'ocr', quality: Math.max(0.72, quality.score) })
    return { text, quality, result }
  } finally {
    page.cleanup()
  }
}

function samplePageIndices(total: number) {
  const positions = [0, 1, Math.floor(total * 0.1), Math.floor(total * 0.25), Math.floor(total * 0.5), Math.floor(total * 0.75), total - 1]
  return [...new Set(positions.map((value) => Math.max(0, Math.min(total - 1, value))))].sort((left, right) => left - right)
}

async function measurePageVisuals(page: PDFPageProxy): Promise<PDFVisualMetrics> {
  const base = page.getViewport({ scale: 1 })
  const scale = Math.max(0.4, Math.min(1.4, sampleCanvasWidth / Math.max(1, base.width)))
  const viewport = page.getViewport({ scale })
  const canvas = document.createElement('canvas')
  canvas.width = Math.max(1, Math.round(viewport.width))
  canvas.height = Math.max(1, Math.round(viewport.height))
  const context = canvas.getContext('2d', { alpha: false, willReadFrequently: true })
  if (!context) throw new Error('Canvas unavailable')
  context.fillStyle = '#ffffff'
  context.fillRect(0, 0, canvas.width, canvas.height)
  await page.render({ canvas, canvasContext: context, viewport }).promise
  const image = context.getImageData(0, 0, canvas.width, canvas.height)
  let white = 0
  let dark = 0
  let mid = 0
  let edges = 0
  let pixels = 0
  const step = 3
  for (let y = 0; y < canvas.height; y += step) {
    for (let x = 0; x < canvas.width; x += step) {
      const index = (y * canvas.width + x) * 4
      const luminance = (image.data[index] ?? 255) * 0.2126 + (image.data[index + 1] ?? 255) * 0.7152 + (image.data[index + 2] ?? 255) * 0.0722
      if (luminance >= 242) white += 1
      else if (luminance <= 78) dark += 1
      else mid += 1
      if (x >= step) {
        const previousIndex = (y * canvas.width + (x - step)) * 4
        const previous = (image.data[previousIndex] ?? 255) * 0.2126 + (image.data[previousIndex + 1] ?? 255) * 0.7152 + (image.data[previousIndex + 2] ?? 255) * 0.0722
        if (Math.abs(luminance - previous) > 48) edges += 1
      }
      pixels += 1
    }
  }
  canvas.width = 1
  canvas.height = 1
  const whiteRatio = white / Math.max(1, pixels)
  const darkRatio = dark / Math.max(1, pixels)
  const midRatio = mid / Math.max(1, pixels)
  const edgeRatio = edges / Math.max(1, pixels)
  let comicScore = 0
  if (whiteRatio < 0.68) comicScore += 2
  if (midRatio > 0.13) comicScore += 2
  if (darkRatio > 0.19) comicScore += 1
  if (edgeRatio > 0.20) comicScore += 1
  if (whiteRatio > 0.82 && midRatio < 0.08 && darkRatio < 0.13) comicScore -= 2
  if (whiteRatio > 0.90 && darkRatio < 0.08) comicScore -= 1
  return { whiteRatio, darkRatio, midRatio, edgeRatio, comicScore }
}

function deduplicatePageSamples(pages: PDFPageText[]) {
  const byPage = new Map<number, PDFPageText>()
  for (const page of pages) {
    const current = byPage.get(page.page_number)
    if (!current || page.source === 'ocr' || page.quality > current.quality) byPage.set(page.page_number, page)
  }
  return [...byPage.values()].sort((left, right) => left.page_number - right.page_number)
}

function trimPageText(text: string) {
  return text.length > maxPageCharacters ? text.slice(0, maxPageCharacters) : text
}

async function readMetadata(pdf: PDFDocumentProxy) {
  const result = await pdf.getMetadata().catch(() => null)
  const info = (result?.info ?? {}) as Record<string, unknown>
  const xmp = result?.metadata
  const title = textValue(info.Title) || textValue(xmp?.get('dc:title'))
  const author = textValue(info.Author) || textValue(xmp?.get('dc:creator'))
  const subject = textValue(info.Subject) || textValue(xmp?.get('dc:description'))
  const language = textValue(info.Language) || textValue(xmp?.get('dc:language'))
  const date = textValue(info.CreationDate) || textValue(info.ModDate) || textValue(xmp?.get('xmp:createdate'))
  const publicationYear = parsePDFYear(date)
  return { title, author, subject, language, publicationYear }
}

async function renderCover(page: PDFPageProxy) {
  const base = page.getViewport({ scale: 1 })
  const width = Math.min(720, Math.max(360, Math.round(base.width)))
  const scale = width / base.width
  const viewport = page.getViewport({ scale })
  const canvas = document.createElement('canvas')
  canvas.width = Math.max(1, Math.round(viewport.width))
  canvas.height = Math.max(1, Math.round(viewport.height))
  const context = canvas.getContext('2d', { alpha: false })
  if (!context) throw new Error('Canvas unavailable')
  context.fillStyle = '#ffffff'
  context.fillRect(0, 0, canvas.width, canvas.height)
  await page.render({ canvas, canvasContext: context, viewport }).promise
  const blob = await canvasToBlob(canvas, 'image/webp', 0.86)
  canvas.width = 1
  canvas.height = 1
  return { blob, width: Math.round(viewport.width), height: Math.round(viewport.height) }
}

async function renderOCRImage(page: PDFPageProxy) {
  const base = page.getViewport({ scale: 1 })
  const scale = Math.max(1.5, Math.min(3.2, ocrCanvasWidth / base.width))
  const viewport = page.getViewport({ scale })
  const canvas = document.createElement('canvas')
  canvas.width = Math.max(1, Math.round(viewport.width))
  canvas.height = Math.max(1, Math.round(viewport.height))
  const context = canvas.getContext('2d', { alpha: false, willReadFrequently: false })
  if (!context) throw new Error('Canvas unavailable')
  context.fillStyle = '#ffffff'
  context.fillRect(0, 0, canvas.width, canvas.height)
  await page.render({ canvas, canvasContext: context, viewport }).promise
  const blob = await canvasToBlob(canvas, 'image/png')
  canvas.width = 1
  canvas.height = 1
  return blob
}

function canvasToBlob(canvas: HTMLCanvasElement, type: string, quality?: number) {
  return new Promise<Blob>((resolve, reject) => {
    canvas.toBlob((blob) => blob ? resolve(blob) : reject(new Error('Could not generate the image.')), type, quality)
  })
}

type PDFTextItemLike = {
  str: string
  transform: number[]
  width: number
  height: number
  hasEOL?: boolean
}

function isPDFTextItem(item: unknown): item is PDFTextItemLike {
  if (typeof item !== 'object' || item === null) return false
  const candidate = item as Partial<PDFTextItemLike>
  return typeof candidate.str === 'string'
    && Array.isArray(candidate.transform)
    && candidate.transform.length >= 6
    && typeof candidate.width === 'number'
    && typeof candidate.height === 'number'
}

function extractPageText(items: readonly unknown[]) {
  const textItems = items.filter(isPDFTextItem)
  if (!textItems.length) return ''

  let output = ''
  let previous: PDFTextItemLike | null = null
  for (const item of textItems) {
    const value = item.str.replace(/\u0000/g, '')
    if (!value) {
      if (item.hasEOL && output && !output.endsWith('\n')) output += '\n'
      previous = item
      continue
    }
    if (previous && output) {
      const lineBreak = previous.hasEOL || movedToAnotherLine(previous, item)
      if (lineBreak) {
        output = output.replace(/[ \t]+$/g, '')
        if (!output.endsWith('\n')) output += '\n'
      } else if (shouldInsertWordSpace(previous, item, output, value)) {
        output += ' '
      }
    }
    output += value
    if (item.hasEOL) {
      output = output.replace(/[ \t]+$/g, '')
      if (!output.endsWith('\n')) output += '\n'
    }
    previous = item
  }
  return normalizeText(output)
}

function movedToAnotherLine(previous: PDFTextItemLike, current: PDFTextItemLike) {
  const previousY = previous.transform[5] ?? 0
  const currentY = current.transform[5] ?? 0
  const previousX = previous.transform[4] ?? 0
  const currentX = current.transform[4] ?? 0
  const height = Math.max(1, Math.abs(previous.height || previous.transform[3] || 0), Math.abs(current.height || current.transform[3] || 0))
  const verticalShift = Math.abs(currentY - previousY)
  const movedBack = currentX + height * 0.75 < previousX
  return verticalShift > Math.max(1.5, height * 0.48) || (movedBack && verticalShift > height * 0.16)
}

function shouldInsertWordSpace(previous: PDFTextItemLike, current: PDFTextItemLike, output: string, currentValue: string) {
  if (/\s$/.test(output) || /^\s/.test(currentValue)) return false
  const previousX = previous.transform[4] ?? 0
  const currentX = current.transform[4] ?? 0
  const previousEnd = previousX + Math.max(0, previous.width || 0)
  const gap = currentX - previousEnd
  const previousCharacters = Math.max(1, Array.from(previous.str.trim()).length)
  const currentCharacters = Math.max(1, Array.from(current.str.trim()).length)
  const previousCharacterWidth = Math.abs(previous.width || 0) / previousCharacters
  const currentCharacterWidth = Math.abs(current.width || 0) / currentCharacters
  const fontHeight = Math.max(1, Math.abs(previous.height || previous.transform[3] || 0), Math.abs(current.height || current.transform[3] || 0))
  const typicalCharacterWidth = Math.max(0.5, previousCharacterWidth, currentCharacterWidth, fontHeight * 0.22)
  return gap > Math.max(0.65, typicalCharacterWidth * 0.34)
}

export function assessPDFTextQuality(value: string): PDFTextQuality {
  const characters = Array.from(value)
  if (!characters.length || !value.trim()) return { score: 0, usable: false, hasContent: false, reason: 'empty' }
  let letters = 0
  let numbers = 0
  let controls = 0
  let privateUse = 0
  let symbols = 0
  let spaces = 0
  for (const character of characters) {
    const code = character.codePointAt(0) ?? 0
    if (/\p{L}/u.test(character)) letters += 1
    else if (/\p{N}/u.test(character)) numbers += 1
    else if (/\s/u.test(character)) spaces += 1
    else if (/\p{Cc}|\p{Cf}/u.test(character) || character === '\ufffd') controls += 1
    else if ((code >= 0xe000 && code <= 0xf8ff) || (code >= 0xf0000 && code <= 0xffffd)) privateUse += 1
    else if (/\p{S}/u.test(character)) symbols += 1
  }
  const total = Math.max(1, characters.length)
  const visible = Math.max(1, total - spaces)
  const alphaNumeric = letters + numbers
  const controlRatio = controls / total
  const privateRatio = privateUse / total
  const letterRatio = letters / visible
  const symbolRatio = symbols / visible
  const words = value.match(/\p{L}{2,}/gu)?.length ?? 0
  const hasContent = alphaNumeric + controls + privateUse + symbols >= 4
  let reason: PDFTextQuality['reason'] = 'valid'
  if (controlRatio > 0.03) reason = 'controls'
  else if (privateRatio > 0.02) reason = 'private_use'
  else if (letters >= 4 && (letterRatio < 0.30 || words === 0)) reason = 'few_letters'
  else if (symbols > 8 && symbolRatio > 0.20) reason = 'symbols'
  const usable = hasContent && reason === 'valid' && (letters >= 4 || numbers >= 3)
  const score = usable
    ? Math.min(1, Math.max(0.45, 0.55 + letterRatio * 0.45 - symbolRatio * 0.25))
    : Math.max(0, 0.35 - controlRatio - privateRatio - symbolRatio * 0.35)
  return { score: roundQuality(score), usable, hasContent, reason }
}

function classifyTextLayer(total: number, nativePages: number, ocrPages: number, emptyPages: number, corruptNativePages: number): PDFAnalysisInput['text_layer'] {
  if (total > 0 && nativePages === total) return 'text'
  if (total > 0 && ocrPages === total) return 'ocr'
  if (nativePages + ocrPages === total && nativePages > 0 && ocrPages > 0) return 'hybrid'
  if (nativePages === 0 && ocrPages === 0) return corruptNativePages > 0 ? 'corrupt' : 'scanned'
  if (emptyPages > 0 || nativePages > 0 || ocrPages > 0) return 'mixed'
  return 'scanned'
}

function chooseOCRLanguage(language: string, status: OCRStatus) {
  const normalized = language.trim().toLowerCase().replace('_', '-').split('-')[0] ?? ''
  const map: Record<string, string> = { pt: 'por', por: 'por', en: 'eng', eng: 'eng', es: 'spa', spa: 'spa', fr: 'fra', fra: 'fra', de: 'deu', deu: 'deu', it: 'ita', ita: 'ita' }
  const requested = map[normalized] || 'por'
  if (status.languages.includes(requested)) return requested
  if (status.languages.includes('por')) return 'por'
  if (status.languages.includes('eng')) return 'eng'
  return status.languages[0] || requested
}

function normalizeText(value: string) {
  return value.replace(/\u0000/g, '').replace(/[ \t]+\n/g, '\n').replace(/\n{3,}/g, '\n\n').replace(/[ \t]{2,}/g, ' ').trim()
}

function textValue(value: unknown): string {
  if (Array.isArray(value)) return value.map(textValue).filter(Boolean).join(', ')
  if (typeof value !== 'string') return ''
  return normalizeText(value)
}

function parsePDFYear(value: string) {
  const match = value.match(/(?:D:)?(\d{4})/)
  if (!match) return undefined
  const year = Number(match[1])
  return year >= 0 && year <= 9999 ? year : undefined
}

function roundQuality(value: number) {
  return Math.round(Math.max(0, Math.min(1, value)) * 1000) / 1000
}

function yieldToBrowser() {
  return new Promise<void>((resolve) => window.setTimeout(resolve, 0))
}
