import { useCallback, useEffect, useMemo, useRef, useState, type CSSProperties, type PointerEvent as ReactPointerEvent } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { GlobalWorkerOptions, TextLayer, getDocument, type PDFDocumentProxy, type PDFPageProxy } from 'pdfjs-dist'
import pdfWorkerURL from 'pdfjs-dist/build/pdf.worker.min.mjs?url'
import { analyzePDFDocument, classifyPDFDocument, recognizePDFPageOnDemand, type PDFAnalysisProgress } from './analyzePdf'
import {
  annotationsExportURL,
  api,
  newReadingSessionID,
  pdfFileURL,
  type Annotation,
  type AnnotationColor,
  type AnnotationRect,
  type Book,
  type BooksResponse,
  type PDFSearchResult,
} from '../../lib/api'
import {
  AlertIcon,
  ArrowLeftIcon,
  ArrowRightIcon,
  BackIcon,
  CheckIcon,
  DownloadIcon,
  FullscreenExitIcon,
  FullscreenIcon,
  SearchIcon,
  SettingsIcon,
  TrashIcon,
} from '../../components/Icons'

GlobalWorkerOptions.workerSrc = pdfWorkerURL

type SelectionDraft = {
  kind: 'highlight' | 'area'
  text: string
  rects: AnnotationRect[]
  prefix?: string
  suffix?: string
  clientX: number
  clientY: number
}

type AreaDrag = { startX: number; startY: number; currentX: number; currentY: number }

type ReaderTapGesture = {
  touchIdentifier: number
  startX: number
  startY: number
  startedAt: number
  startScrollLeft: number
  startScrollTop: number
  suppress: boolean
  selectionChanged: boolean
}

const readerTapMoveTolerance = 12
const readerTapMaximumDuration = 450
const readerDoubleTapDelay = 320
const readerDoubleTapDistance = 28

const colors: AnnotationColor[] = ['yellow', 'green', 'blue', 'pink', 'orange']
const colorLabels: Record<AnnotationColor, string> = {
  yellow: 'Yellow', green: 'Green', blue: 'Blue', pink: 'Pink', orange: 'Orange',
}

