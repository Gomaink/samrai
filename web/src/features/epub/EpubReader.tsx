import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  api,
  epubAnnotationsExportURL,
  newReadingSessionID,
  type AnnotationColor,
  type Book,
  type BooksResponse,
  type EPUBAnnotation,
  type EPUBAnnotationAnchor,
  type EPUBPublication,
  type EPUBReadingLocation,
} from '../../lib/api'
import {
  AlertIcon,
  ArrowLeftIcon,
  ArrowRightIcon,
  BackIcon,
  CheckIcon,
  DirectionIcon,
  DownloadIcon,
  FullscreenExitIcon,
  FullscreenIcon,
  SearchIcon,
  SettingsIcon,
  TrashIcon,
} from '../../components/Icons'

type ReaderTheme = 'paper' | 'white' | 'dark' | 'oled'
type ReaderWidth = 'compact' | 'comfortable' | 'wide'
type ReaderFont = 'publisher' | 'serif' | 'sans'
type ReaderAlignment = 'justify' | 'left'
type SidePanel = 'toc' | 'search' | 'annotations' | 'settings' | null

type FrameState = {
  columnCount: number
  viewportWidth: number
  pageWidth: number
  textWidth: number
}


type EPUBSelectionDraft = {
  text: string
  anchor: EPUBAnnotationAnchor
  clientX: number
  clientY: number
}

const annotationColors: AnnotationColor[] = ['yellow', 'green', 'blue', 'pink', 'orange']
const annotationColorLabels: Record<AnnotationColor, string> = {
  yellow: 'Yellow', green: 'Green', blue: 'Blue', pink: 'Pink', orange: 'Orange',
}

const themeKey = 'samrai.epub.theme'
const fontSizeKey = 'samrai.epub.font-size'
const lineHeightKey = 'samrai.epub.line-height'
const marginKey = 'samrai.epub.margin'
const widthKey = 'samrai.epub.width'
const fontKey = 'samrai.epub.font'
const alignmentKey = 'samrai.epub.alignment'

