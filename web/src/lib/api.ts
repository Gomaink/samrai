export interface User {
  id: number
  username: string
  role: 'admin' | 'reader' | string
  disabled: boolean
  created_at: string
  updated_at: string
}

export interface InstanceSettings {
  name: string
  default_reading_direction: 'ltr' | 'rtl'
}

export interface AuthStatus {
  setup_required: boolean
  authenticated: boolean
  user?: User
  instance: InstanceSettings
}

export interface AuthResponse {
  user: User
}

export interface UsersResponse { items: User[] }
export interface SystemInfo {
  build: { version: string; commit: string; date: string; go: string; os: string; arch: string }
  storage: { database_bytes: number; library_bytes: number; prepared_bytes: number; cache_bytes: number; backup_bytes: number }
  database: { users: number; books: number; series: number; pages: number; jobs: number }
  limits: { max_upload_bytes: number; image_cache_bytes: number; image_workers: number; image_quality: number; image_max_width: number }
  uptime_seconds: number
  data_directory: string
}
export interface AuditEvent { id: number; actor: string; event_type: string; subject: string; detail: string; created_at: string }
export interface AuditEventsResponse { items: AuditEvent[] }
export interface LogsResponse { items: string[] }

export interface Book {
  id: number
  title: string
  original_filename: string
  file_size: number
  page_count: number
  status: 'processing' | 'ready' | 'failed' | string
  format: 'cbz' | 'cbr' | 'cb7' | 'cbt' | 'pdf' | 'epub' | string
  pdf_analysis_status: 'not_applicable' | 'pending' | 'complete' | 'failed' | string
  pdf_text_layer: 'unknown' | 'text' | 'scanned' | 'mixed' | 'corrupt' | 'ocr' | 'hybrid' | string
  pdf_ocr_status: 'not_needed' | 'idle' | 'disabled' | 'needed' | 'partial' | 'complete' | string
  pdf_invalid_pages: number
  pdf_ocr_pages: number
  pdf_document_kind: 'unknown' | 'text' | 'scanned_book' | 'comic' | 'mixed' | string
  pdf_classification_status: 'pending' | 'complete' | 'failed' | string
  pdf_ocr_mode: 'auto' | 'off' | 'on_demand' | 'background' | 'full' | string
  pdf_sampled_pages: number
  pdf_indexed_pages: number
  pdf_analyzed_at?: string
  epub_layout: 'not_applicable' | 'fixed' | 'reflowable' | string
  epub_version: string
  series_id?: number
  series: string
  summary: string
  writer: string
  publisher: string
  publication_year?: number
  volume: string
  number: string
  language: string
  reading_direction?: 'ltr' | 'rtl'
  current_page: number
  reading_location?: Record<string, unknown>
  started: boolean
  completed: boolean
  created_at: string
  updated_at: string
  cover_url: string
  favorite: boolean
}


export interface UpdateBookInput {
  title?: string
  summary?: string
  writer?: string
  publisher?: string
  publication_year?: number
  clear_publication_year?: boolean
  volume?: string
  number?: string
  language?: string
  reading_direction?: 'ltr' | 'rtl' | ''
  series?: string
}


export interface LibraryStats {
  total_books: number
  unread_books: number
  reading_books: number
  completed_books: number
  total_series: number
}

export interface Series {
  id: number
  title: string
  description: string
  reading_direction?: 'ltr' | 'rtl'
  book_count: number
  started_count: number
  completed_count: number
  page_count: number
  cover_book_id: number
  cover_url: string
  created_at: string
  updated_at: string
  favorite: boolean
}

export interface SeriesResponse {
  items: Series[]
  total: number
}

export interface SeriesDetail {
  series: Series
  books: Book[]
}

export interface DashboardResponse {
  continue_reading: Book[]
  recently_added: Book[]
  series: Series[]
  stats: LibraryStats
}

