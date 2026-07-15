import { useEffect, useMemo, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api, type Book, type BookQuery, type LibraryStats, type Series } from '../../lib/api'
import { AlertIcon, BackIcon, CheckIcon, ChevronRightIcon, HeartIcon, LibraryIcon, SeriesIcon, SparklesIcon, UploadIcon } from '../../components/Icons'

export function HomeView({
  canUpload,
  onUpload,
  onOpenBook,
  onOpenSeries,
  onBrowseBooks,
  onBrowseSeries,
}: {
  canUpload: boolean
  onUpload: () => void
  onOpenBook: (id: number) => void
  onOpenSeries: (id: number) => void
  onBrowseBooks: () => void
  onBrowseSeries: () => void
}) {
  const dashboard = useQuery({ queryKey: ['dashboard'], queryFn: api.dashboard })

  if (dashboard.isPending) return <LibraryLoading />
  if (dashboard.isError || !dashboard.data) return <InlineError onRetry={() => void dashboard.refetch()} />

  const { stats, continue_reading: continueReading, recently_added: recentlyAdded, series } = dashboard.data
  if (stats.total_books === 0) return <EmptyLibrary canUpload={canUpload} onUpload={onUpload} />

  return (
    <>
      <header className="page-heading home-heading">
        <div>
          <span className="eyebrow">Overview</span>
          <h1>Happy reading</h1>
          <p>Continue where you left off or choose something new from your collection.</p>
        </div>
        {canUpload ? <button className="button button-primary" onClick={onUpload} type="button"><UploadIcon />Add book</button> : null}
      </header>

      <StatsGrid stats={stats} />

      {continueReading.length > 0 ? (
        <LibrarySection title="Continue reading" subtitle="Your most recent reads" action="View all" onAction={onBrowseBooks}>
          <div className="continue-grid">
            {continueReading.slice(0, 6).map((book) => <ContinueCard book={book} key={book.id} onOpen={() => onOpenBook(book.id)} />)}
          </div>
        </LibrarySection>
      ) : null}

      <LibrarySection title="Recently added" subtitle="The newest titles in your library" action="All books" onAction={onBrowseBooks}>
        <BookGrid books={recentlyAdded.slice(0, 8)} onOpen={onOpenBook} />
      </LibrarySection>

      {series.length > 0 ? (
        <LibrarySection title="Series" subtitle="Collections grouped by volume and edition" action="All series" onAction={onBrowseSeries}>
          <SeriesGrid series={series.slice(0, 6)} onOpen={onOpenSeries} />
        </LibrarySection>
      ) : null}
    </>
  )
}