export function EpubReader({ book, onClose }: { book: Book; onClose: () => void }) {
  const queryClient = useQueryClient()
  const readerRef = useRef<HTMLDivElement>(null)
  const iframeRef = useRef<HTMLIFrameElement>(null)
  const saveTimerRef = useRef<number | null>(null)
  const sessionIDRef = useRef(newReadingSessionID(`epub-${book.id}`))
  const frameStateRef = useRef<FrameState>({ columnCount: 1, viewportWidth: 1, pageWidth: 1, textWidth: 1 })
  const columnIndexRef = useRef(0)
  const pendingProgressRef = useRef<number | null>(null)
  const frameCleanupRef = useRef<(() => void) | null>(null)
  const initializedRef = useRef(false)
  const highlightFrameRef = useRef<number | null>(null)
  const [spineIndex, setSpineIndex] = useState(() => clamp(book.current_page, 0, Math.max(0, book.page_count - 1)))
  const [columnIndex, setColumnIndex] = useState(0)
  const [columnCount, setColumnCount] = useState(1)
  const [controlsVisible, setControlsVisible] = useState(true)
  const [panel, setPanel] = useState<SidePanel>(null)
  const [fullscreen, setFullscreen] = useState(false)
  const [frameReady, setFrameReady] = useState(false)
  const [frameError, setFrameError] = useState('')
  const [saveState, setSaveState] = useState<'idle' | 'saving' | 'saved' | 'error'>('idle')
  const [theme, setTheme] = useState<ReaderTheme>(() => readTheme())
  const [fontSize, setFontSize] = useState(() => readNumber(fontSizeKey, 19, 13, 34))
  const [lineHeight, setLineHeight] = useState(() => readNumber(lineHeightKey, 1.65, 1.2, 2.2))
  const [margin, setMargin] = useState(() => readNumber(marginKey, 48, 18, 96))
  const [readerWidth, setReaderWidth] = useState<ReaderWidth>(() => readReaderWidth())
  const [readerFont, setReaderFont] = useState<ReaderFont>(() => readReaderFont())
  const [alignment, setAlignment] = useState<ReaderAlignment>(() => readReaderAlignment())
  const [search, setSearch] = useState('')
  const [pendingFragment, setPendingFragment] = useState('')
  const [frameReloadNonce, setFrameReloadNonce] = useState(0)
  const [selectionDraft, setSelectionDraft] = useState<EPUBSelectionDraft | null>(null)
  const [selectedColor, setSelectedColor] = useState<AnnotationColor>('yellow')
  const [selectionNote, setSelectionNote] = useState('')
  const [selectionNoteOpen, setSelectionNoteOpen] = useState(false)
  const [annotationSearch, setAnnotationSearch] = useState('')
  const [annotationColorFilter, setAnnotationColorFilter] = useState<AnnotationColor | 'all'>('all')
  const [editingAnnotationID, setEditingAnnotationID] = useState<number | null>(null)
  const [editingAnnotationNote, setEditingAnnotationNote] = useState('')
  const [pendingAnnotationID, setPendingAnnotationID] = useState<number | null>(null)
  const [pendingSearchQuery, setPendingSearchQuery] = useState('')
  const [activeSearchQuery, setActiveSearchQuery] = useState('')

  const publicationQuery = useQuery({
    queryKey: ['epub', book.id],
    queryFn: () => api.epub(book.id),
    staleTime: 5 * 60_000,
  })
  const publication = publicationQuery.data
  const currentItem = publication?.spine[spineIndex]
  const isFixed = publication?.layout === 'fixed'
  const direction = publication?.reading_direction === 'rtl' ? 'rtl' : 'ltr'

  const searchMutation = useMutation({ mutationFn: (query: string) => api.searchEPUB(book.id, query) })
  const annotationsQuery = useQuery({
    queryKey: ['epub-annotations', book.id],
    queryFn: () => api.epubAnnotations(book.id),
  })
  const annotations = annotationsQuery.data?.items ?? []
  const currentAnnotations = useMemo(() => annotations.filter((item) => item.spine_index === spineIndex), [annotations, spineIndex])
  const filteredAnnotations = useMemo(() => {
    const query = annotationSearch.trim().toLocaleLowerCase()
    return annotations.filter((item) => {
      if (annotationColorFilter !== 'all' && item.color !== annotationColorFilter) return false
      if (!query) return true
      return `${item.selected_text} ${item.note}`.toLocaleLowerCase().includes(query)
    })
  }, [annotationColorFilter, annotationSearch, annotations])

  const createEPUBAnnotation = useMutation({
    mutationFn: (input: { draft: EPUBSelectionDraft; note: string }) => api.createEPUBAnnotation(book.id, {
      spine_index: spineIndex,
      resource_path: currentItem?.path ?? '',
      kind: 'highlight',
      color: selectedColor,
      selected_text: input.draft.text,
      note: input.note,
      anchor: input.draft.anchor,
    }),
    onSuccess: async () => {
      clearEPUBSelection(iframeRef.current?.contentWindow)
      setSelectionDraft(null)
      setSelectionNote('')
      setSelectionNoteOpen(false)
      await queryClient.invalidateQueries({ queryKey: ['epub-annotations', book.id] })
    },
  })
  const updateEPUBAnnotation = useMutation({
    mutationFn: (input: { id: number; color?: AnnotationColor; note?: string }) => api.updateEPUBAnnotation(input.id, { color: input.color, note: input.note }),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ['epub-annotations', book.id] })
    },
  })
  const deleteEPUBAnnotation = useMutation({
    mutationFn: (id: number) => api.deleteEPUBAnnotation(id),
    onSuccess: async () => {
      setEditingAnnotationID(null)
      await queryClient.invalidateQueries({ queryKey: ['epub-annotations', book.id] })
    },
  })

  useEffect(() => {
    columnIndexRef.current = columnIndex
  }, [columnIndex])

  useEffect(() => {
    if (!publication || initializedRef.current) return
    initializedRef.current = true
    const saved = parseSavedLocation(book.reading_location)
    const initialSpine = saved && saved.spine_index >= 0 && saved.spine_index < publication.spine.length
      ? saved.spine_index
      : clamp(book.current_page, 0, publication.spine.length - 1)
    setSpineIndex(initialSpine)
    const sameSpine = saved?.spine_index === initialSpine
    pendingProgressRef.current = sameSpine ? clamp(saved?.chapter_progress ?? 0, 0, 1) : 0
    setColumnIndex(sameSpine ? Math.max(0, saved?.column_index ?? 0) : 0)
  }, [book.current_page, book.reading_location, publication])

  const applyCurrentColumn = useCallback((requestedColumn?: number) => {
    const frame = iframeRef.current
    const win = frame?.contentWindow
    if (!frame || !win || isFixed) return
    const width = Math.max(1, frameStateRef.current.viewportWidth)
    const targetColumn = requestedColumn ?? columnIndexRef.current
    const safeColumn = clamp(targetColumn, 0, Math.max(0, frameStateRef.current.columnCount - 1))
    win.scrollTo({ left: safeColumn * width, top: 0, behavior: 'instant' as ScrollBehavior })
    columnIndexRef.current = safeColumn
    setColumnIndex(safeColumn)
  }, [isFixed])

  const navigateToSpine = useCallback((nextIndex: number, nextColumn = 0, fragment = '') => {
    if (!publication || nextIndex < 0 || nextIndex >= publication.spine.length) return
    setSelectionDraft(null)
    setSelectionNote('')
    setSelectionNoteOpen(false)
    setActiveSearchQuery('')

    if (nextIndex === spineIndex) {
      const doc = iframeRef.current?.contentDocument
      const win = iframeRef.current?.contentWindow
      if (fragment && doc && win) {
        const target = doc.getElementById(fragment) || doc.querySelector(`[name="${cssEscape(fragment)}"]`)
        if (target instanceof Element) {
          const column = clamp(Math.floor((target.getBoundingClientRect().left + win.scrollX) / Math.max(1, frameStateRef.current.viewportWidth)), 0, Math.max(0, frameStateRef.current.columnCount - 1))
          applyCurrentColumn(column)
        }
      } else if (nextColumn === Number.MAX_SAFE_INTEGER) {
        applyCurrentColumn(frameStateRef.current.columnCount - 1)
      } else {
        applyCurrentColumn(nextColumn)
      }
      return
    }

    setFrameReady(false)
    setFrameError('')
    setPendingFragment(fragment)
    pendingProgressRef.current = nextColumn === Number.MAX_SAFE_INTEGER ? 1 : 0
    setSpineIndex(nextIndex)
    columnIndexRef.current = Math.max(0, nextColumn)
    setColumnIndex(Math.max(0, nextColumn))
    setColumnCount(1)
  }, [applyCurrentColumn, publication, spineIndex])

  const nextPage = useCallback(() => {
    if (!publication) return
    const currentColumn = columnIndexRef.current
    const currentCount = frameStateRef.current.columnCount
    if (!isFixed && currentColumn < currentCount - 1) {
      applyCurrentColumn(currentColumn + 1)
      return
    }
    navigateToSpine(spineIndex + 1, 0)
  }, [applyCurrentColumn, isFixed, navigateToSpine, publication, spineIndex])

  const previousPage = useCallback(() => {
    if (!publication) return
    const currentColumn = columnIndexRef.current
    if (!isFixed && currentColumn > 0) {
      applyCurrentColumn(currentColumn - 1)
      return
    }
    const previousIndex = spineIndex - 1
    if (previousIndex < 0) return
    navigateToSpine(previousIndex, Number.MAX_SAFE_INTEGER)
  }, [applyCurrentColumn, isFixed, navigateToSpine, publication, spineIndex])

  const leftAction = direction === 'rtl' ? nextPage : previousPage
  const rightAction = direction === 'rtl' ? previousPage : nextPage

  const configureFrame = useCallback(() => {
    const frame = iframeRef.current
    const doc = frame?.contentDocument
    const win = frame?.contentWindow
    if (!frame || !doc || !win || !publication) return

    try {
      setFrameError('')
      doc.documentElement.dataset.samrai = 'true'
      removeDangerousDocumentFeatures(doc)
      const previousState = frameStateRef.current
      const previousProgress = previousState.columnCount <= 1
        ? 0
        : clamp(columnIndexRef.current / Math.max(1, previousState.columnCount - 1), 0, 1)
      let requestedColumn = columnIndexRef.current
      if (publication.layout === 'fixed') {
        configureFixedDocument(frame, doc, theme)
        frameStateRef.current = { columnCount: 1, viewportWidth: Math.max(1, frame.clientWidth), pageWidth: Math.max(1, frame.clientWidth), textWidth: Math.max(1, frame.clientWidth) }
        setColumnCount(1)
        columnIndexRef.current = 0
        setColumnIndex(0)
      } else {
        const state = configureReflowableDocument(frame, doc, { theme, fontSize, lineHeight, margin, readerWidth, readerFont, alignment })
        frameStateRef.current = state
        setColumnCount(state.columnCount)
        const pendingProgress = pendingProgressRef.current
        if (pendingProgress !== null) {
          requestedColumn = Math.round(clamp(pendingProgress, 0, 1) * Math.max(0, state.columnCount - 1))
          pendingProgressRef.current = null
        } else {
          requestedColumn = Math.round(previousProgress * Math.max(0, state.columnCount - 1))
        }
        if (columnIndexRef.current === Number.MAX_SAFE_INTEGER) requestedColumn = state.columnCount - 1
        if (pendingFragment) {
          const target = doc.getElementById(pendingFragment) || doc.querySelector(`[name="${cssEscape(pendingFragment)}"]`)
          if (target instanceof Element) {
            requestedColumn = clamp(Math.floor((target.getBoundingClientRect().left + win.scrollX) / Math.max(1, state.viewportWidth)), 0, state.columnCount - 1)
          }
          setPendingFragment('')
        }
        window.requestAnimationFrame(() => applyCurrentColumn(requestedColumn))
      }

      frameCleanupRef.current?.()
      let suppressClickUntil = 0
      let pendingTapTimer: number | null = null
      let touchGesture: {
        identifier: number
        startX: number
        startY: number
        startedAt: number
        target: EventTarget | null
        moved: boolean
        selectionAtStart: boolean
      } | null = null

      const clearPendingTap = () => {
        if (pendingTapTimer !== null) {
          window.clearTimeout(pendingTapTimer)
          pendingTapTimer = null
        }
      }
      const selectionHandler = () => {
        window.setTimeout(() => {
          const draft = createEPUBSelectionDraft(frame, doc, win)
          if (!draft) return
          clearPendingTap()
          setSelectionDraft(draft)
          setSelectionNote('')
          setSelectionNoteOpen(false)
          setControlsVisible(true)
        }, 0)
      }
      const runTapAction = (clientX: number) => {
        const selection = win.getSelection()
        if (selection && !selection.isCollapsed) return
        setSelectionDraft(null)
        const ratio = clientX / Math.max(1, frame.clientWidth)
        if (ratio < 0.28) leftAction()
        else if (ratio > 0.72) rightAction()
        else setControlsVisible((value) => !value)
      }
      const clickHandler = (event: MouseEvent) => {
        if (window.performance.now() < suppressClickUntil) return
        const selection = win.getSelection()
        if (selection && !selection.isCollapsed) return
        const target = event.target as Element | null
        const anchor = target?.closest?.('a[href]') as HTMLAnchorElement | null
        if (anchor) {
          event.preventDefault()
          handleEPUBLink(anchor.href, publication, spineIndex, navigateToSpine)
          return
        }
        runTapAction(event.clientX)
      }
      const touchStartHandler = (event: TouchEvent) => {
        clearPendingTap()
        if (event.touches.length !== 1 || event.changedTouches.length !== 1) {
          touchGesture = null
          return
        }
        const touch = event.changedTouches.item(0)
        if (!touch) return
        const selection = win.getSelection()
        touchGesture = {
          identifier: touch.identifier,
          startX: touch.clientX,
          startY: touch.clientY,
          startedAt: window.performance.now(),
          target: event.target,
          moved: false,
          selectionAtStart: Boolean(selection && !selection.isCollapsed),
        }
      }
      const touchMoveHandler = (event: TouchEvent) => {
        const gesture = touchGesture
        if (!gesture) return
        if (event.touches.length !== 1) {
          touchGesture = null
          return
        }
        const touch = Array.from(event.touches).find((item) => item.identifier === gesture.identifier)
        if (!touch) {
          touchGesture = null
          return
        }
        if (Math.hypot(touch.clientX - gesture.startX, touch.clientY - gesture.startY) > 12) {
          gesture.moved = true
        }
      }
      const touchEndHandler = (event: TouchEvent) => {
        const gesture = touchGesture
        touchGesture = null
        selectionHandler()
        if (!gesture || event.touches.length !== 0) return
        const touch = Array.from(event.changedTouches).find((item) => item.identifier === gesture.identifier)
        if (!touch) return
        const target = gesture.target as Element | null
        if (target?.closest?.('a[href], audio, video, [contenteditable="true"], [role="button"], [role="link"]')) return
        const duration = window.performance.now() - gesture.startedAt
        const distance = Math.hypot(touch.clientX - gesture.startX, touch.clientY - gesture.startY)
        if (gesture.moved || gesture.selectionAtStart || distance > 12 || duration > 400) return
        const selection = win.getSelection()
        if (selection && !selection.isCollapsed) return

        suppressClickUntil = window.performance.now() + 900
        const clientX = touch.clientX
        pendingTapTimer = window.setTimeout(() => {
          pendingTapTimer = null
          runTapAction(clientX)
        }, 260)
      }
      const touchCancelHandler = () => {
        touchGesture = null
        clearPendingTap()
      }

      const touchSurface = doc.documentElement
      const frameTouchPrimer = () => undefined
      frame.addEventListener('touchstart', frameTouchPrimer, { passive: true })
      doc.addEventListener('click', clickHandler)
      doc.addEventListener('mouseup', selectionHandler)
      doc.addEventListener('keyup', selectionHandler)
      touchSurface.addEventListener('touchstart', touchStartHandler, { capture: true, passive: true })
      touchSurface.addEventListener('touchmove', touchMoveHandler, { capture: true, passive: true })
      touchSurface.addEventListener('touchend', touchEndHandler, { capture: true, passive: true })
      touchSurface.addEventListener('touchcancel', touchCancelHandler, { capture: true, passive: true })
      frameCleanupRef.current = () => {
        clearPendingTap()
        touchGesture = null
        frame.removeEventListener('touchstart', frameTouchPrimer)
        doc.removeEventListener('click', clickHandler)
        doc.removeEventListener('mouseup', selectionHandler)
        doc.removeEventListener('keyup', selectionHandler)
        touchSurface.removeEventListener('touchstart', touchStartHandler, true)
        touchSurface.removeEventListener('touchmove', touchMoveHandler, true)
        touchSurface.removeEventListener('touchend', touchEndHandler, true)
        touchSurface.removeEventListener('touchcancel', touchCancelHandler, true)
        clearEPUBHighlights(doc)
      }
      setFrameReady(true)

      const next = publication.spine[spineIndex + 1]
      if (next) void fetch(next.content_url, { credentials: 'same-origin' }).catch(() => undefined)
    } catch (error) {
      setFrameError(error instanceof Error ? error.message : 'Could not prepare this chapter.')
      setFrameReady(false)
    }
  }, [alignment, applyCurrentColumn, fontSize, leftAction, lineHeight, margin, navigateToSpine, pendingFragment, publication, readerFont, readerWidth, rightAction, spineIndex, theme])

  useEffect(() => () => frameCleanupRef.current?.(), [])

  useEffect(() => {
    if (!currentItem || frameReady || frameError) return
    const timeout = window.setTimeout(() => {
      setFrameError('The chapter took too long to open. Try again or update samrai.')
    }, 15_000)
    return () => window.clearTimeout(timeout)
  }, [currentItem, frameError, frameReady, frameReloadNonce])

  useEffect(() => {
    if (!frameReady || isFixed) return
    applyCurrentColumn(columnIndex)
  }, [applyCurrentColumn, columnIndex, frameReady, isFixed])

  useEffect(() => {
    const frame = iframeRef.current
    if (!frame?.contentDocument || !frameReady) return
    configureFrame()
  }, [configureFrame])

  useEffect(() => {
    const update = () => {
      if (!iframeRef.current?.contentDocument) return
      configureFrame()
    }
    window.addEventListener('resize', update)
    return () => window.removeEventListener('resize', update)
  }, [configureFrame])

  useEffect(() => {
    if (!frameReady) return
    const frame = iframeRef.current
    const doc = frame?.contentDocument
    if (!frame || !doc) return
    if (highlightFrameRef.current !== null) window.cancelAnimationFrame(highlightFrameRef.current)
    highlightFrameRef.current = window.requestAnimationFrame(() => {
      highlightFrameRef.current = null
      renderEPUBHighlights(doc, currentAnnotations, activeSearchQuery)
    })
    return () => {
      if (highlightFrameRef.current !== null) window.cancelAnimationFrame(highlightFrameRef.current)
    }
  }, [activeSearchQuery, currentAnnotations, frameReady, columnIndex, columnCount, theme, fontSize, lineHeight, margin, readerWidth, readerFont, alignment])

  useEffect(() => {
    if (!frameReady) return
    const frame = iframeRef.current
    const doc = frame?.contentDocument
    if (!frame || !doc) return

    if (pendingAnnotationID !== null) {
      const annotation = annotations.find((item) => item.id === pendingAnnotationID)
      if (annotation && annotation.spine_index === spineIndex) {
        const range = resolveEPUBAnnotationRange(doc, annotation)
        if (range) focusEPUBRange(range, frameStateRef.current, applyCurrentColumn)
      }
      setPendingAnnotationID(null)
    }

    if (pendingSearchQuery) {
      const range = findEPUBTextRange(epubFlowRoot(doc), pendingSearchQuery)
      setActiveSearchQuery(pendingSearchQuery)
      if (range) focusEPUBRange(range, frameStateRef.current, applyCurrentColumn)
      setPendingSearchQuery('')
    }
  }, [annotations, applyCurrentColumn, frameReady, pendingAnnotationID, pendingSearchQuery, spineIndex])

  useEffect(() => {
    function onKeyDown(event: KeyboardEvent) {
      if (event.defaultPrevented) return
      if (event.key === 'ArrowLeft') {
        event.preventDefault()
        leftAction()
      } else if (event.key === 'ArrowRight') {
        event.preventDefault()
        rightAction()
      } else if (event.key === ' ' || event.key === 'PageDown') {
        event.preventDefault()
        nextPage()
      } else if (event.key === 'PageUp') {
        event.preventDefault()
        previousPage()
      } else if (event.key.toLowerCase() === 'f') {
        event.preventDefault()
        void toggleFullscreen(readerRef.current)
      } else if (event.key === 'Escape') {
        if (panel) setPanel(null)
        else if (!document.fullscreenElement) void closeReader()
      }
    }
    window.addEventListener('keydown', onKeyDown)
    return () => window.removeEventListener('keydown', onKeyDown)
  })

  useEffect(() => {
    const onFullscreenChange = () => setFullscreen(document.fullscreenElement === readerRef.current)
    document.addEventListener('fullscreenchange', onFullscreenChange)
    return () => document.removeEventListener('fullscreenchange', onFullscreenChange)
  }, [])

  useEffect(() => () => { void api.endReadingSession(book.id, sessionIDRef.current) }, [])

  const saveProgress = useCallback(async () => {
    if (!publication) return
    const location: EPUBReadingLocation = {
      spine_index: spineIndex,
      column_index: isFixed ? 0 : columnIndex,
      column_count: isFixed ? 1 : columnCount,
      chapter_progress: isFixed || columnCount <= 1 ? 0 : columnIndex / Math.max(1, columnCount - 1),
    }
    setSaveState('saving')
    try {
      const progress = await api.saveProgress(book.id, spineIndex, location, sessionIDRef.current)
      setSaveState('saved')
      queryClient.setQueryData<Book>(['book', book.id], (current) => current ? {
        ...current,
        current_page: progress.current_page,
        reading_location: progress.location,
        started: progress.started,
        completed: progress.completed,
      } : current)
      queryClient.setQueryData<BooksResponse>(['books'], (current) => current ? {
        ...current,
        items: current.items.map((item) => item.id === book.id ? {
          ...item,
          current_page: progress.current_page,
          reading_location: progress.location,
          started: progress.started,
          completed: progress.completed,
        } : item),
      } : current)
      window.setTimeout(() => setSaveState((value) => value === 'saved' ? 'idle' : value), 1200)
    } catch {
      setSaveState('error')
    }
  }, [book.id, columnCount, columnIndex, isFixed, publication, queryClient, spineIndex])

  useEffect(() => {
    if (!publication || !frameReady) return
    if (saveTimerRef.current !== null) window.clearTimeout(saveTimerRef.current)
    saveTimerRef.current = window.setTimeout(() => void saveProgress(), 500)
    return () => {
      if (saveTimerRef.current !== null) window.clearTimeout(saveTimerRef.current)
    }
  }, [columnIndex, frameReady, publication, saveProgress, spineIndex])

  useEffect(() => {
    if (!controlsVisible || panel || selectionDraft) return
    const timer = window.setTimeout(() => setControlsVisible(false), 3600)
    return () => window.clearTimeout(timer)
  }, [controlsVisible, panel, selectionDraft, spineIndex, columnIndex])

  async function closeReader() {
    if (saveTimerRef.current !== null) window.clearTimeout(saveTimerRef.current)
    await saveProgress()
    await api.endReadingSession(book.id, sessionIDRef.current).catch(() => undefined)
    if (document.fullscreenElement) await document.exitFullscreen().catch(() => undefined)
    onClose()
  }

  function updateTheme(value: ReaderTheme) {
    setTheme(value)
    localStorage.setItem(themeKey, value)
  }

  function updateFontSize(value: number) {
    const safe = clamp(value, 13, 34)
    setFontSize(safe)
    localStorage.setItem(fontSizeKey, String(safe))
  }

  function updateLineHeight(value: number) {
    const safe = Math.round(clamp(value, 1.2, 2.2) * 10) / 10
    setLineHeight(safe)
    localStorage.setItem(lineHeightKey, String(safe))
  }

  function updateMargin(value: number) {
    const safe = clamp(value, 18, 96)
    setMargin(safe)
    localStorage.setItem(marginKey, String(safe))
  }

  function updateReaderWidth(value: ReaderWidth) {
    setReaderWidth(value)
    localStorage.setItem(widthKey, value)
  }

  function updateReaderFont(value: ReaderFont) {
    setReaderFont(value)
    localStorage.setItem(fontKey, value)
  }

  function updateAlignment(value: ReaderAlignment) {
    setAlignment(value)
    localStorage.setItem(alignmentKey, value)
  }

  const chapterTitle = currentItem?.title || `Section ${spineIndex + 1}`
  const chapterProgress = isFixed || columnCount <= 1 ? 1 : columnIndex / Math.max(1, columnCount - 1)
  const totalProgress = publication ? ((spineIndex + chapterProgress) / publication.spine.length) * 100 : 0
  const canPrevious = spineIndex > 0 || (!isFixed && columnIndex > 0)
  const canNext = publication ? spineIndex < publication.spine.length - 1 || (!isFixed && columnIndex < columnCount - 1) : false

  if (publicationQuery.isPending) {
    return <EPUBStatus title="Opening EPUB" message="Reading the spine and table of contents…" />
  }
  if (publicationQuery.isError || !publication || !currentItem) {
    return <EPUBStatus error title="Could not open the EPUB" message="The file may be incomplete or may have been removed." actionLabel="Back" onAction={onClose} />
  }

  return (
    <div className={`epub-reader epub-theme-${theme}`} ref={readerRef}>
      <div className="epub-stage">
        {/* WebKit requires script permission for parent-installed handlers inside a sandboxed same-origin frame.
            EPUB scripts are still blocked by the response CSP and stripped from the loaded document. */}
        <iframe
          key={`${book.id}-${spineIndex}-${currentItem.content_url}-${frameReloadNonce}`}
          className={`epub-frame${frameReady ? ' epub-frame-ready' : ''}`}
          ref={iframeRef}
          src={currentItem.content_url}
          sandbox="allow-same-origin allow-scripts"
          title={`${book.title} — ${chapterTitle}`}
          onLoad={configureFrame}
          onError={() => setFrameError('Could not load this chapter.')}
        />
        {!frameReady && !frameError ? <div className="epub-loading"><span className="spinner" /><strong>Preparing {isFixed ? 'page' : 'chapter'}…</strong></div> : null}
        {frameError ? <div className="epub-loading epub-error"><AlertIcon /><strong>{frameError}</strong><button className="button button-secondary" onClick={() => { setFrameError(''); setFrameReady(false); setFrameReloadNonce((value) => value + 1) }}>Try again</button></div> : null}
      </div>

      {selectionDraft ? (
        <div className="epub-selection-toolbar" style={{ left: selectionDraft.clientX, top: selectionDraft.clientY }}>
          <div className="epub-selection-colors">
            {annotationColors.map((color) => (
              <button key={color} className={`annotation-color annotation-color-${color}${selectedColor === color ? ' active' : ''}`} aria-label={annotationColorLabels[color]} onClick={() => setSelectedColor(color)} />
            ))}
          </div>
          {selectionNoteOpen ? (
            <div className="epub-selection-note">
              <textarea autoFocus value={selectionNote} onChange={(event) => setSelectionNote(event.target.value)} placeholder="Write your note…" maxLength={20_000} />
              <button disabled={createEPUBAnnotation.isPending} onClick={() => createEPUBAnnotation.mutate({ draft: selectionDraft, note: selectionNote })}>Save</button>
            </div>
          ) : (
            <>
              <button disabled={createEPUBAnnotation.isPending} onClick={() => createEPUBAnnotation.mutate({ draft: selectionDraft, note: '' })}>Highlight</button>
              <button onClick={() => setSelectionNoteOpen(true)}>+ Note</button>
            </>
          )}
          <button className="epub-toolbar-close" aria-label="Cancel selection" onClick={() => { clearEPUBSelection(iframeRef.current?.contentWindow); setSelectionDraft(null); setSelectionNoteOpen(false) }}>×</button>
        </div>
      ) : null}

      <header className={`epub-topbar${controlsVisible ? ' epub-controls-visible' : ''}`}>
        <button className="epub-control-button" aria-label="Back to library" onClick={() => void closeReader()}><BackIcon /></button>
        <div className="epub-title-copy">
          <strong>{book.title}</strong>
          <span>{chapterTitle}</span>
        </div>
        <div className="epub-top-actions">
          {saveState === 'saved' ? <span className="epub-saved"><CheckIcon /> Saved</span> : null}
          {saveState === 'error' ? <span className="epub-save-error">Save failed</span> : null}
          <button className="epub-control-button" aria-label="Search this book" onClick={() => setPanel(panel === 'search' ? null : 'search')}><SearchIcon /></button>
          <button className="epub-control-button epub-annotation-button" aria-label="Highlights and notes" onClick={() => setPanel(panel === 'annotations' ? null : 'annotations')}>✎{annotations.length ? <span>{annotations.length}</span> : null}</button>
          <button className="epub-control-button epub-toc-button" aria-label="Open table of contents" onClick={() => setPanel(panel === 'toc' ? null : 'toc')}>☰</button>
          <button className="epub-control-button" aria-label="Settings" onClick={() => setPanel(panel === 'settings' ? null : 'settings')}><SettingsIcon /></button>
          <button className="epub-control-button" aria-label={fullscreen ? 'Exit full screen' : 'Full screen'} onClick={() => void toggleFullscreen(readerRef.current)}>{fullscreen ? <FullscreenExitIcon /> : <FullscreenIcon />}</button>
        </div>
      </header>

      <button className={`epub-edge-button epub-edge-left${controlsVisible ? ' epub-controls-visible' : ''}`} disabled={(!canPrevious && direction === 'ltr') || (!canNext && direction === 'rtl')} onClick={leftAction} aria-label={direction === 'rtl' ? 'Next page' : 'Previous page'}><ArrowLeftIcon /></button>
      <button className={`epub-edge-button epub-edge-right${controlsVisible ? ' epub-controls-visible' : ''}`} disabled={(!canNext && direction === 'ltr') || (!canPrevious && direction === 'rtl')} onClick={rightAction} aria-label={direction === 'rtl' ? 'Previous page' : 'Next page'}><ArrowRightIcon /></button>

      <footer className={`epub-footer${controlsVisible ? ' epub-controls-visible' : ''}`}>
        <div className="epub-progress-track"><span style={{ width: `${clamp(totalProgress, 0, 100)}%` }} /></div>
        <span>{isFixed ? `Page ${spineIndex + 1} of ${publication.spine.length}` : `Section ${spineIndex + 1} of ${publication.spine.length} · screen ${columnIndex + 1} of ${columnCount}`}</span>
        <small>{Math.round(totalProgress)}%</small>
      </footer>

      {panel ? (
        <aside className="epub-panel">
          <div className="epub-panel-header">
            <div><span className="eyebrow">EPUB {publication.version || ''}</span><h2>{panel === 'toc' ? 'Table of contents' : panel === 'search' ? 'Search' : panel === 'annotations' ? 'Highlights and notes' : 'Reading'}</h2></div>
            <button className="epub-control-button" onClick={() => setPanel(null)} aria-label="Close panel">×</button>
          </div>

          {panel === 'toc' ? (
            <div className="epub-toc-list">
              {(publication.toc.length ? publication.toc : publication.spine.map((item) => ({ position: item.index, label: item.title, path: item.path, fragment: '', spine_index: item.index, depth: 0 }))).map((item) => (
                <button
                  className={item.spine_index === spineIndex ? 'epub-toc-active' : ''}
                  key={`${item.position}-${item.path}-${item.fragment}`}
                  style={{ paddingLeft: `${16 + item.depth * 16}px` }}
                  disabled={item.spine_index === undefined}
                  onClick={() => item.spine_index !== undefined && (navigateToSpine(item.spine_index, 0, item.fragment), setPanel(null))}
                >
                  <span>{item.label}</span>
                  {item.spine_index !== undefined ? <small>{item.spine_index + 1}</small> : null}
                </button>
              ))}
            </div>
          ) : null}

          {panel === 'search' ? (
            <div className="epub-search-panel">
              <form onSubmit={(event) => { event.preventDefault(); if (search.trim().length >= 2) searchMutation.mutate(search.trim()) }}>
                <SearchIcon />
                <input autoFocus value={search} onChange={(event) => setSearch(event.target.value)} placeholder="Word or phrase…" />
                <button className="button button-primary" disabled={searchMutation.isPending || search.trim().length < 2}>Search</button>
              </form>
              {searchMutation.isPending ? <div className="epub-panel-state"><span className="spinner spinner-small" /> Searching…</div> : null}
              {searchMutation.isError ? <div className="epub-panel-state epub-panel-error">Search failed.</div> : null}
              {searchMutation.data && searchMutation.data.items.length === 0 ? <div className="epub-panel-state">No matches found.</div> : null}
              <div className="epub-search-results">
                {searchMutation.data?.items.map((result) => (
                  <button key={`${result.spine_index}-${result.excerpt}`} onClick={() => { setPendingSearchQuery(search.trim()); navigateToSpine(result.spine_index); setPanel(null) }}>
                    <strong>{result.title}</strong>
                    <p>{result.excerpt}</p>
                    <small>{result.matches} {result.matches === 1 ? 'match' : 'matches'}</small>
                  </button>
                ))}
              </div>
            </div>
          ) : null}

          {panel === 'annotations' ? (
            <div className="epub-annotations-panel">
              <div className="epub-annotation-tools">
                <input value={annotationSearch} onChange={(event) => setAnnotationSearch(event.target.value)} placeholder="Search highlights…" />
                <div className="epub-annotation-filters">
                  <button className={annotationColorFilter === 'all' ? 'active' : ''} onClick={() => setAnnotationColorFilter('all')}>All</button>
                  {annotationColors.map((color) => <button key={color} className={`annotation-color annotation-color-${color}${annotationColorFilter === color ? ' active' : ''}`} aria-label={annotationColorLabels[color]} onClick={() => setAnnotationColorFilter(color)} />)}
                </div>
                <a className="button button-secondary epub-export-annotations" href={epubAnnotationsExportURL(book.id)}><DownloadIcon /> Export Markdown</a>
              </div>
              {annotationsQuery.isPending ? <div className="epub-panel-state"><span className="spinner spinner-small" /> Loading highlights…</div> : null}
              {!annotationsQuery.isPending && filteredAnnotations.length === 0 ? <div className="epub-panel-state">No highlights found.</div> : null}
              <div className="epub-annotation-list">
                {filteredAnnotations.map((annotation) => {
                  const editing = editingAnnotationID === annotation.id
                  const sectionTitle = publication.spine[annotation.spine_index]?.title || `Section ${annotation.spine_index + 1}`
                  return (
                    <article className={`epub-annotation-card epub-annotation-${annotation.color}`} key={annotation.id}>
                      <button className="epub-annotation-jump" onClick={() => { setPendingAnnotationID(annotation.id); navigateToSpine(annotation.spine_index); setPanel(null) }}>
                        <small>{sectionTitle}</small>
                        <blockquote>{annotation.selected_text || 'Note without selected text'}</blockquote>
                      </button>
                      {editing ? (
                        <div className="epub-annotation-editor">
                          <div className="epub-annotation-filters">
                            {annotationColors.map((color) => <button key={color} className={`annotation-color annotation-color-${color}${annotation.color === color ? ' active' : ''}`} aria-label={annotationColorLabels[color]} onClick={() => updateEPUBAnnotation.mutate({ id: annotation.id, color })} />)}
                          </div>
                          <textarea value={editingAnnotationNote} onChange={(event) => setEditingAnnotationNote(event.target.value)} placeholder="Optional note…" maxLength={20_000} />
                          <div className="epub-annotation-editor-actions">
                            <button onClick={() => setEditingAnnotationID(null)}>Cancel</button>
                            <button className="button button-primary" disabled={updateEPUBAnnotation.isPending} onClick={() => updateEPUBAnnotation.mutate({ id: annotation.id, note: editingAnnotationNote }, { onSuccess: () => setEditingAnnotationID(null) })}>Save note</button>
                            <button className="epub-delete-annotation" disabled={deleteEPUBAnnotation.isPending} onClick={() => { if (window.confirm('Delete this highlight?')) deleteEPUBAnnotation.mutate(annotation.id) }}><TrashIcon /></button>
                          </div>
                        </div>
                      ) : (
                        <div className="epub-annotation-card-footer">
                          <p>{annotation.note || 'No note.'}</p>
                          <button onClick={() => { setEditingAnnotationID(annotation.id); setEditingAnnotationNote(annotation.note) }}>Edit</button>
                        </div>
                      )}
                    </article>
                  )
                })}
              </div>
            </div>
          ) : null}

          {panel === 'settings' ? (
            <div className="epub-settings-list">
              <section>
                <label>Theme</label>
                <div className="epub-theme-options">
                  {(['paper', 'white', 'dark', 'oled'] as ReaderTheme[]).map((value) => <button key={value} className={theme === value ? 'active' : ''} onClick={() => updateTheme(value)}>{themeLabel(value)}</button>)}
                </div>
              </section>
              {!isFixed ? <>
                <section>
                  <label>Page width</label>
                  <div className="epub-segmented-options">
                    {(['compact', 'comfortable', 'wide'] as ReaderWidth[]).map((value) => <button key={value} className={readerWidth === value ? 'active' : ''} onClick={() => updateReaderWidth(value)}>{readerWidthLabel(value)}</button>)}
                  </div>
                </section>
                <section>
                  <label>Font</label>
                  <div className="epub-segmented-options">
                    {(['publisher', 'serif', 'sans'] as ReaderFont[]).map((value) => <button key={value} className={readerFont === value ? 'active' : ''} onClick={() => updateReaderFont(value)}>{readerFontLabel(value)}</button>)}
                  </div>
                </section>
                <section><label>Font size <strong>{fontSize}px</strong></label><input type="range" min="13" max="34" value={fontSize} onChange={(event) => updateFontSize(Number(event.target.value))} /></section>
                <section><label>Line spacing <strong>{lineHeight.toFixed(1)}</strong></label><input type="range" min="1.2" max="2.2" step="0.1" value={lineHeight} onChange={(event) => updateLineHeight(Number(event.target.value))} /></section>
                <section><label>Page margins <strong>{margin}px</strong></label><input type="range" min="18" max="96" value={margin} onChange={(event) => updateMargin(Number(event.target.value))} /></section>
                <section>
                  <label>Alignment</label>
                  <div className="epub-segmented-options epub-segmented-two">
                    {(['justify', 'left'] as ReaderAlignment[]).map((value) => <button key={value} className={alignment === value ? 'active' : ''} onClick={() => updateAlignment(value)}>{value === 'justify' ? 'Justified' : 'Left aligned'}</button>)}
                  </div>
                </section>
              </> : <p className="epub-fixed-note">This EPUB uses a fixed layout. samrai preserves the original composition of each page.</p>}
              <div className="epub-direction-info"><DirectionIcon /><span>Reading direction: <strong>{direction === 'rtl' ? 'right to left' : 'left to right'}</strong></span></div>
            </div>
          ) : null}
        </aside>
      ) : null}
    </div>
  )
}