export interface BookQuery {
  search?: string
  status?: 'all' | 'unread' | 'reading' | 'completed'
  sort?: 'recent' | 'oldest' | 'title' | 'series' | 'progress' | 'year'
  series_id?: number
  limit?: number
  offset?: number
  favorite?: boolean
}

export interface SeriesQuery {
  search?: string
  sort?: 'title' | 'recent' | 'count'
  limit?: number
  offset?: number
  favorite?: boolean
}

export interface SystemMetrics {
  runtime: {
    uptime_seconds: number
    goroutines: number
    heap_bytes: number
    heap_in_use_bytes: number
    system_bytes: number
    total_alloc_bytes: number
    garbage_cycles: number
  }
  images: {
    hits: number
    misses: number
    generated: number
    generated_bytes: number
    generation_milliseconds: number
    average_generation_milliseconds: number
    hit_rate: number
    cache_bytes: number
    cache_limit_bytes: number
    active_workers: number
    worker_limit: number
    quality: number
    max_width: number
  }
}

export interface BooksResponse {
  items: Book[]
  total: number
}

export interface ReadingProgress {
  book_id: number
  current_page: number
  started: boolean
  completed: boolean
  updated_at: string
  location?: Record<string, unknown>
}

export interface EPUBSpineItem {
  index: number
  item_id: string
  path: string
  media_type: string
  properties: string
  linear: boolean
  title: string
  content_url: string
  searchable: boolean
}

export interface EPUBTOCItem {
  position: number
  label: string
  path: string
  fragment: string
  spine_index?: number
  depth: number
}

export interface EPUBPublication {
  book_id: number
  layout: 'fixed' | 'reflowable' | string
  version: string
  reading_direction: 'ltr' | 'rtl'
  spine: EPUBSpineItem[]
  toc: EPUBTOCItem[]
}

export interface EPUBSearchResult {
  spine_index: number
  title: string
  excerpt: string
  matches: number
}

export interface EPUBSearchResponse { items: EPUBSearchResult[] }

export interface EPUBReadingLocation {
  spine_index: number
  column_index?: number
  column_count?: number
  chapter_progress?: number
}

export interface EPUBAnnotationAnchor {
  start_path: string
  start_offset: number
  end_path: string
  end_offset: number
  prefix?: string
  suffix?: string
}

export interface EPUBAnnotation {
  id: number
  book_id: number
  spine_index: number
  resource_path: string
  kind: 'highlight' | 'note'
  color: AnnotationColor
  selected_text: string
  note: string
  anchor: EPUBAnnotationAnchor
  created_at: string
  updated_at: string
}

export interface EPUBAnnotationsResponse { items: EPUBAnnotation[] }

export interface CreateEPUBAnnotationInput {
  spine_index: number
  resource_path: string
  kind: 'highlight' | 'note'
  color: AnnotationColor
  selected_text: string
  note?: string
  anchor: EPUBAnnotationAnchor
}

export type AnnotationColor = 'yellow' | 'green' | 'blue' | 'pink' | 'orange'
export type AnnotationKind = 'highlight' | 'area'

export interface AnnotationRect {
  x: number
  y: number
  width: number
  height: number
}

export interface AnnotationAnchor {
  rects: AnnotationRect[]
  prefix?: string
  suffix?: string
}

export interface Annotation {
  id: number
  book_id: number
  page_number: number
  kind: AnnotationKind
  color: AnnotationColor
  selected_text: string
  note: string
  anchor: AnnotationAnchor
  created_at: string
  updated_at: string
}

export interface AnnotationsResponse { items: Annotation[] }

export interface CreateAnnotationInput {
  page_number: number
  kind: AnnotationKind
  color: AnnotationColor
  selected_text: string
  note?: string
  anchor: AnnotationAnchor
}


export interface PDFPageText {
  page_number: number
  text: string
  source: 'pdf' | 'ocr' | 'none'
  quality: number
}

export interface PDFAnalysisInput {
  page_count: number
  title: string
  author: string
  subject: string
  language: string
  publication_year?: number
  text_layer: 'text' | 'scanned' | 'mixed' | 'corrupt' | 'ocr' | 'hybrid'
  pages: PDFPageText[]
}


