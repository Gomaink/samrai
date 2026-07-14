export const supportedImportExtensions = ['.cbz', '.zip', '.cbr', '.rar', '.cb7', '.7z', '.cbt', '.tar', '.pdf', '.epub'] as const

export type BatchUploadStatus =
  | 'ready'
  | 'uploading'
  | 'queued'
  | 'processing'
  | 'preparing'
  | 'completed'
  | 'failed'
  | 'duplicate'

export type BatchUploadItem = {
  id: string
  file: File
  title: string
  series: string
  volume: string
  status: BatchUploadStatus
  progress: number
  error: string
  jobId?: number
  bookId?: number
  preparationProgress?: number
  preparationLabel?: string
  preparationWarning?: string
}

const naturalCollator = new Intl.Collator('en-US', { numeric: true, sensitivity: 'base' })

export function isSupportedImportFile(file: File) {
  const lower = file.name.toLowerCase()
  return supportedImportExtensions.some((extension) => lower.endsWith(extension))
}

export function createBatchUploadItems(files: File[]): BatchUploadItem[] {
  return files
    .filter(isSupportedImportFile)
    .sort((left, right) => naturalCollator.compare(left.webkitRelativePath || left.name, right.webkitRelativePath || right.name))
    .map((file) => {
      const metadata = inferImportMetadata(file.name, file.webkitRelativePath)
      return {
        id: globalThis.crypto?.randomUUID?.() ?? `${file.name}-${file.size}-${file.lastModified}-${Math.random()}`,
        file,
        ...metadata,
        status: 'ready',
        progress: 0,
        error: '',
      }
    })
}

export function inferImportMetadata(filename: string, relativePath = '') {
  const extension = supportedImportExtensions.find((suffix) => filename.toLowerCase().endsWith(suffix)) ?? ''
  const originalBase = extension ? filename.slice(0, -extension.length) : filename
  const title = cleanDisplayName(originalBase) || 'Untitled book'
  const folderSeries = inferSeriesFromFolder(relativePath)

  const patterns = [
    /^(.*?)(?:\s*[-–—:]\s*|\s+)(?:vol(?:ume)?|v|tomo|book)\.?\s*0*(\d+(?:[.,]\d+)?)(?:\b.*)?$/iu,
    /^(.*?)(?:\s*[-–—:]\s*|\s+)#\s*0*(\d+(?:[.,]\d+)?)(?:\b.*)?$/iu,
    /^(.*?)(?:\s*[-–—:]\s*|\s+)0*(\d{1,3})(?:\s*)$/u,
  ]

  for (const pattern of patterns) {
    const match = title.match(pattern)
    if (!match) continue
    const series = cleanSeriesName(match[1] ?? '') || folderSeries
    const volume = normalizeVolume(match[2] ?? '')
    if (series && volume) return { title, series, volume }
  }

  return { title, series: folderSeries, volume: '' }
}

export function batchItemKey(file: File) {
  return `${file.name}\u0000${file.size}\u0000${file.lastModified}`
}

function cleanDisplayName(value: string) {
  return value
    .replace(/[_]+/g, ' ')
    .replace(/\s+/g, ' ')
    .trim()
}

function cleanSeriesName(value: string) {
  return cleanDisplayName(value)
    .replace(/[\s._–—:-]+$/g, '')
    .replace(/^[\s._–—:-]+/g, '')
    .trim()
}

function normalizeVolume(value: string) {
  const normalized = value.replace(',', '.')
  if (!normalized.includes('.')) return String(Number.parseInt(normalized, 10))
  return normalized.replace(/^0+(?=\d)/, '').replace(/\.0+$/, '')
}

function inferSeriesFromFolder(relativePath: string) {
  const segments = relativePath.split('/').filter(Boolean)
  if (segments.length < 2) return ''
  return cleanSeriesName(segments[segments.length - 2] ?? '')
}