function configureReflowableDocument(
  frame: HTMLIFrameElement,
  doc: Document,
  options: {
    theme: ReaderTheme
    fontSize: number
    lineHeight: number
    margin: number
    readerWidth: ReaderWidth
    readerFont: ReaderFont
    alignment: ReaderAlignment
  },
): FrameState {
  const viewportWidth = Math.max(320, frame.clientWidth)
  const viewportHeight = Math.max(320, frame.clientHeight)
  const root = doc.documentElement as HTMLElement
  const body = (doc.body || root) as HTMLElement
  const flow = ensureReflowableContainer(doc, body)
  const palette = themePalette(options.theme)
  const horizontalSafety = viewportWidth <= 720 ? 18 : 72
  const desiredTextWidth = readerTextWidth(options.readerWidth)
  const responsiveMarginCap = viewportWidth <= 480 ? 32 : viewportWidth <= 720 ? 40 : 96
  const innerMargin = clamp(Math.min(options.margin, responsiveMarginCap), 18, Math.max(18, Math.floor((viewportWidth - horizontalSafety * 2) / 5)))
  const maximumPageWidth = Math.max(284, viewportWidth - horizontalSafety * 2)
  const pageWidth = Math.min(maximumPageWidth, desiredTextWidth + innerMargin * 2)
  const textWidth = Math.max(248, pageWidth - innerMargin * 2)
  const pageLeft = Math.max(0, Math.floor((viewportWidth - pageWidth) / 2))
  const textLeft = pageLeft + innerMargin
  const topInset = viewportWidth <= 720 ? 72 : 82
  const bottomInset = viewportWidth <= 720 ? 66 : 74
  const flowHeight = Math.max(220, viewportHeight - topInset - bottomInset)
  const columnGap = Math.max(0, viewportWidth - textWidth)

  root.dataset.samraiLayout = 'reflowable'
  root.style.width = `${viewportWidth}px`
  root.style.height = `${viewportHeight}px`
  root.style.margin = '0'
  root.style.padding = '0'
  root.style.overflow = 'hidden'
  root.style.background = `linear-gradient(to right, ${palette.surround} 0 ${pageLeft}px, ${palette.page} ${pageLeft}px ${pageLeft + pageWidth}px, ${palette.surround} ${pageLeft + pageWidth}px 100%) fixed`
  root.style.colorScheme = options.theme === 'dark' || options.theme === 'oled' ? 'dark' : 'light'

  body.style.position = 'relative'
  body.style.left = '0'
  body.style.top = '0'
  body.style.transform = 'none'
  body.style.transformOrigin = ''
  body.style.width = `${viewportWidth}px`
  body.style.height = `${viewportHeight}px`
  body.style.minHeight = `${viewportHeight}px`
  body.style.margin = '0'
  body.style.padding = '0'
  body.style.overflow = 'visible'
  body.style.boxSizing = 'border-box'
  body.style.color = palette.text
  body.style.background = 'transparent'

  flow.style.position = 'relative'
  flow.style.display = 'block'
  flow.style.width = `${textWidth}px`
  flow.style.height = `${flowHeight}px`
  flow.style.minHeight = `${flowHeight}px`
  flow.style.margin = `${topInset}px 0 ${bottomInset}px ${textLeft}px`
  flow.style.padding = '0'
  flow.style.boxSizing = 'border-box'
  flow.style.columnWidth = `${textWidth}px`
  flow.style.columnGap = `${columnGap}px`
  flow.style.columnFill = 'auto'
  flow.style.overflow = 'visible'
  flow.style.fontSize = `${options.fontSize}px`
  flow.style.lineHeight = String(options.lineHeight)
  flow.style.color = palette.text
  flow.style.background = 'transparent'
  flow.style.overflowWrap = 'break-word'
  flow.style.hyphens = 'auto'
  flow.style.textRendering = 'optimizeLegibility'
  flow.style.fontFamily = readerFontFamily(options.readerFont)
  flow.style.textAlign = options.alignment

  applyReflowableReaderStyle(doc, {
    alignment: options.alignment,
    fontFamily: readerFontFamily(options.readerFont),
    fontSize: options.fontSize,
    lineHeight: options.lineHeight,
    palette,
    flowHeight,
  })
  normalizeChapterOpening(flow)

  for (const element of doc.querySelectorAll<HTMLElement>('img, svg, video')) {
    element.style.maxWidth = '100%'
    element.style.maxHeight = `${Math.max(180, flowHeight - 24)}px`
    element.style.width = 'auto'
    element.style.height = 'auto'
    element.style.objectFit = 'contain'
    element.style.breakInside = 'avoid'
    element.style.pageBreakInside = 'avoid'
  }
  for (const element of doc.querySelectorAll<HTMLElement>('pre, table')) {
    element.style.maxWidth = '100%'
    element.style.overflow = 'auto'
  }
  for (const element of doc.querySelectorAll<HTMLAnchorElement>('a')) element.style.color = palette.link

  // Force layout before measuring the overflowing columns.
  void flow.offsetWidth
  const columnCount = Math.max(1, Math.round((flow.scrollWidth + columnGap) / viewportWidth))
  // Give the final column a complete viewport slot so it can be centered just like the first one.
  body.style.width = `${columnCount * viewportWidth}px`
  body.style.minWidth = `${columnCount * viewportWidth}px`
  return { columnCount, viewportWidth, pageWidth, textWidth }
}