export function PdfReader({ book, onClose }: { book: Book; onClose: () => void }) {
  const queryClient = useQueryClient()
  const rootRef = useRef<HTMLDivElement>(null)
  const stageRef = useRef<HTMLDivElement>(null)
  const surfaceRef = useRef<HTMLDivElement>(null)
  const canvasRef = useRef<HTMLCanvasElement>(null)
  const textLayerRef = useRef<HTMLDivElement>(null)
  const textLayerInstanceRef = useRef<TextLayer | null>(null)
  const renderTaskRef = useRef<{ cancel: () => void; promise: Promise<unknown> } | null>(null)
  const renderSequenceRef = useRef(0)
  const sessionIDRef = useRef(newReadingSessionID(`pdf-${book.id}`))
  const sessionRecordedRef = useRef(false)
  const analysisStartedRef = useRef(false)
  const passwordUpdateRef = useRef<((password: string) => void) | null>(null)
  const saveTimerRef = useRef<number | null>(null)
  const selectionFrameRef = useRef<number | null>(null)
  const tapGestureRef = useRef<ReaderTapGesture | null>(null)
  const activeTouchIdentifiersRef = useRef(new Set<number>())
  const pendingTapTimerRef = useRef<number | null>(null)
  const recentTapRef = useRef<{ x: number; y: number; at: number } | null>(null)
  const lastSavedPageRef = useRef(book.started ? book.current_page : -1)
  const pageTextRef = useRef('')

  const [pdfDocument, setPDFDocument] = useState<PDFDocumentProxy | null>(null)
  const [pageCount, setPageCount] = useState(Math.max(1, book.page_count))
  const [pageNumber, setPageNumber] = useState(Math.max(0, book.current_page))
  const [rendering, setRendering] = useState(true)
  const [loadError, setLoadError] = useState('')
  const [controlsVisible, setControlsVisible] = useState(true)
  const [fullscreen, setFullscreen] = useState(false)
  const [annotationsOpen, setAnnotationsOpen] = useState(false)
  const [searchOpen, setSearchOpen] = useState(false)
  const [searchQuery, setSearchQuery] = useState('')
  const [activeSearchQuery, setActiveSearchQuery] = useState('')
  const [analysisProgress, setAnalysisProgress] = useState<PDFAnalysisProgress | null>(null)
  const [passwordPrompt, setPasswordPrompt] = useState<'required' | 'incorrect' | null>(null)
  const [passwordValue, setPasswordValue] = useState('')
  const [areaMode, setAreaMode] = useState(false)
  const [pageHasText, setPageHasText] = useState(true)
  const [selectionDraft, setSelectionDraft] = useState<SelectionDraft | null>(null)
  const [liveSelectionRects, setLiveSelectionRects] = useState<AnnotationRect[]>([])
  const [selectedColor, setSelectedColor] = useState<AnnotationColor>('yellow')
  const [selectedAnnotation, setSelectedAnnotation] = useState<Annotation | null>(null)
  const [annotationNote, setAnnotationNote] = useState('')
  const [areaDrag, setAreaDrag] = useState<AreaDrag | null>(null)
  const [saveState, setSaveState] = useState<'idle' | 'saving' | 'saved' | 'error'>('idle')
  const [stageSize, setStageSize] = useState({ width: window.innerWidth, height: window.innerHeight })
  const [pageOCRState, setPageOCRState] = useState<'idle' | 'running' | 'done' | 'error'>('idle')

  const annotationsQuery = useQuery({
    queryKey: ['annotations', book.id],
    queryFn: () => api.annotations(book.id),
  })
  const ocrStatusQuery = useQuery({
    queryKey: ['ocr-status'],
    queryFn: api.ocrStatus,
    staleTime: 60_000,
  })
  const annotations = annotationsQuery.data?.items ?? []
  const pageAnnotations = useMemo(
    () => annotations.filter((annotation) => annotation.page_number === pageNumber),
    [annotations, pageNumber],
  )

  const normalizedSearch = searchQuery.trim()
  const searchResultsQuery = useQuery({
    queryKey: ['pdf-search', book.id, normalizedSearch],
    queryFn: () => api.searchPDF(book.id, normalizedSearch),
    enabled: searchOpen && normalizedSearch.length >= 2 && book.pdf_indexed_pages > 0,
    staleTime: 30_000,
  })
  const searchResults = searchResultsQuery.data?.items ?? []
  const recognizeCurrentPage = useMutation({
    mutationFn: async () => {
      if (!pdfDocument) throw new Error('PDF unavailable')
      setPageOCRState('running')
      return recognizePDFPageOnDemand(pdfDocument, book.id, pageNumber, book.language)
    },
    onSuccess: () => {
      setPageOCRState('done')
      void queryClient.invalidateQueries({ queryKey: ['book', book.id] })
      void queryClient.invalidateQueries({ queryKey: ['pdf-search', book.id] })
      window.setTimeout(() => setPageOCRState('idle'), 2200)
    },
    onError: () => setPageOCRState('error'),
  })

  useEffect(() => {
    let cancelled = false
    const task = getDocument({ url: pdfFileURL(book.id), withCredentials: true })
    task.onPassword = (updatePassword: (password: string) => void, reason: number) => {
      passwordUpdateRef.current = updatePassword
      setPasswordPrompt(reason === 2 ? 'incorrect' : 'required')
      setPasswordValue('')
      setControlsVisible(true)
    }
    setRendering(true)
    setLoadError('')
    task.promise.then(async (loaded) => {
      if (cancelled) return
      passwordUpdateRef.current = null
      setPasswordPrompt(null)
      setPDFDocument(loaded)
      setPageCount(loaded.numPages)
      setPageNumber((current) => clamp(current, 0, loaded.numPages - 1))
      if (loaded.numPages !== book.page_count) {
        try {
          await api.updatePDFMetadata(book.id, loaded.numPages)
          queryClient.setQueryData<Book>(['book', book.id], (current) => current ? { ...current, page_count: loaded.numPages } : current)
          void queryClient.invalidateQueries({ queryKey: ['books'] })
          void queryClient.invalidateQueries({ queryKey: ['dashboard'] })
        } catch {
          // The PDF can still be read. Metadata synchronization is retried next time.
        }
      }
    }).catch((error) => {
      if (!cancelled) {
        setRendering(false)
        setLoadError(error instanceof Error ? error.message : 'Could not parse the PDF.')
      }
    })
    return () => {
      cancelled = true
      passwordUpdateRef.current = null
      void task.destroy()
    }
  }, [book.id, book.page_count, queryClient])

  useEffect(() => {
    if (!pdfDocument || analysisStartedRef.current) return
    const needsClassification = book.pdf_classification_status !== 'complete'
    const needsNativeIndex = book.pdf_document_kind === 'text' && book.pdf_analysis_status !== 'complete'
    const needsRequestedOCR = (book.pdf_ocr_mode === 'background' || book.pdf_ocr_mode === 'full')
      && book.pdf_analysis_status !== 'complete'
    if (!needsClassification && !needsNativeIndex && !needsRequestedOCR) return

    analysisStartedRef.current = true
    let cancelled = false
    const timer = window.setTimeout(() => {
      void (async () => {
        let documentKind = book.pdf_document_kind
        let ocrMode = book.pdf_ocr_mode
        let analysisStatus = book.pdf_analysis_status

        if (needsClassification) {
          setAnalysisProgress({ current: 0, total: pdfDocument.numPages, percent: 1, label: 'Classifying the PDF without blocking reading' })
          const classification = await classifyPDFDocument(pdfDocument, book.id, setAnalysisProgress)
          if (cancelled) return
          documentKind = classification.document_kind
          ocrMode = book.pdf_ocr_mode === 'auto' ? classification.recommended_ocr_mode : book.pdf_ocr_mode
          analysisStatus = 'classified'
          const indexedPages = classification.pages.filter((page) => page.source !== 'none' && page.text).length
          const ocrPages = classification.pages.filter((page) => page.source === 'ocr').length
          queryClient.setQueryData<Book>(['book', book.id], (current) => current ? {
            ...current,
            page_count: pdfDocument.numPages,
            pdf_analysis_status: 'classified',
            pdf_classification_status: 'complete',
            pdf_document_kind: classification.document_kind,
            pdf_text_layer: classification.text_layer,
            pdf_ocr_mode: ocrMode,
            pdf_ocr_status: ocrMode === 'off' ? (classification.document_kind === 'text' ? 'not_needed' : 'disabled') : 'idle',
            pdf_sampled_pages: classification.sampled_pages,
            pdf_indexed_pages: indexedPages,
            pdf_ocr_pages: ocrPages,
          } : current)
        }

        const shouldIndexNative = documentKind === 'text' && analysisStatus !== 'complete'
        const shouldRunOCR = (ocrMode === 'background' || ocrMode === 'full') && analysisStatus !== 'complete'
        if (shouldIndexNative || shouldRunOCR) {
          const analysis = await analyzePDFDocument(pdfDocument, book.id, setAnalysisProgress, {
            allowOCR: shouldRunOCR,
            skipCover: true,
          })
          if (cancelled) return
          const invalidPages = analysis.pages.filter((page) => page.source === 'none').length
          const ocrPages = analysis.pages.filter((page) => page.source === 'ocr').length
          const indexedPages = analysis.pages.length - invalidPages
          queryClient.setQueryData<Book>(['book', book.id], (current) => current ? {
            ...current,
            page_count: pdfDocument.numPages,
            pdf_analysis_status: 'complete',
            pdf_classification_status: 'complete',
            pdf_text_layer: analysis.text_layer,
            pdf_ocr_status: indexedPages >= pdfDocument.numPages ? 'complete' : ocrPages > 0 ? 'partial' : invalidPages > 0 ? 'needed' : 'not_needed',
            pdf_invalid_pages: invalidPages,
            pdf_ocr_pages: ocrPages,
            pdf_indexed_pages: indexedPages,
          } : current)
        }

        void queryClient.invalidateQueries({ queryKey: ['books'] })
        void queryClient.invalidateQueries({ queryKey: ['dashboard'] })
        window.setTimeout(() => !cancelled && setAnalysisProgress(null), 2200)
      })().catch(() => {
        if (!cancelled) setAnalysisProgress({ current: 0, total: pdfDocument.numPages, percent: 100, label: 'Could not prepare search for this PDF' })
      })
    }, 700)
    return () => {
      cancelled = true
      window.clearTimeout(timer)
    }
  }, [book.id, pdfDocument, queryClient])

  useEffect(() => {
    const stage = stageRef.current
    if (!stage) return
    const observer = new ResizeObserver(([entry]) => {
      if (!entry) return
      setStageSize({ width: entry.contentRect.width, height: entry.contentRect.height })
    })
    observer.observe(stage)
    return () => observer.disconnect()
  }, [])

  useEffect(() => {
    if (!pdfDocument) return
    let cancelled = false
    const sequence = ++renderSequenceRef.current
    renderTaskRef.current?.cancel()
    setRendering(true)
    setLoadError('')
    setSelectionDraft(null)
    setLiveSelectionRects([])
    window.getSelection()?.removeAllRanges()

    void (async () => {
      try {
        const page = await pdfDocument.getPage(pageNumber + 1)
        if (cancelled || sequence !== renderSequenceRef.current) return
        await renderPDFPage(page, stageSize, canvasRef.current, surfaceRef.current, textLayerRef.current, textLayerInstanceRef, renderTaskRef)
        if (cancelled || sequence !== renderSequenceRef.current) return
        const strings = textLayerInstanceRef.current?.textContentItemsStr ?? []
        const pageText = normalizeExtractedText(strings.join(' '))
        pageTextRef.current = pageText
        setPageHasText(Boolean(pageText))
        highlightTextLayerMatches(textLayerRef.current, activeSearchQuery)
        setRendering(false)
        void preloadPDFPages(pdfDocument, pageNumber, pageCount)
      } catch (error) {
        if (cancelled || sequence !== renderSequenceRef.current || isRenderCancellation(error)) return
        setRendering(false)
        setLoadError(error instanceof Error ? error.message : 'Could not render this page.')
      }
    })()

    return () => {
      cancelled = true
      renderTaskRef.current?.cancel()
      textLayerInstanceRef.current?.cancel()
    }
  }, [activeSearchQuery, pdfDocument, pageCount, pageNumber, stageSize])

  useEffect(() => {
    function scheduleSelectionPreview() {
      if (selectionFrameRef.current !== null) window.cancelAnimationFrame(selectionFrameRef.current)
      selectionFrameRef.current = window.requestAnimationFrame(() => {
        selectionFrameRef.current = null
        if (areaMode) {
          setLiveSelectionRects([])
          return
        }
        const surface = surfaceRef.current
        const textLayer = textLayerRef.current
        const selection = window.getSelection()
        if (!surface || !textLayer || !selection || selection.isCollapsed || !selection.rangeCount) {
          setLiveSelectionRects([])
          return
        }
        const range = selection.getRangeAt(0)
        if (!selectionIntersectsLayer(range, textLayer)) {
          setLiveSelectionRects([])
          return
        }
        setLiveSelectionRects(selectionRectsFromRange(range, surface.getBoundingClientRect(), textLayer))
      })
    }

    function finishSelecting() {
      textLayerRef.current?.classList.remove('selecting')
    }

    document.addEventListener('selectionchange', scheduleSelectionPreview)
    window.addEventListener('pointerup', finishSelecting)
    window.addEventListener('pointercancel', finishSelecting)
    window.addEventListener('touchend', finishSelecting)
    window.addEventListener('touchcancel', finishSelecting)
    window.addEventListener('blur', finishSelecting)
    return () => {
      document.removeEventListener('selectionchange', scheduleSelectionPreview)
      window.removeEventListener('pointerup', finishSelecting)
      window.removeEventListener('pointercancel', finishSelecting)
      window.removeEventListener('touchend', finishSelecting)
      window.removeEventListener('touchcancel', finishSelecting)
      window.removeEventListener('blur', finishSelecting)
      if (selectionFrameRef.current !== null) window.cancelAnimationFrame(selectionFrameRef.current)
    }
  }, [areaMode, pageNumber])

  useEffect(() => () => { void api.endReadingSession(book.id, sessionIDRef.current) }, [])

  useEffect(() => () => {
    if (pendingTapTimerRef.current !== null) window.clearTimeout(pendingTapTimerRef.current)
  }, [])

  const saveProgress = useCallback(async (nextPage: number) => {
    if (nextPage === lastSavedPageRef.current && sessionRecordedRef.current) return
    setSaveState('saving')
    try {
      const progress = await api.saveProgress(book.id, nextPage, undefined, sessionIDRef.current)
      lastSavedPageRef.current = nextPage
      sessionRecordedRef.current = true
      setSaveState('saved')
      queryClient.setQueryData<Book>(['book', book.id], (current) => current ? {
        ...current,
        current_page: progress.current_page,
        started: progress.started,
        completed: progress.completed,
      } : current)
      queryClient.setQueriesData<BooksResponse>({ queryKey: ['books'] }, (current) => current ? {
        ...current,
        items: current.items.map((item) => item.id === book.id ? {
          ...item,
          current_page: progress.current_page,
          started: progress.started,
          completed: progress.completed,
        } : item),
      } : current)
      window.setTimeout(() => setSaveState((value) => value === 'saved' ? 'idle' : value), 1400)
    } catch {
      setSaveState('error')
    }
  }, [book.id, queryClient])

  useEffect(() => {
    if (!pdfDocument) return
    if (saveTimerRef.current !== null) window.clearTimeout(saveTimerRef.current)
    saveTimerRef.current = window.setTimeout(() => void saveProgress(pageNumber), 400)
    return () => {
      if (saveTimerRef.current !== null) window.clearTimeout(saveTimerRef.current)
    }
  }, [pdfDocument, pageNumber, saveProgress])

  useEffect(() => {
    if (!controlsVisible || annotationsOpen || searchOpen || selectedAnnotation || selectionDraft || passwordPrompt) return
    const timer = window.setTimeout(() => setControlsVisible(false), 3600)
    return () => window.clearTimeout(timer)
  }, [annotationsOpen, controlsVisible, pageNumber, passwordPrompt, searchOpen, selectedAnnotation, selectionDraft])

  const previous = useCallback(() => setPageNumber((value) => Math.max(0, value - 1)), [])
  const next = useCallback(() => setPageNumber((value) => Math.min(pageCount - 1, value + 1)), [pageCount])

  useEffect(() => {
    function onKeyDown(event: KeyboardEvent) {
      if (event.target instanceof HTMLInputElement || event.target instanceof HTMLTextAreaElement) return
      if (event.key === 'ArrowLeft' || event.key === 'PageUp') {
        event.preventDefault(); previous()
      } else if (event.key === 'ArrowRight' || event.key === 'PageDown' || event.key === ' ') {
        event.preventDefault(); next()
      } else if (event.key.toLowerCase() === 'f') {
        event.preventDefault(); void toggleFullscreen(rootRef.current)
      } else if (event.key === 'Escape') {
        if (selectedAnnotation) setSelectedAnnotation(null)
        else if (searchOpen) setSearchOpen(false)
        else if (annotationsOpen) setAnnotationsOpen(false)
        else if (document.fullscreenElement) void document.exitFullscreen()
        else onClose()
      }
    }
    window.addEventListener('keydown', onKeyDown)
    return () => window.removeEventListener('keydown', onKeyDown)
  }, [annotationsOpen, next, onClose, previous, searchOpen, selectedAnnotation])

  useEffect(() => {
    const update = () => setFullscreen(document.fullscreenElement === rootRef.current)
    document.addEventListener('fullscreenchange', update)
    return () => document.removeEventListener('fullscreenchange', update)
  }, [])

  const createAnnotation = useMutation({
    mutationFn: (input: { draft: SelectionDraft; noteAfter: boolean; paragraph?: boolean }) => {
      const draft = input.paragraph ? expandDraftToParagraph(input.draft, surfaceRef.current, textLayerRef.current) : input.draft
      return api.createAnnotation(book.id, {
        page_number: pageNumber,
        kind: draft.kind,
        color: selectedColor,
        selected_text: draft.text,
        note: '',
        anchor: { rects: draft.rects, prefix: draft.prefix, suffix: draft.suffix },
      }).then((annotation) => ({ annotation, noteAfter: input.noteAfter }))
    },
    onSuccess: ({ annotation, noteAfter }) => {
      queryClient.setQueryData<{ items: Annotation[] }>(['annotations', book.id], (current) => ({ items: [...(current?.items ?? []), annotation] }))
      setSelectionDraft(null)
      setLiveSelectionRects([])
      window.getSelection()?.removeAllRanges()
      if (noteAfter) {
        setSelectedAnnotation(annotation)
        setAnnotationNote(annotation.note)
      }
    },
  })

  const updateAnnotation = useMutation({
    mutationFn: (input: { id: number; color: AnnotationColor; note: string }) => api.updateAnnotation(input.id, { color: input.color, note: input.note }),
    onSuccess: (updated) => {
      queryClient.setQueryData<{ items: Annotation[] }>(['annotations', book.id], (current) => ({
        items: (current?.items ?? []).map((item) => item.id === updated.id ? updated : item),
      }))
      setSelectedAnnotation(updated)
      setAnnotationNote(updated.note)
    },
  })

  const deleteAnnotation = useMutation({
    mutationFn: (id: number) => api.deleteAnnotation(id).then(() => id),
    onSuccess: (id) => {
      queryClient.setQueryData<{ items: Annotation[] }>(['annotations', book.id], (current) => ({ items: (current?.items ?? []).filter((item) => item.id !== id) }))
      setSelectedAnnotation(null)
    },
  })

  function captureSelection() {
    if (areaMode) return
    window.setTimeout(() => {
      const surface = surfaceRef.current
      const textLayer = textLayerRef.current
      const selection = window.getSelection()
      if (!surface || !textLayer || !selection || selection.isCollapsed || !selection.rangeCount) return
      const range = selection.getRangeAt(0)
      if (!selectionIntersectsLayer(range, textLayer)) return
      const text = normalizeExtractedText(selection.toString())
      if (!text) return
      const rects = selectionRectsFromRange(range, surface.getBoundingClientRect(), textLayer)
      if (!rects.length) return
      const fullText = pageTextRef.current
      const index = fullText.toLocaleLowerCase().indexOf(text.toLocaleLowerCase())
      setSelectionDraft({
        kind: 'highlight', text, rects,
        prefix: index >= 0 ? fullText.slice(Math.max(0, index - 120), index) : '',
        suffix: index >= 0 ? fullText.slice(index + text.length, index + text.length + 120) : '',
        clientX: Math.min(window.innerWidth - 170, Math.max(170, normalizedRectsToClientCenter(rects, surface.getBoundingClientRect()).x)),
        clientY: Math.max(82, normalizedRectsToClientCenter(rects, surface.getBoundingClientRect()).y - 18),
      })
      setControlsVisible(true)
    }, 0)
  }

  function startArea(event: ReactPointerEvent<HTMLDivElement>) {
    if (!areaMode) {
      if (event.pointerType !== 'touch' && event.button === 0) textLayerRef.current?.classList.add('selecting')
      return
    }
    if (event.button !== 0) return
    const surface = surfaceRef.current
    if (!surface) return
    event.currentTarget.setPointerCapture(event.pointerId)
    const point = pointerWithinSurface(event, surface)
    setAreaDrag({ startX: point.x, startY: point.y, currentX: point.x, currentY: point.y })
    setSelectionDraft(null)
  }

  function moveArea(event: ReactPointerEvent<HTMLDivElement>) {
    if (!areaMode || !areaDrag || !surfaceRef.current) return
    const point = pointerWithinSurface(event, surfaceRef.current)
    setAreaDrag((current) => current ? { ...current, currentX: point.x, currentY: point.y } : current)
  }

  function finishArea(event: ReactPointerEvent<HTMLDivElement>) {
    if (!areaMode || !areaDrag || !surfaceRef.current) return
    const rect = areaDragToRect(areaDrag)
    setAreaDrag(null)
    if (rect.width < 0.015 || rect.height < 0.015) return
    setSelectionDraft({ kind: 'area', text: '', rects: [rect], clientX: event.clientX, clientY: event.clientY })
    setControlsVisible(true)
  }

  function cancelPendingReaderTap() {
    if (pendingTapTimerRef.current === null) return
    window.clearTimeout(pendingTapTimerRef.current)
    pendingTapTimerRef.current = null
  }

  useEffect(() => {
    const stage = stageRef.current
    if (!stage) return

    function clearGesture() {
      tapGestureRef.current = null
    }

    function rememberActiveTouches(touches: TouchList) {
      const active = activeTouchIdentifiersRef.current
      active.clear()
      for (let index = 0; index < touches.length; index += 1) {
        const touch = touches.item(index)
        if (touch) active.add(touch.identifier)
      }
    }

    function touchByIdentifier(touches: TouchList, identifier: number) {
      for (let index = 0; index < touches.length; index += 1) {
        const touch = touches.item(index)
        if (touch?.identifier === identifier) return touch
      }
      return null
    }

    function startReaderTouch(event: TouchEvent) {
      rememberActiveTouches(event.touches)
      if (event.touches.length !== 1) {
        clearGesture()
        cancelPendingReaderTap()
        return
      }

      const touch = event.touches.item(0)
      if (!touch) return

      const now = performance.now()
      const recentTap = recentTapRef.current
      const isSecondTap = recentTap !== null
        && now - recentTap.at <= readerDoubleTapDelay + 80
        && Math.hypot(touch.clientX - recentTap.x, touch.clientY - recentTap.y) <= readerDoubleTapDistance

      if (isSecondTap) {
        cancelPendingReaderTap()
        recentTapRef.current = null
      }

      if (readerTapIsUnavailable() || isReaderTapBlockedTarget(event.target) || hasPDFTextSelection(textLayerRef.current)) {
        clearGesture()
        return
      }

      tapGestureRef.current = {
        touchIdentifier: touch.identifier,
        startX: touch.clientX,
        startY: touch.clientY,
        startedAt: now,
        startScrollLeft: stage.scrollLeft,
        startScrollTop: stage.scrollTop,
        suppress: isSecondTap,
        selectionChanged: false,
      }
    }

    function moveReaderTouch(event: TouchEvent) {
      rememberActiveTouches(event.touches)
      const gesture = tapGestureRef.current
      if (!gesture) return
      if (event.touches.length !== 1) {
        clearGesture()
        cancelPendingReaderTap()
        return
      }

      const touch = touchByIdentifier(event.touches, gesture.touchIdentifier)
      if (!touch
        || Math.hypot(touch.clientX - gesture.startX, touch.clientY - gesture.startY) > readerTapMoveTolerance
        || Math.abs(stage.scrollLeft - gesture.startScrollLeft) > 2
        || Math.abs(stage.scrollTop - gesture.startScrollTop) > 2) {
        clearGesture()
      }
    }

    function finishReaderTouch(event: TouchEvent) {
      rememberActiveTouches(event.touches)
      const gesture = tapGestureRef.current
      if (!gesture) return

      const touch = touchByIdentifier(event.changedTouches, gesture.touchIdentifier)
      if (!touch) return
      clearGesture()

      const now = performance.now()
      if (gesture.suppress
        || now - gesture.startedAt > readerTapMaximumDuration
        || gesture.selectionChanged
        || isReaderTapBlockedTarget(event.target)
        || readerTapIsUnavailable()
        || Math.abs(stage.scrollLeft - gesture.startScrollLeft) > 2
        || Math.abs(stage.scrollTop - gesture.startScrollTop) > 2) return

      recentTapRef.current = { x: touch.clientX, y: touch.clientY, at: now }
      const clientX = touch.clientX
      cancelPendingReaderTap()
      pendingTapTimerRef.current = window.setTimeout(() => {
        pendingTapTimerRef.current = null
        if (activeTouchIdentifiersRef.current.size > 0
          || readerTapIsUnavailable()
          || hasPDFTextSelection(textLayerRef.current)
          || Math.abs(stage.scrollLeft - gesture.startScrollLeft) > 2
          || Math.abs(stage.scrollTop - gesture.startScrollTop) > 2) return

        const bounds = stage.getBoundingClientRect()
        const position = clamp((clientX - bounds.left) / Math.max(1, bounds.width), 0, 1)
        if (position < 1 / 3) previous()
        else if (position > 2 / 3) next()
        else setControlsVisible((value) => !value)
      }, readerDoubleTapDelay)
    }

    function cancelReaderTouch(event: TouchEvent) {
      rememberActiveTouches(event.touches)
      clearGesture()
      cancelPendingReaderTap()
    }

    function markReaderTouchSelection() {
      const gesture = tapGestureRef.current
      if (gesture && hasPDFTextSelection(textLayerRef.current)) gesture.selectionChanged = true
    }

    const listenerOptions: AddEventListenerOptions = { capture: true, passive: true }
    stage.addEventListener('touchstart', startReaderTouch, listenerOptions)
    stage.addEventListener('touchmove', moveReaderTouch, listenerOptions)
    stage.addEventListener('touchend', finishReaderTouch, listenerOptions)
    stage.addEventListener('touchcancel', cancelReaderTouch, listenerOptions)
    document.addEventListener('selectionchange', markReaderTouchSelection)

    return () => {
      stage.removeEventListener('touchstart', startReaderTouch, listenerOptions)
      stage.removeEventListener('touchmove', moveReaderTouch, listenerOptions)
      stage.removeEventListener('touchend', finishReaderTouch, listenerOptions)
      stage.removeEventListener('touchcancel', cancelReaderTouch, listenerOptions)
      document.removeEventListener('selectionchange', markReaderTouchSelection)
      activeTouchIdentifiersRef.current.clear()
      clearGesture()
      cancelPendingReaderTap()
    }
  }, [annotationsOpen, areaMode, loadError, next, passwordPrompt, previous, rendering, searchOpen, selectedAnnotation, selectionDraft])

  function readerTapIsUnavailable() {
    return areaMode || rendering || Boolean(loadError || passwordPrompt || selectionDraft || selectedAnnotation || searchOpen || annotationsOpen)
  }

  function openAnnotation(annotation: Annotation) {
    setSelectedAnnotation(annotation)
    setAnnotationNote(annotation.note)
    setSelectedColor(annotation.color)
    setControlsVisible(true)
  }

  const areaDraftRect = areaDrag ? areaDragToRect(areaDrag) : null
  const directionRTL = book.reading_direction === 'rtl'
  const leftAction = directionRTL ? next : previous
  const rightAction = directionRTL ? previous : next

  return (
    <div className={`pdf-reader${controlsVisible ? ' pdf-reader-controls-visible' : ''}${areaMode ? ' pdf-reader-area-mode' : ''}`} ref={rootRef}>
      <header className="pdf-reader-topbar">
        <button className="reader-icon-button" onClick={onClose} aria-label="Back"><BackIcon /></button>
        <div className="pdf-reader-title"><strong>{book.title}</strong><span>PDF · page {pageNumber + 1} of {pageCount}</span></div>
        <span className={`reader-save-state reader-save-${saveState}`} aria-live="polite">{saveState === 'saving' ? 'Saving…' : saveState === 'saved' ? <><CheckIcon />Saved</> : saveState === 'error' ? 'Not saved' : null}</span>
        {analysisProgress ? <span className="pdf-analysis-status" title={analysisProgress.label}>{analysisProgress.percent}%</span> : null}
        <button className="reader-icon-button" onClick={() => { setSearchOpen((value) => !value); setAnnotationsOpen(false); setControlsVisible(true) }} title="Search this book"><SearchIcon /></button>
        <button className="reader-icon-button pdf-notes-button" onClick={() => { setAnnotationsOpen((value) => !value); setSearchOpen(false) }} title="Highlights and notes"><SettingsIcon /><span>{annotations.length}</span></button>
        <button className="reader-icon-button" onClick={() => void toggleFullscreen(rootRef.current)} aria-label={fullscreen ? 'Exit full screen' : 'Full screen'}>{fullscreen ? <FullscreenExitIcon /> : <FullscreenIcon />}</button>
      </header>

      <main
        className="pdf-reader-stage"
        ref={stageRef}
        onClick={(event) => {
          if (event.target === event.currentTarget) setControlsVisible((value) => !value)
        }}
      >
        <button className="pdf-reader-zone pdf-reader-zone-left" onClick={leftAction} disabled={directionRTL ? pageNumber >= pageCount - 1 : pageNumber <= 0} aria-label={directionRTL ? 'Next page' : 'Previous page'}><ArrowLeftIcon /></button>
        <button className="pdf-reader-zone pdf-reader-zone-right" onClick={rightAction} disabled={directionRTL ? pageNumber <= 0 : pageNumber >= pageCount - 1} aria-label={directionRTL ? 'Previous page' : 'Next page'}><ArrowRightIcon /></button>

        <div
          className="pdf-page-surface"
          ref={surfaceRef}
          onPointerDown={startArea}
          onPointerMove={moveArea}
          onPointerUp={finishArea}
          onMouseUp={captureSelection}
          onTouchEnd={captureSelection}
        >
          <canvas ref={canvasRef} className="pdf-page-canvas" />
          <div ref={textLayerRef} className="textLayer pdf-text-layer" />
          <div className="pdf-highlight-layer" aria-label="Highlights on this page">
            {(selectionDraft?.kind === 'highlight' ? selectionDraft.rects : liveSelectionRects).map((rect, index) => (
              <span className="pdf-selection-preview" key={`selection-preview-${index}`} style={rectStyle(rect)} />
            ))}
            {pageAnnotations.map((annotation) => mergeNormalizedRects(annotation.anchor.rects).map((rect, index) => (
              <button
                aria-label={index === 0 ? `Open highlight: ${annotation.selected_text || 'area mark'}` : undefined}
                className={`pdf-highlight pdf-highlight-${annotation.color}${selectedAnnotation?.id === annotation.id ? ' pdf-highlight-selected' : ''}`}
                key={`${annotation.id}-${index}`}
                onClick={(event) => { event.stopPropagation(); openAnnotation(annotation) }}
                style={rectStyle(rect)}
                tabIndex={index === 0 ? 0 : -1}
                title={annotation.note || annotation.selected_text || 'Area mark'}
              />
            )))}
            {areaDraftRect ? <span className={`pdf-area-draft pdf-highlight-${selectedColor}`} style={rectStyle(areaDraftRect)} /> : null}
          </div>
        </div>

        {rendering ? <div className="pdf-reader-loading"><span className="spinner" /><strong>Preparing page {pageNumber + 1}…</strong></div> : null}
        {loadError ? <div className="pdf-reader-error"><AlertIcon /><strong>Could not open the page.</strong><p>{loadError}</p><button className="button button-secondary" onClick={() => setStageSize((size) => ({ ...size }))}>Try again</button></div> : null}
      </main>

      <footer className="pdf-reader-bottombar">
        <button className={`button button-secondary pdf-area-toggle${areaMode ? ' pdf-area-toggle-active' : ''}`} onClick={() => { setAreaMode((value) => !value); window.getSelection()?.removeAllRanges(); setSelectionDraft(null); setLiveSelectionRects([]) }}>
          {areaMode ? 'Finish marking' : pageHasText ? 'Mark area' : 'Mark page area'}
        </button>
        <input aria-label="Go to page" min={1} max={pageCount} type="range" value={pageNumber + 1} onChange={(event) => setPageNumber(Number(event.target.value) - 1)} />
        <span>{pageNumber + 1} / {pageCount}</span>
      </footer>

      {selectionDraft ? (
        <div className="pdf-selection-toolbar" style={{ left: selectionDraft.clientX, top: selectionDraft.clientY }}>
          <div className="pdf-color-picker" aria-label="Highlight color">
            {colors.map((color) => <button key={color} className={`annotation-color annotation-color-${color}${selectedColor === color ? ' annotation-color-active' : ''}`} onClick={() => setSelectedColor(color)} aria-label={colorLabels[color]} />)}
          </div>
          <button onClick={() => createAnnotation.mutate({ draft: selectionDraft, noteAfter: false })}>{selectionDraft.kind === 'area' ? 'Save area' : 'Highlight'}</button>
          {selectionDraft.kind === 'highlight' ? <button onClick={() => createAnnotation.mutate({ draft: selectionDraft, noteAfter: false, paragraph: true })}>Paragraph</button> : null}
          <button onClick={() => createAnnotation.mutate({ draft: selectionDraft, noteAfter: true })}>+ Note</button>
          <button className="pdf-toolbar-close" onClick={() => { setSelectionDraft(null); setLiveSelectionRects([]); window.getSelection()?.removeAllRanges() }}>×</button>
        </div>
      ) : null}

      {searchOpen ? (
        <aside className="pdf-search-panel">
          <header><div><span>Search this book</span><strong>{normalizedSearch.length >= 2 ? `${searchResults.length} pages` : 'Type to search'}</strong></div><button className="reader-icon-button" onClick={() => setSearchOpen(false)}>×</button></header>
          <label className="pdf-search-input"><SearchIcon /><input autoFocus value={searchQuery} onChange={(event) => setSearchQuery(event.target.value)} placeholder="Word or phrase…" /></label>
          {book.pdf_classification_status !== 'complete' ? <p className="pdf-search-message">{analysisProgress?.label || 'samrai is identifying this PDF type.'}</p> : null}
          {book.pdf_classification_status === 'complete' && book.pdf_indexed_pages === 0 ? (
            <p className="pdf-search-message">
              {book.pdf_document_kind === 'comic'
                ? 'This PDF was identified as manga or a comic. Automatic OCR is off to avoid unnecessary processing.'
                : book.pdf_ocr_mode === 'off'
                  ? 'Text recognition is off for this PDF.'
                  : 'No pages have been recognized yet. You can process only the current page.'}
            </p>
          ) : null}
          {ocrStatusQuery.data?.available && book.pdf_document_kind !== 'text' ? (
            <button
              className="button button-secondary button-full pdf-ocr-current-button"
              disabled={!pdfDocument || recognizeCurrentPage.isPending}
              onClick={() => recognizeCurrentPage.mutate()}
              type="button"
            >
              {pageOCRState === 'running' ? <span className="spinner spinner-small" /> : <SearchIcon />}
              {pageOCRState === 'running' ? `Recognizing page ${pageNumber + 1}…` : pageOCRState === 'done' ? 'Page added to search' : 'Recognize current page'}
            </button>
          ) : null}
          {pageOCRState === 'error' ? <p className="pdf-search-message pdf-search-error">Could not recognize text on this page.</p> : null}
          {!ocrStatusQuery.data?.available && book.pdf_document_kind !== 'text' ? <p className="pdf-search-message pdf-search-error">{ocrStatusQuery.data?.message || 'OCR is unavailable in this installation.'}</p> : null}
          {searchResultsQuery.isFetching ? <p className="pdf-search-message">Searching…</p> : null}
          {searchResultsQuery.isError ? <p className="pdf-search-message pdf-search-error">Search is unavailable right now.</p> : null}
          <div className="pdf-search-results">
            {normalizedSearch.length >= 2 && book.pdf_indexed_pages > 0 && !searchResultsQuery.isFetching && !searchResults.length ? <p>No results found in indexed pages.</p> : null}
            {searchResults.map((result: PDFSearchResult) => (
              <button key={result.page_number} onClick={() => { setActiveSearchQuery(normalizedSearch); setPageNumber(result.page_number); setSearchOpen(false); setControlsVisible(true) }}>
                <span><small>Page {result.page_number + 1} · {result.matches} match{result.matches === 1 ? '' : 's'}{result.text_source === 'ocr' ? ' · OCR' : ''}{result.match_type === 'repaired' ? ' · reconstructed word' : ''}</small><strong>{result.excerpt}</strong></span>
              </button>
            ))}
          </div>
        </aside>
      ) : null}

      {annotationsOpen ? (
        <aside className="pdf-annotations-panel">
          <header><div><span>Highlights and notes</span><strong>{annotations.length} items</strong></div><button className="reader-icon-button" onClick={() => setAnnotationsOpen(false)}>×</button></header>
          <a className="button button-secondary button-full" href={annotationsExportURL(book.id)}><DownloadIcon />Export Markdown</a>
          <div className="pdf-annotations-list">
            {annotations.length ? annotations.map((annotation) => (
              <button key={annotation.id} onClick={() => { setPageNumber(annotation.page_number); openAnnotation(annotation); setAnnotationsOpen(false) }}>
                <i className={`annotation-color annotation-color-${annotation.color}`} />
                <span><small>Page {annotation.page_number + 1}</small><strong>{annotation.selected_text || 'Area mark'}</strong>{annotation.note ? <em>{annotation.note}</em> : null}</span>
              </button>
            )) : <p>Select text to create your first highlight.</p>}
          </div>
        </aside>
      ) : null}

      {passwordPrompt ? (
        <div className="pdf-note-backdrop">
          <form className="pdf-password-dialog" onSubmit={(event) => {
            event.preventDefault()
            const updatePassword = passwordUpdateRef.current
            if (!updatePassword || !passwordValue) return
            setPasswordPrompt(null)
            updatePassword(passwordValue)
          }}>
            <header><div><small>Protected PDF</small><strong>{passwordPrompt === 'incorrect' ? 'Incorrect password' : 'Enter the document password'}</strong></div><button className="reader-icon-button" type="button" onClick={onClose}>×</button></header>
            <p>The password is used only for this session and is never stored by samrai.</p>
            <input autoFocus type="password" value={passwordValue} onChange={(event) => setPasswordValue(event.target.value)} autoComplete="off" />
            <button className="button button-primary button-full" disabled={!passwordValue} type="submit">Open PDF</button>
          </form>
        </div>
      ) : null}

      {selectedAnnotation ? (
        <div className="pdf-note-backdrop" onMouseDown={(event) => event.target === event.currentTarget && setSelectedAnnotation(null)}>
          <section className="pdf-note-editor" role="dialog" aria-modal="true" aria-label="Edit highlight">
            <header><div><small>Page {selectedAnnotation.page_number + 1}</small><strong>{selectedAnnotation.selected_text || 'Area mark'}</strong></div><button className="reader-icon-button" onClick={() => setSelectedAnnotation(null)}>×</button></header>
            <div className="pdf-color-picker">{colors.map((color) => <button key={color} className={`annotation-color annotation-color-${color}${selectedColor === color ? ' annotation-color-active' : ''}`} onClick={() => setSelectedColor(color)} aria-label={colorLabels[color]} />)}</div>
            <label><span>Note</span><textarea autoFocus rows={6} placeholder="Write a note about this passage…" value={annotationNote} onChange={(event) => setAnnotationNote(event.target.value)} /></label>
            {updateAnnotation.error || deleteAnnotation.error ? <p className="form-error">Could not save the change.</p> : null}
            <footer><button className="button button-secondary button-danger" onClick={() => window.confirm('Delete this highlight?') && deleteAnnotation.mutate(selectedAnnotation.id)}><TrashIcon />Delete</button><button className="button button-primary" onClick={() => updateAnnotation.mutate({ id: selectedAnnotation.id, color: selectedColor, note: annotationNote })}>Save note</button></footer>
          </section>
        </div>
      ) : null}
    </div>
  )
}

