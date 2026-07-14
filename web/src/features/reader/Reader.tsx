import { lazy, Suspense, useCallback, useEffect, useRef, useState, type CSSProperties } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { api, newReadingSessionID, pageImageURL, type Book, type BooksResponse } from '../../lib/api'
import {
  AlertIcon,
  ArrowLeftIcon,
  ArrowRightIcon,
  BackIcon,
  CheckIcon,
  DirectionIcon,
  FitIcon,
  FullscreenExitIcon,
  FullscreenIcon,
  SettingsIcon,
} from '../../components/Icons'

type ReadingDirection = 'ltr' | 'rtl'
type PageFit = 'contain' | 'width' | 'height'
type ReaderBackground = 'black' | 'gray' | 'paper'

type CachedPage = {
  image: HTMLImageElement
  promise: Promise<HTMLImageElement>
}

const directionStorageKey = 'samrai.reader.direction'
const fitStorageKey = 'samrai.reader.fit'
const backgroundStorageKey = 'samrai.reader.background'

const PdfReader = lazy(() => import('../pdf/PdfReader').then((module) => ({ default: module.PdfReader })))
const EpubReader = lazy(() => import('../epub/EpubReader').then((module) => ({ default: module.EpubReader })))


export function Reader({ bookId, onClose }: { bookId: number; onClose: () => void }) {
  const bookQuery = useQuery({ queryKey: ['book', bookId], queryFn: () => api.book(bookId), retry: 1 })
  if (bookQuery.isPending) {
    return <main className="reader reader-loading"><span className="spinner" /><strong>Preparing reader…</strong></main>
  }
  if (bookQuery.isError || !bookQuery.data) {
    return <main className="reader reader-loading"><AlertIcon /><strong>Could not open this book.</strong><button className="button button-secondary" onClick={onClose}>Back</button></main>
  }
  if (bookQuery.data.format === 'pdf') {
    return <Suspense fallback={<main className="reader reader-loading"><span className="spinner" /><strong>Loading PDF reader…</strong></main>}><PdfReader book={bookQuery.data} onClose={onClose} /></Suspense>
  }
  if (bookQuery.data.format === 'epub') {
    return <Suspense fallback={<main className="reader reader-loading"><span className="spinner" /><strong>Loading EPUB reader…</strong></main>}><EpubReader book={bookQuery.data} onClose={onClose} /></Suspense>
  }
  return <ComicReader bookId={bookId} onClose={onClose} />
}