function ensureReflowableContainer(doc: Document, body: HTMLElement) {
  const existing = body.querySelector<HTMLElement>(':scope > [data-samrai-flow="true"]')
  if (existing) return existing
  const flow = doc.createElement('div')
  flow.dataset.samraiFlow = 'true'
  const nodes = Array.from(body.childNodes)
  for (const node of nodes) flow.appendChild(node)
  body.appendChild(flow)
  return flow
}

function applyReflowableReaderStyle(doc: Document, options: {
  alignment: ReaderAlignment
  fontFamily: string
  fontSize: number
  lineHeight: number
  palette: ReturnType<typeof themePalette>
  flowHeight: number
}) {
  let style = doc.getElementById('samrai-reflowable-style') as HTMLStyleElement | null
  if (!style) {
    style = doc.createElement('style')
    style.id = 'samrai-reflowable-style'
    doc.head?.appendChild(style)
  }
  const alignmentRule = options.alignment === 'left' ? 'left' : 'justify'
  style.textContent = `
    html[data-samrai-layout="reflowable"],
    html[data-samrai-layout="reflowable"] body {
      max-width: none !important;
    }
    [data-samrai-flow="true"] {
      font-family: ${options.fontFamily} !important;
      font-size: ${options.fontSize}px !important;
      line-height: ${options.lineHeight} !important;
      color: ${options.palette.text} !important;
    }
    [data-samrai-flow="true"] p,
    [data-samrai-flow="true"] li,
    [data-samrai-flow="true"] blockquote {
      text-align: ${alignmentRule} !important;
      line-height: inherit !important;
    }
    [data-samrai-flow="true"] > :first-child {
      margin-top: 0 !important;
    }
    [data-samrai-flow="true"] h1,
    [data-samrai-flow="true"] h2,
    [data-samrai-flow="true"] h3,
    [data-samrai-flow="true"] h4 {
      break-after: avoid;
      page-break-after: avoid;
      max-width: 100% !important;
    }
    [data-samrai-flow="true"] img,
    [data-samrai-flow="true"] svg,
    [data-samrai-flow="true"] figure,
    [data-samrai-flow="true"] table,
    [data-samrai-flow="true"] pre,
    [data-samrai-flow="true"] blockquote {
      break-inside: avoid;
      page-break-inside: avoid;
    }
    [data-samrai-flow="true"] img,
    [data-samrai-flow="true"] svg {
      max-height: ${Math.max(180, options.flowHeight - 24)}px !important;
    }
  `
}