async function renderPDFPage(
  page: PDFPageProxy,
  stageSize: { width: number; height: number },
  canvas: HTMLCanvasElement | null,
  surface: HTMLDivElement | null,
  textLayerContainer: HTMLDivElement | null,
  textLayerInstanceRef: { current: TextLayer | null },
  renderTaskRef: { current: { cancel: () => void; promise: Promise<unknown> } | null },
) {
  if (!canvas || !surface || !textLayerContainer) return
  const base = page.getViewport({ scale: 1 })
  const mobileEdgeToEdge = window.matchMedia('(max-width: 720px)').matches
  const availableWidth = Math.max(260, stageSize.width - (mobileEdgeToEdge ? 0 : 42))
  const availableHeight = Math.max(320, stageSize.height - (mobileEdgeToEdge ? 0 : 34))
  // On phones, fixed pages follow the same edge-to-edge visual language as
  // reflowable EPUBs. Width is the primary constraint; tall pages may scroll
  // vertically instead of shrinking and creating thick side gutters.
  const scale = Math.max(0.2, mobileEdgeToEdge
    ? availableWidth / base.width
    : Math.min(availableWidth / base.width, availableHeight / base.height))
  const viewport = page.getViewport({ scale })
  const outputScale = Math.min(2, window.devicePixelRatio || 1)

  surface.style.width = `${viewport.width}px`
  surface.style.height = `${viewport.height}px`
  surface.style.setProperty('--total-scale-factor', `${scale}`)
  surface.style.setProperty('--scale-round-x', '1px')
  surface.style.setProperty('--scale-round-y', '1px')
  canvas.width = Math.floor(viewport.width * outputScale)
  canvas.height = Math.floor(viewport.height * outputScale)
  canvas.style.width = `${viewport.width}px`
  canvas.style.height = `${viewport.height}px`

  const renderTask = page.render({
    canvas,
    viewport,
    transform: outputScale === 1 ? undefined : [outputScale, 0, 0, outputScale, 0, 0],
  })
  renderTaskRef.current = renderTask
  await renderTask.promise
  if (renderTaskRef.current === renderTask) renderTaskRef.current = null

  textLayerInstanceRef.current?.cancel()
  textLayerContainer.replaceChildren()
  const textLayer = new TextLayer({
    textContentSource: page.streamTextContent({ includeMarkedContent: true, disableNormalization: true }),
    container: textLayerContainer,
    viewport,
  })
  textLayerInstanceRef.current = textLayer
  await textLayer.render()
  const endOfContent = document.createElement('div')
  endOfContent.className = 'endOfContent'
  textLayerContainer.append(endOfContent)
}