export interface PDFClassificationInput {
  page_count: number
  title: string
  author: string
  subject: string
  language: string
  publication_year?: number
  text_layer: 'text' | 'scanned' | 'mixed' | 'corrupt'
  document_kind: 'unknown' | 'text' | 'scanned_book' | 'comic' | 'mixed'
  recommended_ocr_mode: 'off' | 'on_demand' | 'background' | 'full'
  sampled_pages: number
  pages: PDFPageText[]
}

export interface PDFPageTextInput {
  text: string
  source: 'pdf' | 'ocr'
  quality: number
}

export interface PDFSearchResult {
  page_number: number
  excerpt: string
  matches: number
  match_type: 'exact' | 'repaired'
  text_source: 'pdf' | 'ocr'
}

export interface PDFSearchResponse { items: PDFSearchResult[] }

export interface FavoritesResponse {
  books: Book[]
  series: Series[]
}

export interface ReadingSession {
  id: number
  book_id: number
  book_title: string
  book_format: string
  cover_url: string
  started_at: string
  last_activity_at: string
  ended_at?: string
  start_page: number
  end_page: number
  start_location?: Record<string, unknown>
  end_location?: Record<string, unknown>
  duration_seconds: number
}

export interface HistoryResponse { items: ReadingSession[] }

export interface AnnotationSummary {
  source: 'pdf' | 'epub'
  id: number
  book_id: number
  book_title: string
  book_format: string
  cover_url: string
  position: number
  location: string
  color: AnnotationColor
  selected_text: string
  note: string
  created_at: string
  updated_at: string
}

export interface AllAnnotationsResponse { items: AnnotationSummary[]; total: number }

export interface OCRStatus {
  available: boolean
  executable?: string
  languages: string[]
  workers: number
  message?: string
}

export interface OCRResult {
  text: string
  language: string
  duration_ms: number
  confidence?: number
}

export interface ImportJob {
  id: number
  type: string
  status: 'pending' | 'running' | 'completed' | 'failed' | 'cancelled' | string
  progress: number
  original_filename: string
  error_message?: string
  book_id?: number
  created_at: string
  started_at?: string
  completed_at?: string
}

export interface JobsResponse {
  items: ImportJob[]
}

export interface UploadBookMetadata {
  title?: string
  series?: string
  volume?: string
}

interface ErrorEnvelope {
  error?: {
    code?: string
    message?: string
  }
}

export class ApiError extends Error {
  readonly status: number
  readonly code: string

  constructor(status: number, code: string, message: string) {
    super(message)
    this.name = 'ApiError'
    this.status = status
    this.code = code
  }
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const headers = new Headers(init?.headers)
  headers.set('Accept', 'application/json')
  if (init?.body && !headers.has('Content-Type') && !(init.body instanceof FormData)) {
    headers.set('Content-Type', 'application/json')
  }

  const response = await fetch(path, {
    ...init,
    headers,
    credentials: 'same-origin',
  })

  if (!response.ok) {
    throw await readApiError(response)
  }

  if (response.status === 204) {
    return undefined as T
  }

  return (await response.json()) as T
}

async function readApiError(response: Response): Promise<ApiError> {
  let payload: ErrorEnvelope | undefined
  try {
    payload = (await response.json()) as ErrorEnvelope
  } catch {
    payload = undefined
  }

  return new ApiError(
    response.status,
    payload?.error?.code ?? 'request_failed',
    payload?.error?.message ?? 'Could not complete the request.',
  )
}