export function BooksView({
  search,
  onOpenBook,
  canUpload,
  onUpload,
}: {
  search: string
  onOpenBook: (id: number) => void
  canUpload: boolean
  onUpload: () => void
}) {
  const queryClient = useQueryClient()
  const [status, setStatus] = useState<BookQuery['status']>('all')
  const [sort, setSort] = useState<BookQuery['sort']>('recent')
  const [selected, setSelected] = useState<Set<number>>(() => new Set())
  const deferredSearch = useDebouncedValue(search, 220)
  const books = useQuery({
    queryKey: ['books', { search: deferredSearch, status, sort }],
    queryFn: () => api.books({ search: deferredSearch, status, sort, limit: 500 }),
  })
  const completion = useMutation({
    mutationFn: ({ bookIds, completed }: { bookIds: number[]; completed: boolean }) => api.setBooksCompletion(bookIds, completed),
    onSuccess: () => {
      setSelected(new Set())
      invalidateReadingQueries(queryClient)
    },
  })

  const visibleIDs = useMemo(() => new Set(books.data?.items.map((book) => book.id) ?? []), [books.data?.items])
  useEffect(() => {
    setSelected((current) => {
      const next = new Set(Array.from(current).filter((id) => visibleIDs.has(id)))
      return next.size === current.size ? current : next
    })
  }, [visibleIDs])

  const title = deferredSearch ? `Results for “${deferredSearch}”` : 'My books'
  const allVisibleSelected = visibleIDs.size > 0 && selected.size === visibleIDs.size
  function toggleSelection(bookID: number) {
    setSelected((current) => {
      const next = new Set(current)
      if (next.has(bookID)) next.delete(bookID)
      else next.add(bookID)
      return next
    })
  }
  function toggleAllVisible() {
    setSelected(allVisibleSelected ? new Set() : new Set(visibleIDs))
  }

  return (
    <>
      <header className="page-heading">
        <div>
          <span className="eyebrow">Your collection</span>
          <h1>{title}</h1>
          <p>{books.data ? `${books.data.total} ${books.data.total === 1 ? 'title found' : 'titles found'}` : 'Organize manga, comics, and illustrated books.'}</p>
        </div>
        {canUpload ? <button className="button button-primary" onClick={onUpload} type="button"><UploadIcon />Add book</button> : null}
      </header>

      <div className="catalog-toolbar">
        <div className="filter-tabs" role="group" aria-label="Filter books">
          {([
            ['all', 'All'],
            ['unread', 'Not started'],
            ['reading', 'In progress'],
            ['completed', 'Finished'],
          ] as const).map(([value, label]) => (
            <button className={status === value ? 'filter-tab filter-tab-active' : 'filter-tab'} key={value} onClick={() => setStatus(value)} type="button">{label}</button>
          ))}
        </div>
        <label className="sort-control">
          <span>Sort by</span>
          <select value={sort} onChange={(event) => setSort(event.target.value as BookQuery['sort'])}>
            <option value="recent">Most recent</option>
            <option value="oldest">Oldest</option>
            <option value="title">Title</option>
            <option value="series">Series and volume</option>
            <option value="progress">Last read</option>
            <option value="year">Publication year</option>
          </select>
        </label>
      </div>

      {books.data && books.data.items.length > 0 ? (
        <ReadingSelectionToolbar
          allSelected={allVisibleSelected}
          busy={completion.isPending}
          count={selected.size}
          error={completion.isError}
          onClear={() => setSelected(new Set())}
          onMarkRead={() => completion.mutate({ bookIds: Array.from(selected), completed: true })}
          onMarkUnread={() => completion.mutate({ bookIds: Array.from(selected), completed: false })}
          onToggleAll={toggleAllVisible}
          total={books.data.items.length}
        />
      ) : null}

      {books.isPending ? <LibraryLoading /> : null}
      {books.isError ? <InlineError onRetry={() => void books.refetch()} /> : null}
      {books.data && books.data.items.length === 0 ? <NoResults search={deferredSearch} /> : null}
      {books.data && books.data.items.length > 0 ? <BookGrid books={books.data.items} onOpen={onOpenBook} onToggleSelection={toggleSelection} selectedIDs={selected} /> : null}
    </>
  )
}

export function SeriesView({ search, onOpenSeries }: { search: string; onOpenSeries: (id: number) => void }) {
  const [sort, setSort] = useState<'title' | 'recent' | 'count'>('title')
  const deferredSearch = useDebouncedValue(search, 220)
  const query = useQuery({
    queryKey: ['series', { search: deferredSearch, sort }],
    queryFn: () => api.series({ search: deferredSearch, sort, limit: 500 }),
  })

  return (
    <>
      <header className="page-heading">
        <div>
          <span className="eyebrow">Collections</span>
          <h1>{deferredSearch ? `Series matching “${deferredSearch}”` : 'Series'}</h1>
          <p>{query.data ? `${query.data.total} series` : 'Volumes and editions gathered in one place.'}</p>
        </div>
      </header>
      <div className="catalog-toolbar catalog-toolbar-end">
        <label className="sort-control">
          <span>Sort by</span>
          <select value={sort} onChange={(event) => setSort(event.target.value as typeof sort)}>
            <option value="title">Name</option>
            <option value="recent">Recently updated</option>
            <option value="count">Book count</option>
          </select>
        </label>
      </div>
      {query.isPending ? <SeriesLoading /> : null}
      {query.isError ? <InlineError onRetry={() => void query.refetch()} /> : null}
      {query.data && query.data.items.length === 0 ? <NoResults search={deferredSearch} series /> : null}
      {query.data && query.data.items.length > 0 ? <SeriesGrid series={query.data.items} onOpen={onOpenSeries} /> : null}
    </>
  )
}