async function preloadPDFPages(document: PDFDocumentProxy, currentPage: number, pageCount: number) {
  for (const candidate of [currentPage + 2, currentPage]) {
    if (candidate < 0 || candidate >= pageCount) continue
    try { await document.getPage(candidate + 1) } catch { /* best effort */ }
  }
}

function normalizeRect(rect: DOMRect, surface: DOMRect): AnnotationRect {
  return {
    x: clamp((rect.left - surface.left) / surface.width, 0, 1),
    y: clamp((rect.top - surface.top) / surface.height, 0, 1),
    width: clamp(rect.width / surface.width, 0, 1),
    height: clamp(rect.height / surface.height, 0, 1),
  }
}

function validNormalizedRect(rect: AnnotationRect) {
  return rect.width > 0.001 && rect.height > 0.001 && rect.x >= 0 && rect.y >= 0 && rect.x + rect.width <= 1.01 && rect.y + rect.height <= 1.01
}

function rectStyle(rect: AnnotationRect): CSSProperties {
  return { left: `${rect.x * 100}%`, top: `${rect.y * 100}%`, width: `${rect.width * 100}%`, height: `${rect.height * 100}%` }
}

function normalizedRectsToClientCenter(rects: AnnotationRect[], surface: DOMRect) {
  if (!rects.length) return { x: window.innerWidth / 2, y: 120 }
  const left = Math.min(...rects.map((rect) => surface.left + rect.x * surface.width))
  const right = Math.max(...rects.map((rect) => surface.left + (rect.x + rect.width) * surface.width))
  const top = Math.min(...rects.map((rect) => surface.top + rect.y * surface.height))
  return { x: (left + right) / 2, y: top }
}