export function uploadBook(
  file: File,
  metadata: UploadBookMetadata,
  onProgress: (progress: number) => void,
  signal?: AbortSignal,
): Promise<ImportJob> {
  return new Promise((resolve, reject) => {
    const request = new XMLHttpRequest()
    const form = new FormData()
    form.append('file', file)

    const query = new URLSearchParams()
    if (metadata.title?.trim()) query.set('title', metadata.title.trim())
    if (metadata.series?.trim()) query.set('series', metadata.series.trim())
    if (metadata.volume?.trim()) query.set('volume', metadata.volume.trim())
    const endpoint = query.size ? `/api/v1/uploads?${query.toString()}` : '/api/v1/uploads'

    request.open('POST', endpoint)
    request.responseType = 'json'
    request.withCredentials = true
    request.setRequestHeader('Accept', 'application/json')

    request.upload.addEventListener('progress', (event) => {
      if (!event.lengthComputable) return
      onProgress(Math.min(100, Math.round((event.loaded / event.total) * 100)))
    })

    request.addEventListener('load', () => {
      if (request.status >= 200 && request.status < 300) {
        resolve(request.response as ImportJob)
        return
      }
      const payload = request.response as ErrorEnvelope | null
      reject(new ApiError(
        request.status,
        payload?.error?.code ?? 'upload_failed',
        payload?.error?.message ?? 'Could not upload the file.',
      ))
    })
    request.addEventListener('error', () => reject(new ApiError(0, 'network_error', 'The server connection was interrupted.')))
    request.addEventListener('abort', () => reject(new ApiError(0, 'upload_cancelled', 'The upload was cancelled.')))
    if (signal) {
      if (signal.aborted) {
        request.abort()
        return
      }
      signal.addEventListener('abort', () => request.abort(), { once: true })
    }
    request.send(form)
  })
}


function withQuery(path: string, values: object) {
  const query = new URLSearchParams()
  Object.entries(values).forEach(([key, value]) => {
    if (value === undefined || value === null || value === '') return
    if (typeof value !== 'string' && typeof value !== 'number' && typeof value !== 'boolean') return
    query.set(key, typeof value === 'boolean' ? (value ? '1' : '0') : String(value))
  })
  const encoded = query.toString()
  return encoded ? `${path}?${encoded}` : path
}