function ComicReader({
  bookId,
  onClose,
}: {
  bookId: number
  onClose: () => void
}) {
  const queryClient = useQueryClient()
  const readerRef = useRef<HTMLDivElement>(null)
  const imageHostRef = useRef<HTMLDivElement>(null)
  const cacheRef = useRef(new Map<string, CachedPage>())
  const requestSequenceRef = useRef(0)
  const initializedBookRef = useRef<number | null>(null)
  const saveTimerRef = useRef<number | null>(null)
  const lastSavedPageRef = useRef<number | null>(null)
  const sessionIDRef = useRef(newReadingSessionID(`comic-${bookId}`))
  const sessionRecordedRef = useRef(false)

  const bookQuery = useQuery({
    queryKey: ['book', bookId],
    queryFn: () => api.book(bookId),
    retry: 1,
    staleTime: 30_000,
  })

  const [visiblePage, setVisiblePage] = useState<number | null>(null)
  const [pendingPage, setPendingPage] = useState<number | null>(null)
  const [loadError, setLoadError] = useState('')
  const [failedPage, setFailedPage] = useState<number | null>(null)
  const [controlsVisible, setControlsVisible] = useState(true)
  const [settingsOpen, setSettingsOpen] = useState(false)
  const [direction, setDirection] = useState<ReadingDirection>(() => readDirection())
  const [fit, setFit] = useState<PageFit>(() => readFit())
  const [background, setBackground] = useState<ReaderBackground>(() => readBackground())
  const [fullscreen, setFullscreen] = useState(false)
  const [saveState, setSaveState] = useState<'idle' | 'saving' | 'saved' | 'error'>('idle')
  const [renderWidth, setRenderWidth] = useState(() => targetPageWidth())

  const book = bookQuery.data

  const ensureDecoded = useCallback((pageNumber: number) => {
    const cacheKey = `${pageNumber}@${renderWidth}`
    const existing = cacheRef.current.get(cacheKey)
    if (existing) return existing.promise

    const image = new Image()
    image.decoding = 'async'
    image.src = pageImageURL(bookId, pageNumber, renderWidth)

    const promise = waitForImage(image).then(async () => {
      if (typeof image.decode === 'function') {
        try {
          await image.decode()
        } catch {
          // The load event already proved the image is valid. Some browsers reject
          // decode() when the decoded frame is already available.
        }
      }
      if (!image.naturalWidth || !image.naturalHeight) {
        throw new Error('The received page does not contain a valid image.')
      }
      return image
    })

    cacheRef.current.set(cacheKey, { image, promise })
    promise.catch(() => cacheRef.current.delete(cacheKey))
    return promise
  }, [bookId, renderWidth])

  const preloadAround = useCallback((pageNumber: number, pageCount: number) => {
    const keep = new Set<string>()
    for (const candidate of [pageNumber - 1, pageNumber, pageNumber + 1, pageNumber + 2]) {
      if (candidate < 0 || candidate >= pageCount) continue
      keep.add(`${candidate}@${renderWidth}`)
      if (candidate !== pageNumber) {
        void ensureDecoded(candidate).catch(() => undefined)
      }
    }

    for (const cachedPage of cacheRef.current.keys()) {
      if (!keep.has(cachedPage)) {
        cacheRef.current.delete(cachedPage)
      }
    }
  }, [ensureDecoded, renderWidth])

  const showPage = useCallback(async (pageNumber: number) => {
    if (!book || pageNumber < 0 || pageNumber >= book.page_count) return

    const requestSequence = ++requestSequenceRef.current
    setPendingPage(pageNumber)
    setLoadError('')
    setFailedPage(null)

    try {
      const image = await ensureDecoded(pageNumber)
      if (requestSequence !== requestSequenceRef.current) return

      image.className = 'reader-page-image'
      image.alt = `Page ${pageNumber + 1} of ${book.title}`
      image.draggable = false
      imageHostRef.current?.replaceChildren(image)
      setVisiblePage(pageNumber)
      setPendingPage(null)
      preloadAround(pageNumber, book.page_count)
    } catch (error) {
      if (requestSequence !== requestSequenceRef.current) return
      setPendingPage(null)
      setFailedPage(pageNumber)
      setLoadError(error instanceof Error ? error.message : 'Could not load this page.')
    }
  }, [book, ensureDecoded, preloadAround])

  useEffect(() => {
    if (!book || initializedBookRef.current === book.id) return

    initializedBookRef.current = book.id
    const initialPage = clamp(book.current_page, 0, Math.max(0, book.page_count - 1))
    const bookDirection = book.reading_direction === 'rtl' ? 'rtl' : book.reading_direction === 'ltr' ? 'ltr' : readDirection()
    setDirection(bookDirection)
    setVisiblePage(null)
    imageHostRef.current?.replaceChildren()
    lastSavedPageRef.current = book.started ? initialPage : null
    void showPage(initialPage)
  }, [book, showPage])

  useEffect(() => {
    return () => {
      requestSequenceRef.current += 1
      cacheRef.current.clear()
      if (saveTimerRef.current !== null) {
        window.clearTimeout(saveTimerRef.current)
      }
      void api.endReadingSession(bookId, sessionIDRef.current)
    }
  }, [])

  const saveProgress = useCallback(async (pageNumber: number) => {
    if (!book || pageNumber < 0 || pageNumber >= book.page_count) return
    if (lastSavedPageRef.current === pageNumber && sessionRecordedRef.current) return

    setSaveState('saving')
    try {
      const progress = await api.saveProgress(book.id, pageNumber, undefined, sessionIDRef.current)
      lastSavedPageRef.current = pageNumber
      sessionRecordedRef.current = true
      setSaveState('saved')

      queryClient.setQueryData<Book>(['book', book.id], (current) => current ? {
        ...current,
        current_page: progress.current_page,
        started: progress.started,
        completed: progress.completed,
      } : current)
      queryClient.setQueryData<BooksResponse>(['books'], (current) => current ? {
        ...current,
        items: current.items.map((item) => item.id === book.id ? {
          ...item,
          current_page: progress.current_page,
          started: progress.started,
          completed: progress.completed,
        } : item),
      } : current)

      window.setTimeout(() => setSaveState((current) => current === 'saved' ? 'idle' : current), 1400)
    } catch {
      setSaveState('error')
    }
  }, [book, queryClient])

  useEffect(() => {
    if (visiblePage === null || !book || (visiblePage === lastSavedPageRef.current && sessionRecordedRef.current)) return
    if (saveTimerRef.current !== null) {
      window.clearTimeout(saveTimerRef.current)
    }
    saveTimerRef.current = window.setTimeout(() => {
      void saveProgress(visiblePage)
    }, 350)
    return () => {
      if (saveTimerRef.current !== null) {
        window.clearTimeout(saveTimerRef.current)
      }
    }
  }, [book, saveProgress, visiblePage])

  useEffect(() => {
    if (!controlsVisible || settingsOpen) return
    const timer = window.setTimeout(() => setControlsVisible(false), 3400)
    return () => window.clearTimeout(timer)
  }, [controlsVisible, settingsOpen, visiblePage])

  useEffect(() => {
    let timer: number | undefined
    const update = () => {
      window.clearTimeout(timer)
      timer = window.setTimeout(() => setRenderWidth(targetPageWidth()), 180)
    }
    window.addEventListener('resize', update)
    return () => {
      window.clearTimeout(timer)
      window.removeEventListener('resize', update)
    }
  }, [])

  useEffect(() => {
    if (visiblePage === null || !book) return
    cacheRef.current.clear()
    void showPage(visiblePage)
    // A resolution change should replace the current image only after the
    // optimized variant has been downloaded and decoded.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [renderWidth])

  useEffect(() => {
    const onFullscreenChange = () => setFullscreen(document.fullscreenElement === readerRef.current)
    document.addEventListener('fullscreenchange', onFullscreenChange)
    return () => document.removeEventListener('fullscreenchange', onFullscreenChange)
  }, [])

  const previousPage = useCallback(() => {
    if (visiblePage === null) return
    void showPage(visiblePage - 1)
  }, [showPage, visiblePage])

  const nextPage = useCallback(() => {
    if (visiblePage === null) return
    void showPage(visiblePage + 1)
  }, [showPage, visiblePage])

  const leftAction = direction === 'rtl' ? nextPage : previousPage
  const rightAction = direction === 'rtl' ? previousPage : nextPage

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
      } else if (event.key === 'Escape' && !document.fullscreenElement) {
        event.preventDefault()
        void closeReader()
      }
    }
    window.addEventListener('keydown', onKeyDown)
    return () => window.removeEventListener('keydown', onKeyDown)
  })

  async function closeReader() {
    if (visiblePage !== null) {
      if (saveTimerRef.current !== null) window.clearTimeout(saveTimerRef.current)
      await saveProgress(visiblePage)
    }
    await api.endReadingSession(bookId, sessionIDRef.current).catch(() => undefined)
    if (document.fullscreenElement) {
      await document.exitFullscreen().catch(() => undefined)
    }
    onClose()
  }

  function updateDirection(value: ReadingDirection) {
    setDirection(value)
    localStorage.setItem(directionStorageKey, value)
    setControlsVisible(true)
  }

  function updateFit(value: PageFit) {
    setFit(value)
    localStorage.setItem(fitStorageKey, value)
    setControlsVisible(true)
  }

  function updateBackground(value: ReaderBackground) {
    setBackground(value)
    localStorage.setItem(backgroundStorageKey, value)
    setControlsVisible(true)
  }

  const leftDisabled = visiblePage === null || (direction === 'ltr' ? visiblePage <= 0 : visiblePage >= (book?.page_count ?? 1) - 1)
  const rightDisabled = visiblePage === null || (direction === 'ltr' ? visiblePage >= (book?.page_count ?? 1) - 1 : visiblePage <= 0)
  const progressPercentage = book && visiblePage !== null ? ((visiblePage + 1) / book.page_count) * 100 : 0

  if (bookQuery.isPending) {
    return <ReaderStatus title="Opening book" message="Preparing your saved page…" />
  }

  if (bookQuery.isError || !book) {
    return (
      <ReaderStatus
        error
        title="Could not open the book"
        message="Make sure the file still exists in the library and try again."
        actionLabel="Back to library"
        onAction={onClose}
      />
    )
  }

  return (
    <div className={`reader reader-bg-${background}`} ref={readerRef}>
      <div className={`reader-stage reader-fit-${fit}`}>
        <div className="reader-image-host" ref={imageHostRef} />
        {visiblePage === null ? (
          <div className="reader-initial-loading">
            <span className="spinner" />
            <strong>Loading page</strong>
          </div>
        ) : null}

        <button
          aria-label={direction === 'rtl' ? 'Next page' : 'Previous page'}
          className="reader-zone reader-zone-left"
          disabled={leftDisabled}
          onClick={() => {
            setControlsVisible(false)
            leftAction()
          }}
          type="button"
        />
        <button
          aria-label={controlsVisible ? 'Ocultar controles' : 'Mostrar controles'}
          className="reader-zone reader-zone-center"
          onClick={() => {
            setSettingsOpen(false)
            setControlsVisible((current) => !current)
          }}
          type="button"
        />
        <button
          aria-label={direction === 'rtl' ? 'Previous page' : 'Next page'}
          className="reader-zone reader-zone-right"
          disabled={rightDisabled}
          onClick={() => {
            setControlsVisible(false)
            rightAction()
          }}
          type="button"
        />
      </div>

      {pendingPage !== null && visiblePage !== null ? (
        <div className="reader-loading-indicator" aria-live="polite">
          <span className="spinner spinner-small" />
          Loading page {pendingPage + 1}
        </div>
      ) : null}

      {loadError ? (
        <div className="reader-page-error" role="alert">
          <AlertIcon />
          <span><strong>Could not load page</strong><small>{loadError}</small></span>
          <button onClick={() => failedPage !== null && void showPage(failedPage)} type="button">Try again</button>
        </div>
      ) : null}

      <div className={`reader-controls${controlsVisible ? ' reader-controls-visible' : ''}`}>
        <header className="reader-topbar">
          <button className="reader-control-button reader-back" onClick={() => void closeReader()} type="button">
            <BackIcon />
            <span>Library</span>
          </button>
          <div className="reader-title">
            <strong>{book.title}</strong>
            <small>{visiblePage === null ? 'Preparing…' : `Page ${visiblePage + 1} of ${book.page_count}`}</small>
          </div>
          <div className="reader-top-actions">
            <span className={`reader-save-state reader-save-${saveState}`}>
              {saveState === 'saving' ? 'Saving…' : saveState === 'saved' ? <><CheckIcon /> Saved</> : saveState === 'error' ? 'Not saved' : null}
            </span>
            <button
              aria-label={fullscreen ? 'Exit full screen' : 'Enter full screen'}
              className="reader-icon-button"
              onClick={() => void toggleFullscreen(readerRef.current)}
              type="button"
            >
              {fullscreen ? <FullscreenExitIcon /> : <FullscreenIcon />}
            </button>
            <button
              aria-expanded={settingsOpen}
              aria-label="Reader settings"
              className={`reader-icon-button${settingsOpen ? ' reader-icon-button-active' : ''}`}
              onClick={() => setSettingsOpen((current) => !current)}
              type="button"
            >
              <SettingsIcon />
            </button>
          </div>
        </header>

        <footer className="reader-bottombar">
          <button aria-label="Previous page" className="reader-icon-button" disabled={visiblePage === 0} onClick={previousPage} type="button">
            <ArrowLeftIcon />
          </button>
          <div className="reader-progress-wrap">
            <input
              aria-label="Reading progress"
              max={Math.max(0, book.page_count - 1)}
              min={0}
              onChange={(event) => void showPage(Number(event.target.value))}
              style={{ '--reader-progress': `${progressPercentage}%` } as CSSProperties}
              type="range"
              value={visiblePage ?? 0}
            />
            <span><strong>{visiblePage === null ? '—' : visiblePage + 1}</strong><i />{book.page_count}</span>
          </div>
          <button aria-label="Next page" className="reader-icon-button" disabled={visiblePage === book.page_count - 1} onClick={nextPage} type="button">
            <ArrowRightIcon />
          </button>
        </footer>

        {settingsOpen ? (
          <aside className="reader-settings" onClick={(event) => event.stopPropagation()}>
            <section>
              <header><DirectionIcon /><span><strong>Direction</strong><small>Sets which side advances or goes back.</small></span></header>
              <div className="reader-segmented">
                <button className={direction === 'ltr' ? 'active' : ''} onClick={() => updateDirection('ltr')} type="button">Western</button>
                <button className={direction === 'rtl' ? 'active' : ''} onClick={() => updateDirection('rtl')} type="button">Manga</button>
              </div>
            </section>
            <section>
              <header><FitIcon /><span><strong>Page fit</strong><small>Choose how the image fills the screen.</small></span></header>
              <div className="reader-segmented reader-segmented-three">
                <button className={fit === 'contain' ? 'active' : ''} onClick={() => updateFit('contain')} type="button">Screen</button>
                <button className={fit === 'width' ? 'active' : ''} onClick={() => updateFit('width')} type="button">Width</button>
                <button className={fit === 'height' ? 'active' : ''} onClick={() => updateFit('height')} type="button">Height</button>
              </div>
            </section>
            <section>
              <header><span className="reader-color-icon" /><span><strong>Background</strong><small>Changes only the area around the page.</small></span></header>
              <div className="reader-background-options">
                {(['black', 'gray', 'paper'] as ReaderBackground[]).map((value) => (
                  <button
                    aria-label={value === 'black' ? 'Black background' : value === 'gray' ? 'Gray background' : 'Paper background'}
                    className={`${value}${background === value ? ' active' : ''}`}
                    key={value}
                    onClick={() => updateBackground(value)}
                    type="button"
                  />
                ))}
              </div>
            </section>
            <p className="reader-shortcuts">←/→ navigate · Space advance · F full screen · Esc exit</p>
          </aside>
        ) : null}
      </div>
    </div>
  )
}