function selectionIntersectsLayer(range: Range, textLayer: HTMLDivElement) {
  try {
    return textLayer.contains(range.startContainer) || textLayer.contains(range.endContainer) || range.intersectsNode(textLayer)
  } catch {
    return false
  }
}

function selectionRectsFromRange(range: Range, surface: DOMRect, textLayer: HTMLDivElement): AnnotationRect[] {
  const clientRects = dedupeClientRects(Array.from(range.getClientRects())
    .map((rect) => clipClientRect(rect, surface))
    .filter((rect): rect is DOMRect => Boolean(rect && rect.width > 0.75 && rect.height > 1)))
  const lineGuides = buildTextLineGuides(textLayer, surface)
  return mergeClientRects(clientRects, lineGuides)
    .map((rect) => normalizeRect(rect, surface))
    .filter(validNormalizedRect)
}

function clipClientRect(rect: DOMRect, surface: DOMRect) {
  const left = Math.max(rect.left, surface.left)
  const top = Math.max(rect.top, surface.top)
  const right = Math.min(rect.right, surface.right)
  const bottom = Math.min(rect.bottom, surface.bottom)
  if (right <= left || bottom <= top) return null
  return new DOMRect(left, top, right - left, bottom - top)
}

type TextLineGuide = {
  top: number
  bottom: number
  center: number
  height: number
  left: number
  right: number
}