export function SeriesDetailView({
  seriesId,
  onBack,
  onOpenBook,
}: {
  seriesId: number
  onBack: () => void
  onOpenBook: (id: number) => void
}) {
  const queryClient = useQueryClient()
  const [selected, setSelected] = useState<Set<number>>(() => new Set())
  const query = useQuery({ queryKey: ['series-detail', seriesId], queryFn: () => api.seriesDetail(seriesId) })
  const favorite = useMutation({
    mutationFn: () => api.setSeriesFavorite(seriesId, !query.data?.series.favorite),
    onSuccess: ({ favorite }) => {
      queryClient.setQueryData(['series-detail', seriesId], (current: typeof query.data) => current ? { ...current, series: { ...current.series, favorite } } : current)
      void queryClient.invalidateQueries({ queryKey: ['series'] })
      void queryClient.invalidateQueries({ queryKey: ['favorites'] })
    },
  })
  const completion = useMutation({
    mutationFn: ({ bookIds, completed }: { bookIds: number[]; completed: boolean }) => api.setBooksCompletion(bookIds, completed),
    onSuccess: () => {
      setSelected(new Set())
      invalidateReadingQueries(queryClient)
    },
  })

  const visibleIDs = useMemo(() => new Set(query.data?.books.map((book) => book.id) ?? []), [query.data?.books])
  useEffect(() => {
    setSelected((current) => {
      const next = new Set(Array.from(current).filter((id) => visibleIDs.has(id)))
      return next.size === current.size ? current : next
    })
  }, [visibleIDs])

  if (query.isPending) return <LibraryLoading />
  if (query.isError || !query.data) return <InlineError onRetry={() => void query.refetch()} />

  const { series, books } = query.data
  const allSeriesCompleted = series.book_count > 0 && series.completed_count === series.book_count
  const allSelected = selected.size > 0 && selected.size === books.length
  function toggleSelection(bookID: number) {
    setSelected((current) => {
      const next = new Set(current)
      if (next.has(bookID)) next.delete(bookID)
      else next.add(bookID)
      return next
    })
  }

  return (
    <>
      <div className="series-detail-actions">
        <button className="button button-secondary series-back" onClick={onBack} type="button"><BackIcon />All series</button>
        <div className="series-detail-action-group">
          <button className="button button-secondary" disabled={completion.isPending} onClick={() => completion.mutate({ bookIds: books.map((book) => book.id), completed: !allSeriesCompleted })} type="button"><CheckIcon />{allSeriesCompleted ? 'Mark series as unread' : 'Mark series as read'}</button>
          <button aria-pressed={series.favorite} className={`button button-secondary${series.favorite ? ' favorite-action-active' : ''}`} disabled={favorite.isPending} onClick={() => favorite.mutate()} type="button"><HeartIcon />{series.favorite ? 'Favorite' : 'Add series to favorites'}</button>
        </div>
      </div>
      <section className="series-hero">
        <div className="series-hero-cover">
          {series.cover_url ? <img alt={`Cover of ${series.title}`} src={series.cover_url} /> : <SeriesIcon />}
        </div>
        <div className="series-hero-copy">
          <span className="eyebrow">Series</span>
          <h1>{series.title}</h1>
          <p>{series.description || `${series.book_count} ${series.book_count === 1 ? 'book' : 'books'} · ${series.page_count} pages total`}</p>
          <div className="series-stat-row">
            <span><strong>{series.book_count}</strong><small>volumes</small></span>
            <span><strong>{series.started_count}</strong><small>started</small></span>
            <span><strong>{series.completed_count}</strong><small>completed</small></span>
          </div>
        </div>
      </section>
      <LibrarySection title="Volumes and editions" subtitle="Sorted by volume, number, and title">
        <ReadingSelectionToolbar
          allSelected={allSelected}
          busy={completion.isPending}
          count={selected.size}
          error={completion.isError}
          onClear={() => setSelected(new Set())}
          onMarkRead={() => completion.mutate({ bookIds: Array.from(selected), completed: true })}
          onMarkUnread={() => completion.mutate({ bookIds: Array.from(selected), completed: false })}
          onToggleAll={() => setSelected(allSelected ? new Set() : new Set(books.map((book) => book.id)))}
          total={books.length}
        />
        <BookGrid books={books} onOpen={onOpenBook} onToggleSelection={toggleSelection} selectedIDs={selected} showVolume />
      </LibrarySection>
    </>
  )
}