function normalizeChapterOpening(flow: HTMLElement) {
  const first = firstVisibleElement(flow)
  if (!first) return
  const chain: HTMLElement[] = []
  let current: HTMLElement | null = first
  while (current && current !== flow) {
    chain.push(current)
    current = current.parentElement
  }
  for (const element of chain) {
    const style = element.ownerDocument.defaultView?.getComputedStyle(element)
    if (!style) continue
    const marginTop = Number.parseFloat(style.marginTop)
    const paddingTop = Number.parseFloat(style.paddingTop)
    if (Number.isFinite(marginTop) && marginTop > 96) element.style.setProperty('margin-top', '1.5em', 'important')
    if (Number.isFinite(paddingTop) && paddingTop > 96) element.style.setProperty('padding-top', '0', 'important')
    if (style.minHeight.endsWith('px') && Number.parseFloat(style.minHeight) > flow.clientHeight * 0.7) element.style.setProperty('min-height', 'auto', 'important')
  }
}

function firstVisibleElement(root: HTMLElement): HTMLElement | null {
  const walker = root.ownerDocument.createTreeWalker(root, NodeFilter.SHOW_ELEMENT)
  let node = walker.nextNode() as HTMLElement | null
  while (node) {
    if (!['SCRIPT', 'STYLE', 'LINK', 'META'].includes(node.tagName) && (node.textContent?.trim() || node.querySelector('img,svg'))) return node
    node = walker.nextNode() as HTMLElement | null
  }
  return null
}