export const api = {
  authStatus: () => request<AuthStatus>('/api/v1/auth/status'),
  setup: (username: string, password: string) =>
    request<AuthResponse>('/api/v1/auth/setup', {
      method: 'POST',
      body: JSON.stringify({ username, password }),
    }),
  login: (username: string, password: string) =>
    request<AuthResponse>('/api/v1/auth/login', {
      method: 'POST',
      body: JSON.stringify({ username, password }),
    }),
  logout: () => request<void>('/api/v1/auth/logout', { method: 'POST' }),
  changePassword: (currentPassword: string, newPassword: string) => request<void>('/api/v1/auth/password', { method: 'PUT', body: JSON.stringify({ current_password: currentPassword, new_password: newPassword }) }),
  settings: () => request<InstanceSettings>('/api/v1/settings'),
  updateSettings: (input: InstanceSettings) => request<InstanceSettings>('/api/v1/settings', { method: 'PATCH', body: JSON.stringify(input) }),
  users: () => request<UsersResponse>('/api/v1/users'),
  createUser: (username: string, password: string, role: 'admin' | 'reader') => request<User>('/api/v1/users', { method: 'POST', body: JSON.stringify({ username, password, role }) }),
  updateUser: (userId: number, input: { role?: 'admin' | 'reader'; disabled?: boolean }) => request<User>(`/api/v1/users/${userId}`, { method: 'PATCH', body: JSON.stringify(input) }),
  resetUserPassword: (userId: number, password: string) => request<void>(`/api/v1/users/${userId}/password`, { method: 'PUT', body: JSON.stringify({ password }) }),
  deleteUser: (userId: number) => request<void>(`/api/v1/users/${userId}`, { method: 'DELETE' }),
  systemInfo: () => request<SystemInfo>('/api/v1/system/info'),
  logs: () => request<LogsResponse>('/api/v1/system/logs?limit=100'),
  audit: () => request<AuditEventsResponse>('/api/v1/system/audit'),
  dashboard: () => request<DashboardResponse>('/api/v1/dashboard'),
  books: (query: BookQuery = {}) => request<BooksResponse>(withQuery('/api/v1/books', query)),
  series: (query: SeriesQuery = {}) => request<SeriesResponse>(withQuery('/api/v1/series', query)),
  seriesDetail: (seriesId: number) => request<SeriesDetail>(`/api/v1/series/${seriesId}`),
  book: (bookId: number) => request<Book>(`/api/v1/books/${bookId}`),
  updateBook: (bookId: number, input: UpdateBookInput) =>
    request<Book>(`/api/v1/books/${bookId}`, { method: 'PATCH', body: JSON.stringify(input) }),
  deleteBook: (bookId: number) => request<void>(`/api/v1/books/${bookId}`, { method: 'DELETE' }),
  resetProgress: (bookId: number) => request<void>(`/api/v1/books/${bookId}/progress`, { method: 'DELETE' }),
  setBooksCompletion: (bookIds: number[], completed: boolean) => request<{ updated: number; completed: boolean }>('/api/v1/books/progress/completion', { method: 'PATCH', body: JSON.stringify({ book_ids: bookIds, completed }) }),
  metrics: () => request<SystemMetrics>('/api/v1/system/metrics'),
  saveProgress: (bookId: number, currentPage: number, location?: object, sessionId?: string) =>
    request<ReadingProgress>(`/api/v1/books/${bookId}/progress`, {
      method: 'PUT',
      body: JSON.stringify({ current_page: currentPage, location, session_id: sessionId }),
    }),
  endReadingSession: (bookId: number, sessionId: string) => request<void>('/api/v1/reading-sessions/end', { method: 'POST', body: JSON.stringify({ book_id: bookId, session_id: sessionId }) }),
  favorites: () => request<FavoritesResponse>('/api/v1/favorites'),
  setBookFavorite: (bookId: number, favorite: boolean) => request<{ favorite: boolean }>(`/api/v1/books/${bookId}/favorite`, { method: favorite ? 'PUT' : 'DELETE' }),
  setSeriesFavorite: (seriesId: number, favorite: boolean) => request<{ favorite: boolean }>(`/api/v1/series/${seriesId}/favorite`, { method: favorite ? 'PUT' : 'DELETE' }),
  history: () => request<HistoryResponse>('/api/v1/history'),
  clearHistory: () => request<void>('/api/v1/history', { method: 'DELETE' }),
  allAnnotations: (query: { search?: string; color?: string; source?: string; limit?: number; offset?: number } = {}) => request<AllAnnotationsResponse>(withQuery('/api/v1/annotations', query)),
  purgeImageCache: () => request<void>('/api/v1/system/cache/images', { method: 'DELETE' }),
  epub: (bookId: number) => request<EPUBPublication>(`/api/v1/books/${bookId}/epub`),
  searchEPUB: (bookId: number, query: string, limit = 50) => request<EPUBSearchResponse>(withQuery(`/api/v1/books/${bookId}/epub/search`, { q: query, limit })),
  epubAnnotations: (bookId: number) => request<EPUBAnnotationsResponse>(`/api/v1/books/${bookId}/epub/annotations`),
  createEPUBAnnotation: (bookId: number, input: CreateEPUBAnnotationInput) => request<EPUBAnnotation>(`/api/v1/books/${bookId}/epub/annotations`, { method: 'POST', body: JSON.stringify(input) }),
  updateEPUBAnnotation: (annotationId: number, input: { color?: AnnotationColor; note?: string }) => request<EPUBAnnotation>(`/api/v1/epub-annotations/${annotationId}`, { method: 'PATCH', body: JSON.stringify(input) }),
  deleteEPUBAnnotation: (annotationId: number) => request<void>(`/api/v1/epub-annotations/${annotationId}`, { method: 'DELETE' }),
  jobs: () => request<JobsResponse>('/api/v1/jobs'),
  updatePDFMetadata: (bookId: number, pageCount: number) => request<void>(`/api/v1/books/${bookId}/pdf/metadata`, { method: 'PUT', body: JSON.stringify({ page_count: pageCount }) }),
  savePDFAnalysis: (bookId: number, input: PDFAnalysisInput) => request<void>(`/api/v1/books/${bookId}/pdf/analysis`, { method: 'PUT', body: JSON.stringify(input) }),
  savePDFClassification: (bookId: number, input: PDFClassificationInput) => request<void>(`/api/v1/books/${bookId}/pdf/classification`, { method: 'PUT', body: JSON.stringify(input) }),
  updatePDFOCRMode: (bookId: number, mode: 'auto' | 'off' | 'on_demand' | 'background' | 'full') => request<void>(`/api/v1/books/${bookId}/pdf/ocr-mode`, { method: 'PATCH', body: JSON.stringify({ mode }) }),
  savePDFPageText: (bookId: number, pageNumber: number, input: PDFPageTextInput) => request<void>(`/api/v1/books/${bookId}/pdf/pages/${pageNumber}/text`, { method: 'PUT', body: JSON.stringify(input) }),
  searchPDF: (bookId: number, query: string, limit = 50) => request<PDFSearchResponse>(withQuery(`/api/v1/books/${bookId}/pdf/search`, { q: query, limit })),
  ocrStatus: () => request<OCRStatus>(`/api/v1/system/ocr`),
  annotations: (bookId: number) => request<AnnotationsResponse>(`/api/v1/books/${bookId}/annotations`),
  createAnnotation: (bookId: number, input: CreateAnnotationInput) => request<Annotation>(`/api/v1/books/${bookId}/annotations`, { method: 'POST', body: JSON.stringify(input) }),
  updateAnnotation: (annotationId: number, input: { color?: AnnotationColor; note?: string }) => request<Annotation>(`/api/v1/annotations/${annotationId}`, { method: 'PATCH', body: JSON.stringify(input) }),
  deleteAnnotation: (annotationId: number) => request<void>(`/api/v1/annotations/${annotationId}`, { method: 'DELETE' }),
}