type ClientRectLine = TextLineGuide & {
  rects: DOMRect[]
}

function buildTextLineGuides(textLayer: HTMLDivElement, surface: DOMRect): TextLineGuide[] {
  const spanRects = dedupeClientRects(Array.from(textLayer.querySelectorAll<HTMLSpanElement>('span'))
    .filter((span) => span.getAttribute('role') !== 'img' && normalizeExtractedText(span.textContent ?? ''))
    .map((span) => clipClientRect(span.getBoundingClientRect(), surface))
    .filter((rect): rect is DOMRect => Boolean(rect && rect.width > 0.75 && rect.height > 1)))
  if (!spanRects.length) return []

  return groupRectsIntoLines(spanRects)
    .flatMap((line) => splitLineIntoSegments(line, surface.width))
    .map((segment) => lineGuideFromRects(segment))
    .sort((a, b) => Math.abs(a.center - b.center) <= 1 ? a.left - b.left : a.center - b.center)
}

function lineGuideFromRects(rects: DOMRect[]): TextLineGuide {
  const centers = rects.map((rect) => rect.top + rect.height / 2)
  const heights = rects.map((rect) => rect.height)
  const center = median(centers)
  const height = median(heights)
  return {
    top: center - height / 2,
    bottom: center + height / 2,
    center,
    height,
    left: Math.min(...rects.map((rect) => rect.left)),
    right: Math.max(...rects.map((rect) => rect.right)),
  }
}