function ReaderStatus({
  title,
  message,
  error,
  actionLabel,
  onAction,
}: {
  title: string
  message: string
  error?: boolean
  actionLabel?: string
  onAction?: () => void
}) {
  return (
    <main className="reader-status">
      <span className={error ? 'reader-status-icon reader-status-error' : 'spinner'}>{error ? <AlertIcon /> : null}</span>
      <h1>{title}</h1>
      <p>{message}</p>
      {actionLabel && onAction ? <button className="button button-primary" onClick={onAction} type="button">{actionLabel}</button> : null}
    </main>
  )
}

function waitForImage(image: HTMLImageElement) {
  if (image.complete) {
    return image.naturalWidth > 0 ? Promise.resolve() : Promise.reject(new Error('The page could not be decoded.'))
  }
  return new Promise<void>((resolve, reject) => {
    image.addEventListener('load', () => resolve(), { once: true })
    image.addEventListener('error', () => reject(new Error('The server could not deliver this page.')), { once: true })
  })
}

async function toggleFullscreen(element: HTMLElement | null) {
  if (!element) return
  if (document.fullscreenElement) {
    await document.exitFullscreen().catch(() => undefined)
  } else {
    await element.requestFullscreen().catch(() => undefined)
  }
}

function readDirection(): ReadingDirection {
  return localStorage.getItem(directionStorageKey) === 'rtl' ? 'rtl' : 'ltr'
}

function readFit(): PageFit {
  const value = localStorage.getItem(fitStorageKey)
  return value === 'width' || value === 'height' ? value : 'contain'
}

function readBackground(): ReaderBackground {
  const value = localStorage.getItem(backgroundStorageKey)
  return value === 'gray' || value === 'paper' ? value : 'black'
}

function targetPageWidth() {
  const density = Math.min(window.devicePixelRatio || 1, 2.5)
  return Math.min(3840, Math.max(480, Math.ceil(window.innerWidth * density)))
}

function clamp(value: number, minimum: number, maximum: number) {
  return Math.min(maximum, Math.max(minimum, value))
}