function readerTextWidth(width: ReaderWidth) {
  return width === 'compact' ? 560 : width === 'wide' ? 800 : 680
}

function readerFontFamily(font: ReaderFont) {
  if (font === 'publisher') return 'inherit'
  if (font === 'sans') return 'ui-sans-serif, system-ui, -apple-system, "Segoe UI", sans-serif'
  return 'ui-serif, Georgia, Cambria, "Times New Roman", serif'
}

function configureFixedDocument(frame: HTMLIFrameElement, doc: Document, theme: ReaderTheme) {
  const root = doc.documentElement as HTMLElement
  const host = (doc.body || root) as HTMLElement
  const palette = themePalette(theme)
  root.style.margin = '0'
  root.style.width = '100%'
  root.style.height = '100%'
  root.style.overflow = 'hidden'
  root.style.background = palette.background
  host.style.margin = '0'
  host.style.overflow = 'hidden'
  host.style.background = palette.background
  host.style.transform = ''
  host.style.left = ''
  host.style.top = ''
  host.style.width = ''
  host.style.height = ''
  host.style.position = ''
  const viewport = parseViewport(doc.querySelector<HTMLMetaElement>('meta[name="viewport"]')?.content || '')
  const sourceWidth = viewport.width || Math.max(host.scrollWidth, root.scrollWidth, 1)
  const sourceHeight = viewport.height || Math.max(host.scrollHeight, root.scrollHeight, 1)
  const mobileEdgeToEdge = window.matchMedia('(max-width: 720px)').matches
  const rawScale = mobileEdgeToEdge
    ? frame.clientWidth / sourceWidth
    : Math.min(frame.clientWidth / sourceWidth, frame.clientHeight / sourceHeight)
  const scale = Number.isFinite(rawScale) && rawScale > 0 ? rawScale : 1
  host.style.position = 'absolute'
  host.style.width = `${sourceWidth}px`
  host.style.height = `${sourceHeight}px`
  host.style.transformOrigin = 'top left'
  host.style.transform = `scale(${scale})`
  host.style.left = mobileEdgeToEdge ? '0px' : `${Math.max(0, (frame.clientWidth - sourceWidth * scale) / 2)}px`
  host.style.top = `${Math.max(0, (frame.clientHeight - sourceHeight * scale) / 2)}px`
  root.style.overflowY = sourceHeight * scale > frame.clientHeight ? 'auto' : 'hidden'
  root.style.overflowX = 'hidden'
  for (const image of doc.querySelectorAll<HTMLElement>('img, svg')) {
    image.style.maxWidth = '100%'
    image.style.maxHeight = '100%'
  }
}

function removeDangerousDocumentFeatures(doc: Document) {
  for (const element of doc.querySelectorAll('script, iframe, frame, object, embed, form, input, button, textarea, select, base, meta[http-equiv]')) element.remove()
  for (const element of doc.querySelectorAll<HTMLLinkElement>('link[rel]')) {
    const relation = element.rel.toLowerCase()
    if (relation !== 'stylesheet') element.remove()
  }
  for (const element of doc.querySelectorAll<HTMLElement>('*')) {
    for (const attribute of Array.from(element.attributes)) {
      if (attribute.name.toLowerCase().startsWith('on')) element.removeAttribute(attribute.name)
    }
  }
}