function dedupeClientRects(rects: DOMRect[]): DOMRect[] {
  const result: DOMRect[] = []
  const sorted = [...rects].sort((a, b) => a.top === b.top ? a.left - b.left : a.top - b.top)
  for (const rect of sorted) {
    const duplicate = result.some((existing) => (
      Math.abs(existing.left - rect.left) <= 0.75
      && Math.abs(existing.top - rect.top) <= 0.75
      && Math.abs(existing.width - rect.width) <= 1
      && Math.abs(existing.height - rect.height) <= 1
    ))
    if (!duplicate) result.push(rect)
  }
  return result
}

function groupRectsIntoLines(rects: DOMRect[]): DOMRect[][] {
  const sorted = [...rects].sort((a, b) => {
    const centerA = a.top + a.height / 2
    const centerB = b.top + b.height / 2
    return Math.abs(centerA - centerB) <= 1 ? a.left - b.left : centerA - centerB
  })
  const lines: DOMRect[][] = []

  for (const rect of sorted) {
    const centerY = rect.top + rect.height / 2
    let bestLine: DOMRect[] | undefined
    let bestDistance = Number.POSITIVE_INFINITY

    for (const line of lines) {
      const centers = line.map((item) => item.top + item.height / 2)
      const heights = line.map((item) => item.height)
      const lineCenter = median(centers)
      const lineHeight = median(heights)
      const distance = Math.abs(centerY - lineCenter)
      const overlap = Math.max(0, Math.min(Math.max(...line.map((item) => item.bottom)), rect.bottom) - Math.max(Math.min(...line.map((item) => item.top)), rect.top))
      const overlapRatio = overlap / Math.max(1, Math.min(lineHeight, rect.height))
      const tolerance = Math.max(3, Math.max(lineHeight, rect.height) * 0.72)
      if ((overlapRatio >= 0.22 || distance <= tolerance) && distance < bestDistance) {
        bestLine = line
        bestDistance = distance
      }
    }

    if (bestLine) bestLine.push(rect)
    else lines.push([rect])
  }

  return lines.sort((a, b) => median(a.map((rect) => rect.top + rect.height / 2)) - median(b.map((rect) => rect.top + rect.height / 2)))
}

function splitLineIntoSegments(rects: DOMRect[], surfaceWidth: number): DOMRect[][] {
  if (rects.length < 2) return [rects]
  const ordered = [...rects].sort((a, b) => a.left - b.left)
  const lineHeight = Math.max(1, median(ordered.map((rect) => rect.height)))
  // Justified prose can contain unusually large word spaces. Only a much larger
  // discontinuity is considered a separate column or text block.
  const columnGap = Math.max(lineHeight * 4.8, surfaceWidth * 0.065)
  const segments: DOMRect[][] = [[ordered[0]!]]

  for (const rect of ordered.slice(1)) {
    const current = segments[segments.length - 1]!
    const right = Math.max(...current.map((item) => item.right))
    if (rect.left - right > columnGap) segments.push([rect])
    else current.push(rect)
  }
  return segments
}

function mergeClientRects(rects: DOMRect[], guides: TextLineGuide[] = []): DOMRect[] {
  if (!rects.length) return []
  const lines: ClientRectLine[] = guides.map((guide) => ({ rects: [], ...guide }))
  const fallbackRects: DOMRect[] = []

  for (const rect of rects) {
    const centerY = rect.top + rect.height / 2
    let best: ClientRectLine | undefined
    let bestScore = Number.POSITIVE_INFINITY

    for (const line of lines) {
      const verticalDistance = Math.abs(centerY - line.center)
      const verticalOverlap = Math.max(0, Math.min(line.bottom, rect.bottom) - Math.max(line.top, rect.top))
      const overlapRatio = verticalOverlap / Math.max(1, Math.min(line.height, rect.height))
      const verticalTolerance = Math.max(3, Math.max(line.height, rect.height) * 0.82)
      if (overlapRatio < 0.16 && verticalDistance > verticalTolerance) continue

      const horizontalGap = rect.right < line.left
        ? line.left - rect.right
        : rect.left > line.right
          ? rect.left - line.right
          : 0
      const horizontalTolerance = Math.max(line.height * 2.5, 18)
      if (horizontalGap > horizontalTolerance) continue

      const score = verticalDistance + horizontalGap * 0.15
      if (score < bestScore) {
        best = line
        bestScore = score
      }
    }

    if (best) best.rects.push(rect)
    else fallbackRects.push(rect)
  }

  const fallbackWidth = rects.length
    ? Math.max(...rects.map((rect) => rect.right)) - Math.min(...rects.map((rect) => rect.left))
    : 0
  for (const fallbackLine of groupRectsIntoLines(fallbackRects)) {
    for (const segment of splitLineIntoSegments(fallbackLine, Math.max(1, fallbackWidth))) {
      lines.push({ rects: segment, ...lineGuideFromRects(segment) })
    }
  }

  const activeLines = lines
    .filter((line) => line.rects.length)
    .sort((a, b) => Math.abs(a.center - b.center) <= 1 ? a.left - b.left : a.center - b.center)

  return activeLines.map((line, index) => {
    const selectedLeft = Math.min(...line.rects.map((rect) => rect.left))
    const selectedRight = Math.max(...line.rects.map((rect) => rect.right))
    const referenceHeight = Math.max(1, Math.min(line.height, median(line.rects.map((rect) => rect.height))))
    const bandHeight = Math.max(2, referenceHeight * 0.80)
    const bandTop = line.center - bandHeight / 2 + bandHeight * 0.07

    let left = selectedLeft
    let right = selectedRight

    // A DOM Range is continuous. For a multi-line range, every intermediate
    // line is completely selected; only the first and last lines may be partial.
    // Snapping those edges to the PDF text-line guide avoids browser-specific
    // whitespace rectangles while preserving the user's real start and end.
    if (guides.length && activeLines.length > 1) {
      if (index > 0) left = line.left
      if (index < activeLines.length - 1) right = line.right
    }

    const edgeSnap = referenceHeight * 1.15
    if (Math.abs(left - line.left) <= edgeSnap) left = line.left
    if (Math.abs(right - line.right) <= edgeSnap) right = line.right

    left = Math.max(line.left, left)
    right = Math.min(line.right, right)
    return new DOMRect(left - 0.6, bandTop, Math.max(1, right - left + 1.2), bandHeight)
  })
}