function ReadingSelectionToolbar({
  total,
  count,
  allSelected,
  busy,
  error,
  onToggleAll,
  onClear,
  onMarkRead,
  onMarkUnread,
}: {
  total: number
  count: number
  allSelected: boolean
  busy: boolean
  error: boolean
  onToggleAll: () => void
  onClear: () => void
  onMarkRead: () => void
  onMarkUnread: () => void
}) {
  return (
    <div className="reading-selection-toolbar" aria-label="Bulk reading status">
      <div>
        <strong>{count > 0 ? `${count} selected` : 'Select books to update their reading status'}</strong>
        <small>{total} {total === 1 ? 'book' : 'books'} in this view</small>
      </div>
      <div className="reading-selection-actions">
        <button className="button button-secondary" disabled={busy} onClick={onToggleAll} type="button">{allSelected ? 'Deselect all' : 'Select all'}</button>
        {count > 0 ? <button className="button button-secondary" disabled={busy} onClick={onClear} type="button">Clear</button> : null}
        <button className="button button-secondary" disabled={busy || count === 0} onClick={onMarkUnread} type="button">Mark as unread</button>
        <button className="button button-primary" disabled={busy || count === 0} onClick={onMarkRead} type="button"><CheckIcon />Mark as read</button>
      </div>
      {error ? <p className="form-error">Could not update the selected books.</p> : null}
    </div>
  )
}

function invalidateReadingQueries(queryClient: ReturnType<typeof useQueryClient>) {
  void queryClient.invalidateQueries({ queryKey: ['book'] })
  void queryClient.invalidateQueries({ queryKey: ['books'] })
  void queryClient.invalidateQueries({ queryKey: ['dashboard'] })
  void queryClient.invalidateQueries({ queryKey: ['series'] })
  void queryClient.invalidateQueries({ queryKey: ['series-detail'] })
}

function StatsGrid({ stats }: { stats: LibraryStats }) {
  return (
    <section className="overview-grid overview-grid-four" aria-label="Library summary">
      <article className="metric-card"><span>Books</span><strong>{stats.total_books}</strong><small>in the library</small></article>
      <article className="metric-card"><span>In progress</span><strong>{stats.reading_books}</strong><small>active reads</small></article>
      <article className="metric-card"><span>Finished</span><strong>{stats.completed_books}</strong><small>finished books</small></article>
      <article className="metric-card"><span>Series</span><strong>{stats.total_series}</strong><small>organized collections</small></article>
    </section>
  )
}

function LibrarySection({
  title,
  subtitle,
  action,
  onAction,
  children,
}: {
  title: string
  subtitle: string
  action?: string
  onAction?: () => void
  children: React.ReactNode
}) {
  return (
    <section className="library-section">
      <header className="section-heading">
        <div><h2>{title}</h2><p>{subtitle}</p></div>
        {action && onAction ? <button className="section-action" onClick={onAction} type="button">{action}<ChevronRightIcon /></button> : null}
      </header>
      {children}
    </section>
  )
}

function ContinueCard({ book, onOpen }: { book: Book; onOpen: () => void }) {
  const progress = book.page_count > 0 ? Math.max(1, Math.round(((book.current_page + 1) / book.page_count) * 100)) : 0
  return (
    <button className="continue-card" onClick={onOpen} type="button">
      <img alt={`Cover of ${book.title}`} src={book.cover_url} />
      <span className="continue-copy">
        {book.series ? <small>{book.series}</small> : <small>Continue reading</small>}
        <strong>{book.title}</strong>
        <span>Page {book.current_page + 1} of {book.page_count}</span>
        <i><b style={{ width: `${progress}%` }} /></i>
      </span>
      <ChevronRightIcon />
    </button>
  )
}