export function pageImageURL(bookId: number, pageNumber: number, width?: number) {
  const query = width && width > 0 ? `?width=${Math.round(width)}&format=webp` : ''
  return `/api/v1/books/${bookId}/pages/${pageNumber}/image${query}`
}

export function pdfFileURL(bookId: number) {
  return `/api/v1/books/${bookId}/file`
}

export function annotationsExportURL(bookId: number) {
  return `/api/v1/books/${bookId}/annotations/export`
}

export function epubAnnotationsExportURL(bookId: number) {
  return `/api/v1/books/${bookId}/epub/annotations/export`
}

export function allAnnotationsExportURL() {
  return '/api/v1/annotations/export'
}

export async function uploadPDFCover(bookId: number, blob: Blob, width: number, height: number) {
  const response = await fetch(`/api/v1/books/${bookId}/pdf/cover?width=${Math.round(width)}&height=${Math.round(height)}`, {
    method: 'PUT',
    body: blob,
    credentials: 'same-origin',
    headers: { 'Content-Type': blob.type || 'image/webp', Accept: 'application/json' },
  })
  if (!response.ok) throw await readApiError(response)
}


export async function recognizePDFPage(bookId: number, blob: Blob, language = 'por') {
  const response = await fetch(`/api/v1/books/${bookId}/pdf/ocr?language=${encodeURIComponent(language)}`, {
    method: 'POST',
    body: blob,
    credentials: 'same-origin',
    headers: { 'Content-Type': blob.type || 'image/png', Accept: 'application/json' },
  })
  if (!response.ok) throw await readApiError(response)
  return (await response.json()) as OCRResult
}

export function newReadingSessionID(prefix: string) {
  const secure = globalThis.crypto
  const token = typeof secure?.randomUUID === 'function'
    ? secure.randomUUID()
    : secure?.getRandomValues
      ? Array.from(secure.getRandomValues(new Uint32Array(4)), (value) => value.toString(16).padStart(8, '0')).join('')
      : `${Date.now().toString(36)}-${Math.random().toString(36).slice(2)}`
  return `${prefix}-${token}`.slice(0, 128)
}