function epubFlowRoot(doc: Document): HTMLElement {
  return doc.querySelector<HTMLElement>('[data-samrai-flow="true"]') || doc.body || doc.documentElement
}

function createEPUBSelectionDraft(frame: HTMLIFrameElement, doc: Document, win: Window): EPUBSelectionDraft | null {
  const selection = win.getSelection()
  if (!selection || selection.isCollapsed || !selection.rangeCount) return null
  const range = selection.getRangeAt(0)
  const root = epubFlowRoot(doc)
  if (!root.contains(range.commonAncestorContainer)) return null
  const text = normalizeEPUBSelectionText(selection.toString())
  if (!text || text.length > 20_000) return null
  const startPath = epubNodePath(root, range.startContainer)
  const endPath = epubNodePath(root, range.endContainer)
  if (!startPath || !endPath) return null
  const context = epubRangeContext(root, range, 180)
  const rects = Array.from(range.getClientRects()).filter((rect) => rect.width > 0 && rect.height > 0)
  const lastRect = rects.at(-1) || range.getBoundingClientRect()
  const frameRect = frame.getBoundingClientRect()
  const clientX = clamp(frameRect.left + lastRect.left + Math.min(lastRect.width / 2, 160), 12, window.innerWidth - 12)
  const clientY = clamp(frameRect.top + lastRect.top - 14, 56, window.innerHeight - 56)
  return {
    text,
    anchor: {
      start_path: startPath,
      start_offset: range.startOffset,
      end_path: endPath,
      end_offset: range.endOffset,
      prefix: context.prefix,
      suffix: context.suffix,
    },
    clientX,
    clientY,
  }
}

function normalizeEPUBSelectionText(value: string) {
  return value.replace(/\s+/g, ' ').trim()
}

function epubNodePath(root: Node, node: Node): string | null {
  if (node === root) return ''
  const parts: number[] = []
  let current: Node | null = node
  while (current && current !== root) {
    const parentNode: ParentNode | null = current.parentNode
    if (!parentNode) return null
    const index = Array.prototype.indexOf.call(parentNode.childNodes, current) as number
    if (index < 0) return null
    parts.push(index)
    current = parentNode
  }
  return current === root ? parts.reverse().join('/') : null
}

function epubNodeFromPath(root: Node, path: string): Node | null {
  if (path === '') return root
  let current: Node = root
  for (const part of path.split('/')) {
    const index = Number(part)
    if (!Number.isInteger(index) || index < 0 || index >= current.childNodes.length) return null
    const child = current.childNodes.item(index)
    if (!child) return null
    current = child
  }
  return current
}

function epubTextNodes(root: Node): Text[] {
  const doc = root.ownerDocument || document
  const walker = doc.createTreeWalker(root, NodeFilter.SHOW_TEXT, {
    acceptNode(node) {
      const parent = node.parentElement
      if (!node.textContent || !parent || parent.closest('#samrai-highlight-overlay, script, style, noscript')) return NodeFilter.FILTER_REJECT
      return NodeFilter.FILTER_ACCEPT
    },
  })
  const nodes: Text[] = []
  let node = walker.nextNode()
  while (node) {
    nodes.push(node as Text)
    node = walker.nextNode()
  }
  return nodes
}

function epubRangeContext(root: HTMLElement, range: Range, size: number) {
  const nodes = epubTextNodes(root)
  let text = ''
  let start = -1
  let end = -1
  for (const node of nodes) {
    const nodeStart = text.length
    const value = node.data
    if (node === range.startContainer) start = nodeStart + range.startOffset
    if (node === range.endContainer) end = nodeStart + range.endOffset
    text += value
  }
  if (start < 0 || end < start) return { prefix: '', suffix: '' }
  return {
    prefix: normalizeEPUBSelectionText(text.slice(Math.max(0, start - size), start)).slice(-size),
    suffix: normalizeEPUBSelectionText(text.slice(end, end + size)).slice(0, size),
  }
}

function resolveEPUBAnnotationRange(doc: Document, annotation: EPUBAnnotation): Range | null {
  const root = epubFlowRoot(doc)
  const startNode = epubNodeFromPath(root, annotation.anchor.start_path)
  const endNode = epubNodeFromPath(root, annotation.anchor.end_path)
  if (startNode && endNode) {
    try {
      const range = doc.createRange()
      range.setStart(startNode, Math.min(annotation.anchor.start_offset, epubNodeLength(startNode)))
      range.setEnd(endNode, Math.min(annotation.anchor.end_offset, epubNodeLength(endNode)))
      const resolved = normalizeEPUBSelectionText(range.toString())
      if (resolved && (resolved === normalizeEPUBSelectionText(annotation.selected_text) || resolved.includes(normalizeEPUBSelectionText(annotation.selected_text)))) return range
    } catch {
      // Fall through to quote-based anchoring.
    }
  }
  return findEPUBTextRange(root, annotation.selected_text, annotation.anchor.prefix, annotation.anchor.suffix)
}

function epubNodeLength(node: Node) {
  return node.nodeType === Node.TEXT_NODE ? (node.textContent?.length ?? 0) : node.childNodes.length
}

type EPUBFlatText = { text: string; nodes: Array<{ node: Text; start: number; end: number }> }

function flattenEPUBText(root: HTMLElement): EPUBFlatText {
  const nodes = epubTextNodes(root)
  const positions: EPUBFlatText['nodes'] = []
  let text = ''
  for (const node of nodes) {
    const start = text.length
    text += node.data
    positions.push({ node, start, end: text.length })
  }
  return { text, nodes: positions }
}

function normalizedTextMap(value: string) {
  let normalized = ''
  const map: number[] = []
  let pendingSpace = false
  for (let index = 0; index < value.length; index++) {
    const character = value.charAt(index)
    const folded = character.normalize('NFD').replace(/\p{M}/gu, '').toLocaleLowerCase()
    if (!/[\p{L}\p{N}]/u.test(folded)) {
      pendingSpace = normalized.length > 0
      continue
    }
    if (pendingSpace && normalized.length > 0) {
      normalized += ' '
      map.push(index)
      pendingSpace = false
    }
    for (const output of folded) {
      if (!/[\p{L}\p{N}]/u.test(output)) continue
      normalized += output
      map.push(index)
    }
  }
  return { normalized: normalized.trim(), map }
}

function normalizeEPUBSearchText(value: string) {
  return normalizedTextMap(value).normalized
}

function findEPUBTextRange(root: HTMLElement, quote: string, prefix = '', suffix = ''): Range | null {
  const target = normalizeEPUBSearchText(quote)
  if (!target) return null
  const flat = flattenEPUBText(root)
  const mapped = normalizedTextMap(flat.text)
  const candidates: number[] = []
  let cursor = 0
  while (cursor <= mapped.normalized.length - target.length) {
    const index = mapped.normalized.indexOf(target, cursor)
    if (index < 0) break
    candidates.push(index)
    cursor = index + Math.max(1, target.length)
  }
  if (!candidates.length) return null
  const normalizedPrefix = normalizeEPUBSearchText(prefix)
  const normalizedSuffix = normalizeEPUBSearchText(suffix)
  let best = candidates[0]!
  let bestScore = -1
  for (const candidate of candidates) {
    const before = mapped.normalized.slice(Math.max(0, candidate - normalizedPrefix.length - 24), candidate)
    const after = mapped.normalized.slice(candidate + target.length, candidate + target.length + normalizedSuffix.length + 24)
    let score = 0
    if (normalizedPrefix && before.endsWith(normalizedPrefix)) score += 2
    if (normalizedSuffix && after.startsWith(normalizedSuffix)) score += 2
    if (score > bestScore) {
      bestScore = score
      best = candidate
    }
  }
  const originalStart = mapped.map[best] ?? 0
  const originalEnd = (mapped.map[Math.min(mapped.map.length - 1, best + target.length - 1)] ?? originalStart) + 1
  return rangeFromFlatOffsets(root.ownerDocument, flat, originalStart, originalEnd)
}

function rangeFromFlatOffsets(doc: Document, flat: EPUBFlatText, start: number, end: number): Range | null {
  const startEntry = flat.nodes.find((item) => start >= item.start && start <= item.end)
  const endEntry = flat.nodes.find((item) => end >= item.start && end <= item.end) || flat.nodes.at(-1)
  if (!startEntry || !endEntry) return null
  try {
    const range = doc.createRange()
    range.setStart(startEntry.node, clamp(start - startEntry.start, 0, startEntry.node.length))
    range.setEnd(endEntry.node, clamp(end - endEntry.start, 0, endEntry.node.length))
    return range
  } catch {
    return null
  }
}