export function BookGrid({
  books,
  onOpen,
  showVolume = false,
  selectedIDs,
  onToggleSelection,
}: {
  books: Book[]
  onOpen: (id: number) => void
  showVolume?: boolean
  selectedIDs?: Set<number>
  onToggleSelection?: (id: number) => void
}) {
  return (
    <section className="book-grid" aria-label="Library books">
      {books.map((book) => (
        <BookCard
          book={book}
          key={book.id}
          onOpen={() => onOpen(book.id)}
          onToggleSelection={onToggleSelection ? () => onToggleSelection(book.id) : undefined}
          selected={selectedIDs?.has(book.id) ?? false}
          showVolume={showVolume}
        />
      ))}
    </section>
  )
}

function BookCard({
  book,
  onOpen,
  showVolume,
  selected,
  onToggleSelection,
}: {
  book: Book
  onOpen: () => void
  showVolume: boolean
  selected: boolean
  onToggleSelection?: () => void
}) {
  const queryClient = useQueryClient()
  const favorite = useMutation({
    mutationFn: () => api.setBookFavorite(book.id, !book.favorite),
    onSuccess: ({ favorite }) => {
      queryClient.setQueryData<Book>(['book', book.id], (current) => current ? { ...current, favorite } : current)
      void queryClient.invalidateQueries({ queryKey: ['books'] })
      void queryClient.invalidateQueries({ queryKey: ['dashboard'] })
      void queryClient.invalidateQueries({ queryKey: ['favorites'] })
    },
  })
  const progress = book.completed ? 100 : book.started && book.page_count > 0 ? Math.round(((book.current_page + 1) / book.page_count) * 100) : 0
  const volumeLabel = [book.volume ? `Vol. ${book.volume}` : '', book.number ? `#${book.number}` : ''].filter(Boolean).join(' · ')
  return (
    <article className={`library-book${selected ? ' library-book-selected' : ''}`}>
      {onToggleSelection ? <button aria-label={selected ? `Deselect ${book.title}` : `Select ${book.title}`} aria-pressed={selected} className={`book-selection-button${selected ? ' book-selection-button-active' : ''}`} onClick={onToggleSelection} type="button"><CheckIcon /></button> : null}
      <button aria-label={book.favorite ? 'Remove from favorites' : 'Add to favorites'} aria-pressed={book.favorite} className={`favorite-button${book.favorite ? ' favorite-button-active' : ''}`} disabled={favorite.isPending} onClick={() => favorite.mutate()} type="button"><HeartIcon /></button>
      <button className="library-cover library-cover-button" onClick={onOpen} type="button">
        <img alt={`Cover of ${book.title}`} loading="lazy" src={book.cover_url} />
        <span className="page-pill">{showVolume && volumeLabel ? volumeLabel : bookFormatSummary(book)}</span>
        {book.started ? <span className="cover-progress"><i style={{ width: `${progress}%` }} /></span> : null}
      </button>
      <div className="library-book-copy">
        {book.series ? <small>{book.series}</small> : null}
        <h2 title={book.title}>{book.title}</h2>
        <p>{book.completed ? 'Finished' : book.started ? readingPositionLabel(book) : showVolume && volumeLabel ? volumeLabel : 'Not started'}</p>
      </div>
      <button className="book-open" onClick={onOpen} type="button">
        {book.completed ? 'Read again' : book.started ? 'Continue reading' : 'Start reading'}
        <ChevronRightIcon />
      </button>
    </article>
  )
}

export function SeriesGrid({ series, onOpen }: { series: Series[]; onOpen: (id: number) => void }) {
  return (
    <section className="series-grid" aria-label="Library series">
      {series.map((item) => <SeriesCard item={item} key={item.id} onOpen={() => onOpen(item.id)} />)}
    </section>
  )
}