function mergeNormalizedRects(rects: AnnotationRect[]): AnnotationRect[] {
  if (rects.length < 2) return rects
  const virtualSurface = new DOMRect(0, 0, 10_000, 10_000)
  const clientRects = rects.map((rect) => new DOMRect(
    rect.x * virtualSurface.width,
    rect.y * virtualSurface.height,
    rect.width * virtualSurface.width,
    rect.height * virtualSurface.height,
  ))
  return mergeClientRects(clientRects)
    .map((rect) => normalizeRect(rect, virtualSurface))
    .filter(validNormalizedRect)
}

function normalizeExtractedText(value: string) {
  return value.replace(/\s+/g, ' ').trim()
}

function expandDraftToParagraph(draft: SelectionDraft, surface: HTMLDivElement | null, textLayer: HTMLDivElement | null): SelectionDraft {
  if (draft.kind !== 'highlight' || !surface || !textLayer) return draft
  const surfaceRect = surface.getBoundingClientRect()
  const spans = Array.from(textLayer.querySelectorAll('span'))
    .map((span) => ({ text: normalizeExtractedText(span.textContent ?? ''), rect: span.getBoundingClientRect() }))
    .filter((item) => item.text && item.rect.width > 0 && item.rect.height > 0)
    .sort((a, b) => Math.abs(a.rect.top - b.rect.top) < 4 ? a.rect.left - b.rect.left : a.rect.top - b.rect.top)
  if (!spans.length) return draft

  const selectedTop = Math.min(...draft.rects.map((rect) => rect.y)) * surfaceRect.height + surfaceRect.top
  const selectedBottom = Math.max(...draft.rects.map((rect) => rect.y + rect.height)) * surfaceRect.height + surfaceRect.top
  const lineTolerance = Math.max(3, median(spans.map((item) => item.rect.height)) * 0.45)
  const lines: { top: number; bottom: number; left: number; right: number; text: string[] }[] = []
  for (const item of spans) {
    let line = lines.find((candidate) => Math.abs(candidate.top - item.rect.top) <= lineTolerance)
    if (!line) {
      line = { top: item.rect.top, bottom: item.rect.bottom, left: item.rect.left, right: item.rect.right, text: [] }
      lines.push(line)
    }
    line.top = Math.min(line.top, item.rect.top); line.bottom = Math.max(line.bottom, item.rect.bottom)
    line.left = Math.min(line.left, item.rect.left); line.right = Math.max(line.right, item.rect.right)
    line.text.push(item.text)
  }
  lines.sort((a, b) => a.top - b.top)
  let first = lines.findIndex((line) => line.bottom >= selectedTop && line.top <= selectedBottom)
  if (first < 0) return draft
  let last = first
  while (last + 1 < lines.length && lines[last + 1]!.top <= selectedBottom) last++
  const medianHeight = median(lines.map((line) => line.bottom - line.top))
  const ordinaryGaps = lines.slice(1)
    .map((line, index) => Math.max(0, line.top - lines[index]!.bottom))
    .filter((gap) => gap <= medianHeight)
  const ordinaryGap = median(ordinaryGaps)
  const paragraphGapLimit = Math.min(medianHeight * 0.9, Math.max(4, medianHeight * 0.35, ordinaryGap * 2.2))
  while (first > 0 && lines[first]!.top - lines[first - 1]!.bottom <= paragraphGapLimit) first--
  while (last + 1 < lines.length && lines[last + 1]!.top - lines[last]!.bottom <= paragraphGapLimit) last++
  const paragraph = lines.slice(first, last + 1)
  return {
    ...draft,
    text: paragraph.map((line) => line.text.join(' ')).join('\n'),
    rects: paragraph.map((line) => normalizeRect(new DOMRect(line.left, line.top, line.right - line.left, line.bottom - line.top), surfaceRect)).filter(validNormalizedRect),
  }
}

function median(values: number[]) {
  if (!values.length) return 12
  const sorted = [...values].sort((a, b) => a - b)
  return sorted[Math.floor(sorted.length / 2)] ?? 12
}

function pointerWithinSurface(event: ReactPointerEvent, surface: HTMLDivElement) {
  const rect = surface.getBoundingClientRect()
  return { x: clamp((event.clientX - rect.left) / rect.width, 0, 1), y: clamp((event.clientY - rect.top) / rect.height, 0, 1) }
}

function isReaderTapBlockedTarget(target: EventTarget | null) {
  if (!(target instanceof Element)) return false
  return Boolean(target.closest([
    'a[href]',
    'button',
    'input',
    'select',
    'textarea',
    'label',
    'summary',
    '[contenteditable]:not([contenteditable="false"])',
    '[role="button"]',
    '[role="link"]',
    '[role="checkbox"]',
    '[role="menuitem"]',
    '[role="option"]',
    '[role="radio"]',
    '[role="slider"]',
    '[role="switch"]',
    '[role="tab"]',
    '[role="textbox"]',
    '[data-reader-tap-ignore]',
  ].join(',')))
}

function hasPDFTextSelection(textLayer: HTMLDivElement | null) {
  const selection = window.getSelection()
  if (!selection || selection.isCollapsed || !selection.rangeCount) return false
  if (!textLayer) return true
  return selectionIntersectsLayer(selection.getRangeAt(0), textLayer)
}

function areaDragToRect(drag: AreaDrag): AnnotationRect {
  return {
    x: Math.min(drag.startX, drag.currentX), y: Math.min(drag.startY, drag.currentY),
    width: Math.abs(drag.currentX - drag.startX), height: Math.abs(drag.currentY - drag.startY),
  }
}

function highlightTextLayerMatches(textLayer: HTMLDivElement | null, query: string) {
  if (!textLayer) return
  const spans = Array.from(textLayer.querySelectorAll<HTMLSpanElement>('span'))
    .filter((span) => normalizeExtractedText(span.textContent ?? ''))
  spans.forEach((span) => span.classList.remove('pdf-search-hit'))
  const normalizedQuery = normalizeExtractedText(query).toLocaleLowerCase()
  if (normalizedQuery.length < 2 || !spans.length) return

  const ranges: { span: HTMLSpanElement; start: number; end: number }[] = []
  let combined = ''
  for (const span of spans) {
    const value = normalizeExtractedText(span.textContent ?? '')
    if (!value) continue
    if (combined) combined += ' '
    const start = combined.length
    combined += value
    ranges.push({ span, start, end: combined.length })
  }
  const lower = combined.toLocaleLowerCase()
  let offset = 0
  while (offset < lower.length) {
    const index = lower.indexOf(normalizedQuery, offset)
    if (index < 0) break
    const end = index + normalizedQuery.length
    ranges.forEach((range) => {
      if (range.end > index && range.start < end) range.span.classList.add('pdf-search-hit')
    })
    offset = Math.max(end, index + 1)
  }
}

function isRenderCancellation(error: unknown) {
  return error instanceof Error && (error.name === 'RenderingCancelledException' || /cancel/i.test(error.message))
}

async function toggleFullscreen(element: HTMLElement | null) {
  if (!element) return
  if (document.fullscreenElement) await document.exitFullscreen()
  else await element.requestFullscreen()
}

function clamp(value: number, minimum: number, maximum: number) {
  return Math.min(maximum, Math.max(minimum, value))
}