function ensureEPUBHighlightStyle(doc: Document) {
  let style = doc.getElementById('samrai-epub-highlight-style') as HTMLStyleElement | null
  if (!style) {
    style = doc.createElement('style')
    style.id = 'samrai-epub-highlight-style'
    doc.head?.appendChild(style)
  }
  style.textContent = `
    ::highlight(samrai-yellow) { background: rgba(255, 207, 61, .48); color: inherit; }
    ::highlight(samrai-green) { background: rgba(53, 211, 137, .38); color: inherit; }
    ::highlight(samrai-blue) { background: rgba(62, 159, 255, .40); color: inherit; }
    ::highlight(samrai-pink) { background: rgba(239, 86, 164, .38); color: inherit; }
    ::highlight(samrai-orange) { background: rgba(255, 139, 47, .42); color: inherit; }
    ::highlight(samrai-search) { background: rgba(142, 124, 255, .60); color: inherit; text-decoration: underline 2px rgba(110, 88, 255, .9); }
  `
}

function renderEPUBHighlights(doc: Document, annotations: EPUBAnnotation[], searchQuery: string) {
  clearEPUBHighlights(doc)
  ensureEPUBHighlightStyle(doc)
  const win = doc.defaultView
  if (!win) return
  const ranges = new Map<AnnotationColor, Range[]>(annotationColors.map((color) => [color, []]))
  for (const annotation of annotations) {
    const range = resolveEPUBAnnotationRange(doc, annotation)
    if (range) ranges.get(annotation.color)?.push(range)
  }
  const searchRange = searchQuery ? findEPUBTextRange(epubFlowRoot(doc), searchQuery) : null
  const css = (win.CSS as unknown as { highlights?: Map<string, unknown> })?.highlights
  const HighlightCtor = (win as unknown as { Highlight?: new (...ranges: Range[]) => unknown }).Highlight
  if (css && HighlightCtor) {
    for (const [color, colorRanges] of ranges) {
      if (colorRanges.length) css.set(`samrai-${color}`, new HighlightCtor(...colorRanges))
    }
    if (searchRange) css.set('samrai-search', new HighlightCtor(searchRange))
    return
  }
  renderEPUBHighlightOverlay(doc, ranges, searchRange)
}

function renderEPUBHighlightOverlay(doc: Document, ranges: Map<AnnotationColor, Range[]>, searchRange: Range | null) {
  const root = doc.documentElement
  const overlay = doc.createElement('div')
  overlay.id = 'samrai-highlight-overlay'
  overlay.style.cssText = `position:absolute;left:0;top:0;width:${Math.max(root.scrollWidth, root.clientWidth)}px;height:${Math.max(root.scrollHeight, root.clientHeight)}px;pointer-events:none;z-index:2147483000;`
  const win = doc.defaultView
  if (!win) return
  const paint = (range: Range, background: string, search = false) => {
    for (const rect of Array.from(range.getClientRects())) {
      if (rect.width <= 0 || rect.height <= 0) continue
      const mark = doc.createElement('span')
      mark.style.cssText = `position:absolute;left:${rect.left + win.scrollX}px;top:${rect.top + win.scrollY + rect.height * .08}px;width:${rect.width}px;height:${rect.height * .84}px;background:${background};border-radius:2px;${search ? 'box-shadow:inset 0 -2px rgba(110,88,255,.9);' : ''}`
      overlay.appendChild(mark)
    }
  }
  const colors: Record<AnnotationColor, string> = {
    yellow: 'rgba(255,207,61,.48)', green: 'rgba(53,211,137,.38)', blue: 'rgba(62,159,255,.40)', pink: 'rgba(239,86,164,.38)', orange: 'rgba(255,139,47,.42)',
  }
  for (const [color, colorRanges] of ranges) for (const range of colorRanges) paint(range, colors[color])
  if (searchRange) paint(searchRange, 'rgba(142,124,255,.60)', true)
  doc.body?.appendChild(overlay)
}

function clearEPUBHighlights(doc: Document) {
  doc.getElementById('samrai-highlight-overlay')?.remove()
  const win = doc.defaultView
  const css = (win?.CSS as unknown as { highlights?: Map<string, unknown> })?.highlights
  if (css) {
    for (const color of annotationColors) css.delete(`samrai-${color}`)
    css.delete('samrai-search')
  }
}

function focusEPUBRange(range: Range, frameState: FrameState, applyColumn: (column?: number) => void) {
  const rect = range.getBoundingClientRect()
  const win = range.startContainer.ownerDocument?.defaultView
  if (!win) return
  const horizontal = rect.left + win.scrollX
  const column = clamp(Math.floor(horizontal / Math.max(1, frameState.viewportWidth)), 0, Math.max(0, frameState.columnCount - 1))
  applyColumn(column)
}

function clearEPUBSelection(win?: Window | null) {
  win?.getSelection()?.removeAllRanges()
}

function handleEPUBLink(href: string, publication: EPUBPublication, currentIndex: number, navigate: (index: number, column?: number, fragment?: string) => void) {
  try {
    const url = new URL(href, window.location.origin)
    if (url.origin !== window.location.origin) return
    const target = publication.spine.find((item) => new URL(item.content_url, window.location.origin).pathname === url.pathname)
    if (target) navigate(target.index, target.index === currentIndex ? undefined : 0, decodeURIComponent(url.hash.replace(/^#/, '')))
  } catch {
    // Invalid links are ignored inside the sandboxed document.
  }
}

function cssEscape(value: string) {
  if (typeof CSS !== 'undefined' && typeof CSS.escape === 'function') return CSS.escape(value)
  return value.replace(/[^a-zA-Z0-9_-]/g, (character) => `\\${character}`)
}

function parseViewport(content: string) {
  const result: { width?: number; height?: number } = {}
  for (const part of content.split(',')) {
    const [name, rawValue] = part.split('=').map((value) => value.trim().toLowerCase())
    const value = Number(rawValue)
    if (name === 'width' && Number.isFinite(value)) result.width = value
    if (name === 'height' && Number.isFinite(value)) result.height = value
  }
  return result
}

function parseSavedLocation(value: Book['reading_location']): EPUBReadingLocation | null {
  if (!value || typeof value !== 'object') return null
  const spine = Number(value.spine_index)
  if (!Number.isInteger(spine)) return null
  return {
    spine_index: spine,
    column_index: Number.isFinite(Number(value.column_index)) ? Number(value.column_index) : 0,
    column_count: Number.isFinite(Number(value.column_count)) ? Number(value.column_count) : 1,
    chapter_progress: Number.isFinite(Number(value.chapter_progress)) ? Number(value.chapter_progress) : 0,
  }
}

function themePalette(theme: ReaderTheme) {
  switch (theme) {
    case 'white': return { background: '#ffffff', page: '#ffffff', surround: '#e5e7eb', text: '#17191f', link: '#4f46c8' }
    case 'dark': return { background: '#1d2029', page: '#1d2029', surround: '#11131a', text: '#e8e8ed', link: '#a99eff' }
    case 'oled': return { background: '#000000', page: '#000000', surround: '#000000', text: '#eeeeef', link: '#b4a9ff' }
    default: return { background: '#f4ecd9', page: '#f4ecd9', surround: '#d7cdbc', text: '#29251f', link: '#6750b8' }
  }
}

function themeLabel(theme: ReaderTheme) {
  return theme === 'paper' ? 'Paper' : theme === 'white' ? 'White' : theme === 'dark' ? 'Dark' : 'OLED'
}

function readerWidthLabel(width: ReaderWidth) {
  return width === 'compact' ? 'Compact' : width === 'wide' ? 'Wide' : 'Comfortable'
}

function readerFontLabel(font: ReaderFont) {
  return font === 'publisher' ? 'Original' : font === 'sans' ? 'Sans serif' : 'Serif'
}

function readTheme(): ReaderTheme {
  const value = localStorage.getItem(themeKey)
  return value === 'white' || value === 'dark' || value === 'oled' ? value : 'paper'
}

function readReaderWidth(): ReaderWidth {
  const value = localStorage.getItem(widthKey)
  return value === 'compact' || value === 'wide' ? value : 'comfortable'
}

function readReaderFont(): ReaderFont {
  const value = localStorage.getItem(fontKey)
  return value === 'publisher' || value === 'sans' ? value : 'serif'
}

function readReaderAlignment(): ReaderAlignment {
  return localStorage.getItem(alignmentKey) === 'left' ? 'left' : 'justify'
}

function readNumber(key: string, fallback: number, minimum: number, maximum: number) {
  const value = Number(localStorage.getItem(key))
  return Number.isFinite(value) ? clamp(value, minimum, maximum) : fallback
}

function clamp(value: number, minimum: number, maximum: number) {
  return Math.min(maximum, Math.max(minimum, value))
}

async function toggleFullscreen(element: HTMLElement | null) {
  if (!element) return
  if (document.fullscreenElement) await document.exitFullscreen().catch(() => undefined)
  else await element.requestFullscreen().catch(() => undefined)
}

function EPUBStatus({ title, message, error, actionLabel, onAction }: { title: string; message: string; error?: boolean; actionLabel?: string; onAction?: () => void }) {
  return <main className="reader reader-loading">{error ? <AlertIcon /> : <span className="spinner" />}<strong>{title}</strong><p>{message}</p>{actionLabel && onAction ? <button className="button button-secondary" onClick={onAction}>{actionLabel}</button> : null}</main>
}