function SeriesCard({ item, onOpen }: { item: Series; onOpen: () => void }) {
  const queryClient = useQueryClient()
  const favorite = useMutation({
    mutationFn: () => api.setSeriesFavorite(item.id, !item.favorite),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['series'] })
      void queryClient.invalidateQueries({ queryKey: ['series-detail', item.id] })
      void queryClient.invalidateQueries({ queryKey: ['favorites'] })
    },
  })
  return <article className="series-card-shell">
    <button aria-label={item.favorite ? 'Remove series from favorites' : 'Add series to favorites'} aria-pressed={item.favorite} className={`favorite-button series-favorite${item.favorite ? ' favorite-button-active' : ''}`} disabled={favorite.isPending} onClick={() => favorite.mutate()} type="button"><HeartIcon /></button>
    <button className="series-card" onClick={onOpen} type="button">
      <span className="series-cover-stack">
        <i className="series-sheet series-sheet-back" />
        <i className="series-sheet series-sheet-middle" />
        {item.cover_url ? <img alt={`Cover of ${item.title}`} loading="lazy" src={item.cover_url} /> : <span className="series-placeholder"><SeriesIcon /></span>}
      </span>
      <span className="series-card-copy"><small>{item.book_count} {item.book_count === 1 ? 'book' : 'books'}</small><strong>{item.title}</strong><span>{item.completed_count === item.book_count && item.book_count > 0 ? 'Series complete' : item.started_count > 0 ? `${item.started_count} started` : `${item.page_count} pages`}</span></span>
      <ChevronRightIcon />
    </button>
  </article>
}

function EmptyLibrary({ onUpload, canUpload }: { onUpload: () => void; canUpload: boolean }) {
  return (
    <section className="empty-library">
      <div className="empty-art" aria-hidden="true"><span className="empty-book empty-book-one" /><span className="empty-book empty-book-two" /><span className="empty-book empty-book-three" /></div>
      <span className="empty-badge"><SparklesIcon />Library ready</span>
      <h2>Add your first book</h2>
      <p>Upload CBZ, CBR, CB7, CBT, PDF, or EPUB files from the browser. Comic archives share the same fast, paged reader.</p>
      {canUpload ? <button className="button button-primary" onClick={onUpload} type="button"><UploadIcon />Select file</button> : null}
    </section>
  )
}

function bookFormatSummary(book: Book) {
  if (book.format === 'epub') {
    const layout = book.epub_layout === 'fixed' ? 'fixed' : 'reflowable'
    return `EPUB ${layout} · ${book.page_count} ${book.epub_layout === 'fixed' ? 'pages' : 'sections'}`
  }
  if (book.format === 'pdf') return `PDF · ${book.page_count} pages`
  return `${book.format.toUpperCase()} · ${book.page_count} pages`
}

function readingPositionLabel(book: Book) {
  if (book.format === 'epub' && book.epub_layout !== 'fixed') return `Section ${book.current_page + 1} of ${book.page_count}`
  return `Page ${book.current_page + 1} of ${book.page_count}`
}

function NoResults({ search, series = false }: { search: string; series?: boolean }) {
  return (
    <section className="no-results">
      {series ? <SeriesIcon /> : <LibraryIcon />}
      <h2>{search ? 'No results found' : series ? 'No series yet' : 'No books in this filter'}</h2>
      <p>{search ? `Nothing matched “${search}”. Try another search.` : series ? 'Books with the same series name will be grouped here.' : 'Choose another filter to see the rest of the library.'}</p>
    </section>
  )
}

function InlineError({ onRetry }: { onRetry: () => void }) {
  return (
    <section className="inline-error">
      <AlertIcon />
      <div><strong>Could not load this section</strong><p>Check the server connection and try again.</p></div>
      <button className="button button-secondary" onClick={onRetry} type="button">Try again</button>
    </section>
  )
}

function LibraryLoading() {
  return <section className="library-loading" aria-label="Loading library">{Array.from({ length: 8 }, (_, index) => <span className="book-skeleton" key={index} />)}</section>
}

function SeriesLoading() {
  return <section className="series-grid" aria-label="Loading series">{Array.from({ length: 6 }, (_, index) => <span className="series-skeleton" key={index} />)}</section>
}

function useDebouncedValue(value: string, delay: number) {
  const [debounced, setDebounced] = useState(value)
  useEffect(() => {
    const timer = window.setTimeout(() => setDebounced(value.trim()), delay)
    return () => window.clearTimeout(timer)
  }, [delay, value])
  return debounced
}
