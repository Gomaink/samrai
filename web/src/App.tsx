import { DragEvent, FormEvent, useEffect, useMemo, useRef, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ApiError, api, uploadBook, type AuthStatus, type Book, type ImportJob, type User } from './lib/api'
import { Reader } from './features/reader/Reader'
import { BookDetails } from './features/books/BookDetails'
import { BooksView, HomeView, SeriesDetailView, SeriesView } from './features/library/LibraryViews'
import { AnnotationsView, FavoritesView, HistoryView } from './features/library/EngagementViews'
import { SettingsView } from './features/settings/SettingsView'
import { batchItemKey, createBatchUploadItems, type BatchUploadItem } from './features/imports/batch'
import {
  AlertIcon,
  CheckIcon,
  ChevronRightIcon,
  ClockIcon,
  CloseIcon,
  EyeIcon,
  EyeOffIcon,
  HeartIcon,
  HistoryIcon,
  HomeIcon,
  LibraryIcon,
  LogOutIcon,
  MenuIcon,
  NotesIcon,
  SearchIcon,
  SettingsIcon,
  SeriesIcon,
  SparklesIcon,
  TrashIcon,
  RefreshIcon,
  UploadIcon,
} from './components/Icons'

const authStatusKey = ['auth-status'] as const

export function App() {
  const status = useQuery({
    queryKey: authStatusKey,
    queryFn: api.authStatus,
  })

  useEffect(() => {
    if (status.data?.instance?.name) document.title = status.data.instance.name
  }, [status.data?.instance?.name])

  if (status.isPending) {
    return <LoadingScreen />
  }

  if (status.isError) {
    return <ConnectionError error={status.error} onRetry={() => void status.refetch()} />
  }

  const instanceName = status.data.instance?.name || 'samrai'
  if (status.data.setup_required) return <SetupScreen instanceName={instanceName} />
  if (!status.data.authenticated || !status.data.user) return <LoginScreen instanceName={instanceName} />
  return <LibraryShell instanceName={instanceName} user={status.data.user} />
}

function LoadingScreen() {
  return (
    <main className="loading-screen" aria-label="Loading samrai">
      <BrandMark size="large" />
      <div className="loading-copy">
        <strong>samrai</strong>
        <span>starting local library</span>
      </div>
      <span className="spinner" aria-hidden="true" />
    </main>
  )
}

function ConnectionError({ error, onRetry }: { error: Error; onRetry: () => void }) {
  return (
    <main className="status-screen">
      <div className="status-card">
        <BrandMark size="large" />
        <span className="eyebrow">Server unavailable</span>
        <h1>Could not open samrai</h1>
        <p>{error instanceof ApiError ? error.message : 'Make sure the Go server is running on port 8080.'}</p>
        <button className="button button-primary" type="button" onClick={onRetry}>
          Try again
          <ChevronRightIcon />
        </button>
      </div>
    </main>
  )
}

function SetupScreen({ instanceName }: { instanceName: string }) {
  const queryClient = useQueryClient()
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [confirmation, setConfirmation] = useState('')
  const [showPassword, setShowPassword] = useState(false)
  const [localError, setLocalError] = useState('')

  const setup = useMutation({
    mutationFn: () => api.setup(username, password),
    onSuccess: ({ user }) => {
      queryClient.setQueryData<AuthStatus>(authStatusKey, (current) => ({ setup_required: false, authenticated: true, user, instance: current?.instance ?? { name: instanceName, default_reading_direction: 'ltr' } }))
    },
  })

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    setLocalError('')

    if (!/^[A-Za-z0-9][A-Za-z0-9._-]{2,31}$/.test(username.trim())) {
      setLocalError('Use 3 to 32 letters, numbers, periods, hyphens, or underscores.')
      return
    }
    if (password.length < 10) {
      setLocalError('The password must be at least 10 characters long.')
      return
    }
    if (password !== confirmation) {
      setLocalError('The passwords do not match.')
      return
    }

    setup.mutate()
  }

  const error = localError || mutationMessage(setup.error)

  return (
    <AuthLayout
      instanceName={instanceName}
      eyebrow="setup://first_run"
      title="set up your reading space"
      description="Create the administrator account that will manage uploads, metadata, and instance settings."
      footer="local-first · your files stay on your server"
    >
      <form className="auth-form" onSubmit={submit} noValidate>
        <Field label="Username" hint="You will use this name to sign in.">
          <input
            autoComplete="username"
            autoFocus
            maxLength={32}
            minLength={3}
            name="username"
            onChange={(event) => setUsername(event.target.value)}
            placeholder="samuel"
            spellCheck={false}
            value={username}
          />
        </Field>

        <Field label="Password" hint="At least 10 characters.">
          <PasswordInput
            autoComplete="new-password"
            name="password"
            onChange={setPassword}
            show={showPassword}
            toggle={() => setShowPassword((value) => !value)}
            value={password}
          />
        </Field>

        <Field label="Confirm password">
          <PasswordInput
            autoComplete="new-password"
            name="password-confirmation"
            onChange={setConfirmation}
            show={showPassword}
            toggle={() => setShowPassword((value) => !value)}
            value={confirmation}
          />
        </Field>

        {error ? <FormError message={error} /> : null}

        <button className="button button-primary button-full" disabled={setup.isPending} type="submit">
          {setup.isPending ? <span className="spinner spinner-small" aria-hidden="true" /> : <SparklesIcon />}
          {setup.isPending ? 'Creating administrator…' : 'Create administrator'}
        </button>
      </form>
    </AuthLayout>
  )
}

function LoginScreen({ instanceName }: { instanceName: string }) {
  const queryClient = useQueryClient()
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [showPassword, setShowPassword] = useState(false)

  const login = useMutation({
    mutationFn: () => api.login(username, password),
    onSuccess: ({ user }) => {
      queryClient.setQueryData<AuthStatus>(authStatusKey, (current) => ({ setup_required: false, authenticated: true, user, instance: current?.instance ?? { name: instanceName, default_reading_direction: 'ltr' } }))
    },
  })

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    login.mutate()
  }

  return (
    <AuthLayout
      instanceName={instanceName}
      eyebrow="auth://sign_in"
      title="pick up where you left off"
      description="Sign in to open your library and continue reading."
      footer="samrai · self-hosted · distraction-free"
    >
      <form className="auth-form" onSubmit={submit}>
        <Field label="Username">
          <input
            autoComplete="username"
            autoFocus
            name="username"
            onChange={(event) => setUsername(event.target.value)}
            placeholder="Your username"
            required
            spellCheck={false}
            value={username}
          />
        </Field>

        <Field label="Password">
          <PasswordInput
            autoComplete="current-password"
            name="password"
            onChange={setPassword}
            show={showPassword}
            toggle={() => setShowPassword((value) => !value)}
            value={password}
          />
        </Field>

        {login.error ? <FormError message={mutationMessage(login.error)} /> : null}

        <button className="button button-primary button-full" disabled={login.isPending} type="submit">
          {login.isPending ? <span className="spinner spinner-small" aria-hidden="true" /> : null}
          {login.isPending ? 'Signing in…' : 'Sign in'}
          {!login.isPending ? <ChevronRightIcon /> : null}
        </button>
      </form>
    </AuthLayout>
  )
}

function AuthLayout({
  instanceName,
  eyebrow,
  title,
  description,
  footer,
  children,
}: {
  instanceName: string
  eyebrow: string
  title: string
  description: string
  footer: string
  children: React.ReactNode
}) {
  return (
    <main className="auth-page">
      <section className="auth-showcase" aria-hidden="true">
        <div className="showcase-glow showcase-glow-one" />
        <div className="showcase-glow showcase-glow-two" />
        <div className="showcase-content">
          <div className="brand-lockup brand-lockup-light">
            <BrandMark />
            <span>{instanceName}</span>
          </div>
          <div className="showcase-message">
            <span className="showcase-kicker">library://private</span>
            <p>comics, manga, PDFs, and EPUBs in a focused local interface built for reading.</p>
          </div>
          <div className="samrai-console">
            <header><span>samrai://library</span><i>online</i></header>
            <div className="console-line console-line-command"><b>$</b><span>open --continue</span></div>
            <div className="console-book">
              <span className="console-index">01</span>
              <div><small>reading.now</small><strong>crime and punishment</strong><em>page 184 / 672</em></div>
              <span className="console-progress"><i /></span>
            </div>
            <div className="console-line"><b>+</b><span>annotations synced</span><small>24</small></div>
            <div className="console-line"><b>+</b><span>library available</span><small>local</small></div>
            <footer><span>esc to exit</span><span>ctrl+k to search</span></footer>
          </div>
        </div>
      </section>

      <section className="auth-panel">
        <div className="auth-panel-inner">
          <div className="brand-lockup auth-mobile-brand">
            <BrandMark />
            <span>{instanceName}</span>
          </div>
          <header className="auth-header">
            <span className="eyebrow">{eyebrow}</span>
            <h1>{title}</h1>
            <p>{description}</p>
          </header>
          {children}
          <footer className="auth-footer">{footer}</footer>
        </div>
      </section>
    </main>
  )
}

function Field({ label, hint, children }: { label: string; hint?: string; children: React.ReactNode }) {
  return (
    <label className="field">
      <span className="field-heading">
        <strong>{label}</strong>
        {hint ? <small>{hint}</small> : null}
      </span>
      {children}
    </label>
  )
}

function PasswordInput({
  value,
  onChange,
  show,
  toggle,
  name,
  autoComplete,
}: {
  value: string
  onChange: (value: string) => void
  show: boolean
  toggle: () => void
  name: string
  autoComplete: string
}) {
  return (
    <span className="password-input">
      <input
        autoComplete={autoComplete}
        name={name}
        onChange={(event) => onChange(event.target.value)}
        required
        type={show ? 'text' : 'password'}
        value={value}
      />
      <button aria-label={show ? 'Hide password' : 'Show password'} onClick={toggle} tabIndex={-1} type="button">
        {show ? <EyeOffIcon /> : <EyeIcon />}
      </button>
    </span>
  )
}

function FormError({ message }: { message: string }) {
  return <p className="form-error" role="alert">{message}</p>
}

function LibraryShell({ user, instanceName }: { user: User; instanceName: string }) {
  const queryClient = useQueryClient()
  const [displayName, setDisplayName] = useState(instanceName)
  const [sidebarOpen, setSidebarOpen] = useState(false)
  const [uploadOpen, setUploadOpen] = useState(false)
  const [metricsOpen, setMetricsOpen] = useState(false)
  const [search, setSearch] = useState('')
  const searchRef = useRef<HTMLInputElement>(null)
  const [readerBookID, setReaderBookID] = useState<number | null>(() => readerBookIDFromPath())
  const [detailBookID, setDetailBookID] = useState<number | null>(() => detailBookIDFromPath())
  const [libraryRoute, setLibraryRoute] = useState<LibraryRoute>(() => accessibleLibraryRoute(user, window.location.pathname))
  const returnPathRef = useRef('/books')
  const initials = useMemo(() => user.username.slice(0, 2).toUpperCase(), [user.username])
  useEffect(() => { document.title = displayName }, [displayName])

  useEffect(() => {
    const onPopState = () => {
      setReaderBookID(readerBookIDFromPath())
      setDetailBookID(detailBookIDFromPath())
      setLibraryRoute(accessibleLibraryRoute(user, window.location.pathname))
    }
    window.addEventListener('popstate', onPopState)
    return () => window.removeEventListener('popstate', onPopState)
  }, [user])

  useEffect(() => {
    if (user.role !== 'admin' && window.location.pathname === '/settings') {
      window.history.replaceState({}, '', '/')
    }
  }, [user.role])

  useEffect(() => {
    const onKeyDown = (event: KeyboardEvent) => {
      if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === 'k') {
        event.preventDefault()
        searchRef.current?.focus()
      } else if (event.key === '/' && document.activeElement?.tagName !== 'INPUT' && document.activeElement?.tagName !== 'TEXTAREA') {
        event.preventDefault()
        searchRef.current?.focus()
      }
    }
    window.addEventListener('keydown', onKeyDown)
    return () => window.removeEventListener('keydown', onKeyDown)
  }, [])

  function navigate(route: LibraryRoute, path: string) {
    window.history.pushState({}, '', path)
    setReaderBookID(null)
    setDetailBookID(null)
    setLibraryRoute(route)
    setSidebarOpen(false)
  }

  function openHome() {
    setSearch('')
    navigate({ view: 'home' }, '/')
  }

  function openBooks() {
    navigate({ view: 'books' }, '/books')
  }

  function openSeriesList() { navigate({ view: 'series' }, '/series') }
  function openFavorites() { setSearch(''); navigate({ view: 'favorites' }, '/favorites') }
  function openHistory() { setSearch(''); navigate({ view: 'history' }, '/history') }
  function openAnnotations() { setSearch(''); navigate({ view: 'annotations' }, '/annotations') }
  function openSettings() { setSearch(''); navigate({ view: 'settings' }, '/settings') }

  function openSeries(seriesID: number) {
    navigate({ view: 'series-detail', seriesID }, `/series/${seriesID}`)
  }

  function openDetails(bookID: number) {
    returnPathRef.current = libraryPath(libraryRoute)
    window.history.pushState({}, '', `/book/${bookID}`)
    setReaderBookID(null)
    setDetailBookID(bookID)
  }

  function closeDetails() {
    const path = returnPathRef.current || '/books'
    window.history.pushState({}, '', path)
    setDetailBookID(null)
    setLibraryRoute(libraryRouteFromPath(path))
  }

  function openReader(bookID: number) {
    window.history.pushState({}, '', `/read/${bookID}`)
    setDetailBookID(null)
    setReaderBookID(bookID)
  }

  function closeReader() {
    const bookID = readerBookID
    window.history.pushState({}, '', bookID ? `/book/${bookID}` : '/books')
    setReaderBookID(null)
    setDetailBookID(bookID)
    void queryClient.invalidateQueries({ queryKey: ['dashboard'] })
    void queryClient.invalidateQueries({ queryKey: ['books'] })
    void queryClient.invalidateQueries({ queryKey: ['series'] })
  }

  const dashboard = useQuery({ queryKey: ['dashboard'], queryFn: api.dashboard })
  const jobs = useQuery({
    queryKey: ['jobs'],
    queryFn: api.jobs,
    enabled: user.role === 'admin',
    refetchInterval: (query) => {
      const items = query.state.data?.items ?? []
      return items.some((job) => job.status === 'pending' || job.status === 'running') ? 1200 : 5000
    },
  })

  const completedJobSignature = (jobs.data?.items ?? [])
    .filter((job) => job.status === 'completed')
    .map((job) => job.id)
    .join(',')

  useEffect(() => {
    if (!completedJobSignature) return
    void queryClient.invalidateQueries({ queryKey: ['dashboard'] })
    void queryClient.invalidateQueries({ queryKey: ['books'] })
    void queryClient.invalidateQueries({ queryKey: ['series'] })
  }, [completedJobSignature, queryClient])

  const logout = useMutation({
    mutationFn: api.logout,
    onSuccess: () => {
      queryClient.setQueryData<AuthStatus>(authStatusKey, (current) => ({ setup_required: false, authenticated: false, instance: current?.instance ?? { name: displayName, default_reading_direction: 'ltr' } }))
    },
  })

  const jobItems = jobs.data?.items ?? []
  const activeJobs = jobItems.filter((job) => job.status === 'pending' || job.status === 'running')
  const failedJobs = jobItems.filter((job) => job.status === 'failed')
  const totalBooks = dashboard.data?.stats.total_books ?? 0
  const totalSeries = dashboard.data?.stats.total_series ?? 0

  if (readerBookID !== null) {
    return <Reader bookId={readerBookID} onClose={closeReader} />
  }

  if (detailBookID !== null) {
    return <BookDetails bookId={detailBookID} admin={user.role === 'admin'} onBack={closeDetails} onRead={() => openReader(detailBookID)} onDeleted={closeDetails} />
  }

  const hasOwnSearch = ['favorites', 'history', 'annotations', 'settings'].includes(libraryRoute.view)
  const searchPlaceholder = libraryRoute.view === 'settings' ? 'Instance settings' : libraryRoute.view === 'series' || libraryRoute.view === 'series-detail' ? 'Search series…' : 'Search titles, series, and authors…'

  return (
    <div className="app-shell">
      <a className="skip-link" href="#main-content">Skip to content</a>
      {sidebarOpen ? <button className="sidebar-backdrop" aria-label="Close menu" onClick={() => setSidebarOpen(false)} /> : null}
      <aside className={`sidebar${sidebarOpen ? ' sidebar-open' : ''}`}>
        <div className="sidebar-top">
          <div className="brand-lockup">
            <BrandMark />
            <span>{displayName}</span>
          </div>
          <button className="icon-button sidebar-close" aria-label="Close menu" onClick={() => setSidebarOpen(false)} type="button"><CloseIcon /></button>
        </div>

        <nav className="main-nav" aria-label="Primary navigation">
          <span className="nav-label">Library</span>
          <button className={`nav-item${libraryRoute.view === 'home' ? ' nav-item-active' : ''}`} onClick={openHome} type="button"><HomeIcon /><span>Home</span></button>
          <button className={`nav-item${libraryRoute.view === 'books' ? ' nav-item-active' : ''}`} onClick={openBooks} type="button"><LibraryIcon /><span>My books</span><span className="nav-count">{totalBooks}</span></button>
          <button className={`nav-item${libraryRoute.view === 'series' || libraryRoute.view === 'series-detail' ? ' nav-item-active' : ''}`} onClick={openSeriesList} type="button"><SeriesIcon /><span>Series</span>{totalSeries ? <span className="nav-count">{totalSeries}</span> : null}</button>
          <button className={`nav-item${libraryRoute.view === 'favorites' ? ' nav-item-active' : ''}`} onClick={openFavorites} type="button"><HeartIcon /><span>Favorites</span></button>
          <button className={`nav-item${libraryRoute.view === 'history' ? ' nav-item-active' : ''}`} onClick={openHistory} type="button"><HistoryIcon /><span>History</span></button>
          <button className={`nav-item${libraryRoute.view === 'annotations' ? ' nav-item-active' : ''}`} onClick={openAnnotations} type="button"><NotesIcon /><span>Annotations</span></button>
          {user.role === 'admin' ? <button className="nav-item" onClick={() => setUploadOpen(true)} type="button"><UploadIcon /><span>Uploads</span>{activeJobs.length ? <span className="nav-count">{activeJobs.length}</span> : null}</button> : null}
          <span className="nav-label nav-label-spaced">System</span>
          {user.role === 'admin' ? <button className={`nav-item${libraryRoute.view === 'settings' ? ' nav-item-active' : ''}`} onClick={openSettings} type="button"><SettingsIcon /><span>Settings</span></button> : null}
          <button className="nav-item" disabled={user.role !== 'admin'} onClick={() => setMetricsOpen(true)} type="button"><SettingsIcon /><span>Performance</span></button>
        </nav>

        <div className="sidebar-user">
          <span className="avatar">{initials}</span>
          <span className="user-copy"><strong>{user.username}</strong><small>{user.role === 'admin' ? 'Administrator' : 'Reader'}</small></span>
          <button className="icon-button" aria-label="Sign out" disabled={logout.isPending} onClick={() => logout.mutate()} type="button"><LogOutIcon /></button>
        </div>
      </aside>

      <main className="workspace">
        <header className="topbar">
          <button className="icon-button menu-button" aria-label="Open menu" onClick={() => setSidebarOpen(true)} type="button"><MenuIcon /></button>
          <div className="search-box">
            <SearchIcon />
            <input
              aria-label="Search the library"
              onChange={(event) => {
                const value = event.target.value
                setSearch(value)
                if (value.trim() && libraryRoute.view === 'home') openBooks()
                if (value.trim() && libraryRoute.view === 'series-detail') openSeriesList()
              }}
              placeholder={searchPlaceholder}
              disabled={hasOwnSearch}
              ref={searchRef}
              value={search}
            />
            <kbd>Ctrl K</kbd>
          </div>
          <div className="topbar-status"><span /><small>samrai://local</small></div>
        </header>

        <div className="workspace-content" id="main-content" tabIndex={-1}>
          {user.role === 'admin' && (activeJobs.length || failedJobs.length) ? <ImportActivity jobs={jobItems.slice(0, 5)} /> : null}
          {libraryRoute.view === 'home' ? (
            <HomeView canUpload={user.role === 'admin'} onUpload={() => setUploadOpen(true)} onOpenBook={openDetails} onOpenSeries={openSeries} onBrowseBooks={openBooks} onBrowseSeries={openSeriesList} />
          ) : null}
          {libraryRoute.view === 'books' ? <BooksView search={search} onOpenBook={openDetails} canUpload={user.role === 'admin'} onUpload={() => setUploadOpen(true)} /> : null}
          {libraryRoute.view === 'series' ? <SeriesView search={search} onOpenSeries={openSeries} /> : null}
          {libraryRoute.view === 'series-detail' && libraryRoute.seriesID ? <SeriesDetailView seriesId={libraryRoute.seriesID} onBack={openSeriesList} onOpenBook={openDetails} /> : null}
          {libraryRoute.view === 'favorites' ? <FavoritesView onOpenBook={openDetails} onOpenSeries={openSeries} /> : null}
          {libraryRoute.view === 'history' ? <HistoryView onOpenBook={openDetails} /> : null}
          {libraryRoute.view === 'annotations' ? <AnnotationsView onOpenBook={openReader} /> : null}
          {libraryRoute.view === 'settings' && user.role === 'admin' ? <SettingsView currentUser={user} instanceName={displayName} onInstanceNameChange={(name) => { setDisplayName(name); queryClient.setQueryData<AuthStatus>(authStatusKey, current => current ? { ...current, instance: { ...current.instance, name } } : current) }} /> : null}
        </div>
      </main>

      {uploadOpen && user.role === 'admin' ? (
        <UploadDialog
          jobs={jobItems}
          onClose={() => setUploadOpen(false)}
          onQueued={() => void queryClient.invalidateQueries({ queryKey: ['jobs'] })}
        />
      ) : null}
      {metricsOpen && user.role === 'admin' ? <PerformanceDialog onClose={() => setMetricsOpen(false)} /> : null}
    </div>
  )
}

function EmptyLibrary({ onUpload, canUpload }: { onUpload: () => void; canUpload: boolean }) {
  return (
    <section className="empty-library">
      <div className="empty-art" aria-hidden="true">
        <span className="empty-book empty-book-one" />
        <span className="empty-book empty-book-two" />
        <span className="empty-book empty-book-three" />
      </div>
      <span className="empty-badge"><SparklesIcon />Library ready</span>
      <h2>Add your first book</h2>
      <p>Upload CBZ, CBR, CB7, CBT, PDF, or EPUB files from your browser. All comic formats use the same paged reader.</p>
      {canUpload ? <button className="button button-primary" onClick={onUpload} type="button"><UploadIcon />Select file</button> : null}
    </section>
  )
}

function LibraryLoading() {
  return (
    <section className="library-loading" aria-label="Loading library">
      {Array.from({ length: 6 }, (_, index) => <span className="book-skeleton" key={index} />)}
    </section>
  )
}

function InlineError({ message, onRetry }: { message: string; onRetry: () => void }) {
  return (
    <section className="inline-error">
      <AlertIcon />
      <div><strong>Could not load this section</strong><p>{message || 'Try again.'}</p></div>
      <button className="button button-secondary" onClick={onRetry} type="button">Try again</button>
    </section>
  )
}

function BookGrid({ books, onOpen }: { books: Book[]; onOpen: (bookID: number) => void }) {
  return (
    <section className="book-grid" aria-label="Library books">
      {books.map((book) => (
        <article className="library-book" key={book.id}>
          <button className="library-cover library-cover-button" onClick={() => onOpen(book.id)} type="button">
            <img alt={`Cover of ${book.title}`} loading="lazy" src={book.cover_url} />
            <span className="page-pill">{book.format === 'epub' ? `EPUB · ${book.page_count} sections` : book.format === 'pdf' ? `PDF · ${book.page_count} pages` : `${book.format.toUpperCase()} · ${book.page_count} pages`}</span>
          </button>
          <div className="library-book-copy">
            <h2 title={book.title}>{book.title}</h2>
            <p>{book.completed ? 'Finished' : book.started ? `Page ${book.current_page + 1} of ${book.page_count}` : 'Not started'}</p>
          </div>
          <button className="book-open" onClick={() => onOpen(book.id)} type="button">
            {book.completed ? 'Read again' : book.started ? 'Continue reading' : 'Start reading'}
            <ChevronRightIcon />
          </button>
        </article>
      ))}
    </section>
  )
}

function ImportActivity({ jobs }: { jobs: ImportJob[] }) {
  return (
    <section className="activity-card" aria-label="Import activity">
      <header><div><span className="eyebrow">Processing</span><h2>Recent uploads</h2></div></header>
      <div className="activity-list">
        {jobs.map((job) => <JobRow job={job} key={job.id} />)}
      </div>
    </section>
  )
}

function JobRow({ job }: { job: ImportJob }) {
  const running = job.status === 'pending' || job.status === 'running'
  const failed = job.status === 'failed'
  return (
    <div className={`job-row${failed ? ' job-row-failed' : ''}`}>
      <span className="job-icon">{failed ? <AlertIcon /> : running ? <ClockIcon /> : <CheckIcon />}</span>
      <span className="job-copy">
        <strong>{job.original_filename || 'Book'}</strong>
        <small>{failed ? job.error_message : running ? (job.status === 'pending' ? 'Queued for processing' : 'Validating and organizing the book') : 'Imported successfully'}</small>
      </span>
      <span className="job-progress-label">{running ? `${job.progress}%` : failed ? 'Failed' : 'Complete'}</span>
      {running ? <span className="job-progress"><i style={{ width: `${job.progress}%` }} /></span> : null}
    </div>
  )
}

function UploadDialog({ jobs, onClose, onQueued }: { jobs: ImportJob[]; onClose: () => void; onQueued: () => void }) {
  const queryClient = useQueryClient()
  const inputRef = useRef<HTMLInputElement>(null)
  const folderInputRef = useRef<HTMLInputElement>(null)
  const itemsRef = useRef<BatchUploadItem[]>([])
  const activeUploadsRef = useRef(new Map<string, AbortController>())
  const activePreparationsRef = useRef(new Set<string>())
  const [items, setItems] = useState<BatchUploadItem[]>([])
  const [dragging, setDragging] = useState(false)
  const [started, setStarted] = useState(false)
  const [batchSeries, setBatchSeries] = useState('')
  const [selectionMessage, setSelectionMessage] = useState('')
  const [uploadPump, setUploadPump] = useState(0)
  const [preparationPump, setPreparationPump] = useState(0)

  useEffect(() => {
    folderInputRef.current?.setAttribute('webkitdirectory', '')
    folderInputRef.current?.setAttribute('directory', '')
  }, [])

  useEffect(() => {
    itemsRef.current = items
  }, [items])

  useEffect(() => () => {
    activeUploadsRef.current.forEach((controller) => controller.abort())
  }, [])

  function updateItem(id: string, update: Partial<BatchUploadItem>) {
    setItems((current) => current.map((item) => item.id === id ? { ...item, ...update } : item))
  }

  function addFiles(fileList: FileList | File[]) {
    const files = Array.from(fileList)
    const created = createBatchUploadItems(files)
    const invalidCount = files.length - created.length
    const existing = new Set(itemsRef.current.map((item) => batchItemKey(item.file)))
    let duplicateCount = 0
    const unique = created.filter((item) => {
      const key = batchItemKey(item.file)
      if (existing.has(key)) {
        duplicateCount += 1
        return false
      }
      existing.add(key)
      return true
    })
    setItems((current) => {
      return [...current, ...unique].sort((left, right) => left.file.name.localeCompare(right.file.name, 'en-US', { numeric: true, sensitivity: 'base' }))
    })

    const inferredSeries = [...new Set(created.map((item) => item.series).filter(Boolean))]
    if (!batchSeries && inferredSeries.length === 1) setBatchSeries(inferredSeries[0] ?? '')
    const messages = []
    if (invalidCount) messages.push(`${invalidCount} file${invalidCount === 1 ? '' : 's'} ignored because the format is unsupported`)
    if (duplicateCount) messages.push(`${duplicateCount} duplicate${duplicateCount === 1 ? '' : 's'} in the selection`)
    setSelectionMessage(messages.join(' · '))
  }

  function drop(event: DragEvent<HTMLDivElement>) {
    event.preventDefault()
    setDragging(false)
    addFiles(event.dataTransfer.files)
  }

  function applySeriesToAll() {
    const series = batchSeries.trim()
    if (!series) return
    setItems((current) => current.map((item) => item.status === 'ready' ? { ...item, series } : item))
  }

  async function startUpload(item: BatchUploadItem) {
    if (activeUploadsRef.current.has(item.id)) return
    const controller = new AbortController()
    activeUploadsRef.current.set(item.id, controller)
    updateItem(item.id, { status: 'uploading', progress: 0, error: '' })
    try {
      const job = await uploadBook(
        item.file,
        { title: item.title, series: item.series, volume: item.volume },
        (progress) => updateItem(item.id, { progress }),
        controller.signal,
      )
      updateItem(item.id, { status: 'queued', progress: 100, jobId: job.id })
      onQueued()
    } catch (uploadError) {
      const duplicate = uploadError instanceof ApiError && uploadError.code === 'duplicate_book'
      updateItem(item.id, {
        status: duplicate ? 'duplicate' : 'failed',
        progress: 100,
        error: mutationMessage(uploadError),
      })
    } finally {
      activeUploadsRef.current.delete(item.id)
      setUploadPump((value) => value + 1)
    }
  }

  useEffect(() => {
    if (!started) return
    const slots = Math.max(0, 2 - activeUploadsRef.current.size)
    if (!slots) return
    items
      .filter((item) => item.status === 'ready' && !activeUploadsRef.current.has(item.id))
      .slice(0, slots)
      .forEach((item) => void startUpload(item))
  }, [items, started, uploadPump])

  useEffect(() => {
    if (!jobs.length) return
    const byID = new Map(jobs.map((job) => [job.id, job]))
    setItems((current) => current.map((item) => {
      if (!item.jobId) return item
      const job = byID.get(item.jobId)
      if (!job) return item
      if (job.status === 'failed') {
        if (item.status === 'failed' && item.error === job.error_message) return item
        return { ...item, status: 'failed', progress: 100, error: job.error_message || 'Could not import the file.' }
      }
      if (job.status === 'completed') {
        const bookId = job.book_id
        if (item.file.name.toLowerCase().endsWith('.pdf') && bookId && item.status !== 'preparing' && item.status !== 'completed') {
          return { ...item, status: 'preparing', progress: 100, bookId, preparationProgress: 1, preparationLabel: 'Waiting for quick PDF preparation' }
        }
        if (item.status === 'preparing' || item.status === 'completed') return item
        return { ...item, status: 'completed', progress: 100, bookId }
      }
      if (job.status === 'running') {
        if (item.status === 'processing' && item.progress === job.progress) return item
        return { ...item, status: 'processing', progress: job.progress }
      }
      if (job.status === 'pending' && item.status !== 'queued') return { ...item, status: 'queued', progress: 100 }
      return item
    }))
  }, [jobs])

  async function preparePDF(item: BatchUploadItem) {
    if (!item.bookId || activePreparationsRef.current.has(item.id)) return
    activePreparationsRef.current.add(item.id)
    try {
      const { analyzePDFFile } = await import('./features/pdf/analyzePdf')
      await analyzePDFFile(item.file, item.bookId, (progress) => updateItem(item.id, {
        preparationProgress: progress.percent,
        preparationLabel: progress.label,
      }))
      updateItem(item.id, {
        status: 'completed',
        preparationProgress: 100,
        preparationLabel: 'Cover, metadata, and classification prepared',
      })
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ['books'] }),
        queryClient.invalidateQueries({ queryKey: ['book', item.bookId] }),
        queryClient.invalidateQueries({ queryKey: ['dashboard'] }),
        queryClient.invalidateQueries({ queryKey: ['series'] }),
      ])
    } catch (analysisError) {
      updateItem(item.id, {
        status: 'completed',
        preparationProgress: 100,
        preparationLabel: 'PDF imported; preparation will resume when opened',
        preparationWarning: mutationMessage(analysisError),
      })
    } finally {
      activePreparationsRef.current.delete(item.id)
      setPreparationPump((value) => value + 1)
    }
  }

  useEffect(() => {
    if (activePreparationsRef.current.size) return
    const next = items.find((item) => item.status === 'preparing' && item.bookId && !activePreparationsRef.current.has(item.id))
    if (next) void preparePDF(next)
  }, [items, preparationPump])

  const readyCount = items.filter((item) => item.status === 'ready').length
  const uploadCount = items.filter((item) => item.status === 'uploading').length
  const processingCount = items.filter((item) => ['queued', 'processing', 'preparing'].includes(item.status)).length
  const completedCount = items.filter((item) => item.status === 'completed').length
  const failedCount = items.filter((item) => item.status === 'failed').length
  const duplicateCount = items.filter((item) => item.status === 'duplicate').length
  const hasActiveUpload = uploadCount > 0
  const editable = !started

  return (
    <div className="modal-backdrop" role="presentation" onMouseDown={(event) => event.target === event.currentTarget && !hasActiveUpload && onClose()}>
      <section aria-labelledby="upload-title" aria-modal="true" className="upload-dialog upload-dialog-batch" role="dialog">
        <header className="dialog-header">
          <div><span className="eyebrow">Batch import</span><h2 id="upload-title">Add volumes</h2><p>Select multiple files, review their series and volume metadata, and let samrai manage the queue.</p></div>
          <button aria-label="Close" className="icon-button" disabled={hasActiveUpload} onClick={onClose} type="button"><CloseIcon /></button>
        </header>

        <div
          aria-disabled={started}
          className={`drop-zone drop-zone-batch${dragging ? ' drop-zone-active' : ''}${items.length ? ' drop-zone-selected' : ''}${started ? ' drop-zone-disabled' : ''}`}
          onClick={() => { if (!started) inputRef.current?.click() }}
          onDragEnter={(event) => { event.preventDefault(); if (!started) setDragging(true) }}
          onDragLeave={() => setDragging(false)}
          onDragOver={(event) => event.preventDefault()}
          onDrop={(event) => { if (started) { event.preventDefault(); return }; drop(event) }}
          onKeyDown={(event) => {
            if (!started && (event.key === 'Enter' || event.key === ' ')) {
              event.preventDefault()
              inputRef.current?.click()
            }
          }}
          role="button"
          tabIndex={started ? -1 : 0}
        >
          <input accept=".cbz,.zip,.cbr,.rar,.cb7,.7z,.cbt,.tar,.pdf,.epub,application/zip,application/x-rar-compressed,application/x-7z-compressed,application/x-tar,application/pdf,application/epub+zip" disabled={started} hidden multiple onChange={(event) => { if (event.target.files) addFiles(event.target.files); event.target.value = '' }} ref={inputRef} type="file" />
          <input accept=".cbz,.zip,.cbr,.rar,.cb7,.7z,.cbt,.tar,.pdf,.epub" disabled={started} hidden multiple onChange={(event) => { if (event.target.files) addFiles(event.target.files); event.target.value = '' }} ref={folderInputRef} type="file" />
          <span className="drop-icon"><UploadIcon /></span>
          <strong>{items.length ? `${items.length} file${items.length === 1 ? '' : 's'} under review` : 'Drop all volumes here'}</strong>
          <span>{items.length ? 'click to add more files' : 'or choose files and folders from your browser'}</span>
          <div className="upload-source-actions">
            <button className="button button-secondary button-small" disabled={!editable} onClick={(event) => { event.stopPropagation(); inputRef.current?.click() }} type="button">Select files</button>
            <button className="button button-secondary button-small" disabled={!editable} onClick={(event) => { event.stopPropagation(); folderInputRef.current?.click() }} type="button">Select folder</button>
          </div>
        </div>

        <div className="upload-rules">
          <span><CheckIcon />Up to two simultaneous uploads</span>
          <span><CheckIcon />Natural numeric order</span>
          <span><CheckIcon />Duplicates checked by SHA-256</span>
        </div>

        {selectionMessage ? <p className="upload-selection-message">{selectionMessage}</p> : null}

        {items.length ? (
          <>
            <div className="batch-summary">
              <span><strong>{readyCount}</strong> ready</span>
              <span><strong>{uploadCount + processingCount}</strong> in progress</span>
              <span><strong>{completedCount}</strong> completed</span>
              <span><strong>{failedCount + duplicateCount}</strong> need attention</span>
            </div>

            {editable ? (
              <div className="batch-series-control">
                <label><span>Series for the entire batch</span><input onChange={(event) => setBatchSeries(event.target.value)} placeholder="e.g. Berserk" value={batchSeries} /></label>
                <button className="button button-secondary button-small" disabled={!batchSeries.trim()} onClick={applySeriesToAll} type="button">Apply to volumes</button>
              </div>
            ) : null}

            <div className="batch-file-list" aria-label="Selected files">
              {items.map((item, index) => {
                const canEditItem = item.status === 'ready'
                const canRetry = item.status === 'failed'
                const status = batchUploadStatus(item)
                const progress = item.status === 'preparing' ? item.preparationProgress ?? 0 : item.progress
                return (
                  <article className={`batch-file-row batch-file-${item.status}`} key={item.id}>
                    <header>
                      <span className="batch-file-index">{String(index + 1).padStart(2, '0')}</span>
                      <span className="batch-file-name"><strong title={item.file.name}>{item.file.name}</strong><small>{formatBytes(item.file.size)}</small></span>
                      <span className="batch-file-status">{status}</span>
                      {canRetry ? <button aria-label={`Retry ${item.file.name}`} className="batch-file-action" onClick={() => updateItem(item.id, { status: 'ready', progress: 0, error: '', jobId: undefined, bookId: undefined, preparationProgress: undefined, preparationLabel: undefined, preparationWarning: undefined })} type="button"><RefreshIcon /></button> : null}
                      {canEditItem || item.status === 'failed' || item.status === 'duplicate' ? <button aria-label={`Remove ${item.file.name}`} className="batch-file-action" onClick={() => setItems((current) => current.filter((candidate) => candidate.id !== item.id))} type="button"><TrashIcon /></button> : null}
                    </header>
                    <div className="batch-metadata-grid">
                      <label><span>Title</span><input disabled={!canEditItem} maxLength={240} onChange={(event) => updateItem(item.id, { title: event.target.value })} value={item.title} /></label>
                      <label><span>Series</span><input disabled={!canEditItem} maxLength={240} onChange={(event) => updateItem(item.id, { series: event.target.value })} placeholder="No series" value={item.series} /></label>
                      <label><span>Volume</span><input disabled={!canEditItem} maxLength={100} onChange={(event) => updateItem(item.id, { volume: event.target.value })} placeholder="—" value={item.volume} /></label>
                    </div>
                    {item.status !== 'ready' ? <div className="batch-file-progress"><i style={{ width: `${Math.max(0, Math.min(100, progress))}%` }} /></div> : null}
                    {item.error ? <p className="batch-file-error">{item.error}</p> : null}
                    {item.preparationWarning ? <p className="batch-file-warning">{item.preparationWarning}</p> : null}
                    {item.status === 'preparing' && item.preparationLabel ? <p className="batch-file-caption">{item.preparationLabel}</p> : null}
                  </article>
                )
              })}
            </div>
          </>
        ) : null}

        <footer className="dialog-actions batch-dialog-actions">
          <button className="button button-secondary" disabled={hasActiveUpload} onClick={onClose} type="button">{started ? 'Close' : 'Cancel'}</button>
          {failedCount ? <button className="button button-secondary" onClick={() => setItems((current) => current.map((item) => item.status === 'failed' ? { ...item, status: 'ready', progress: 0, error: '', jobId: undefined, bookId: undefined, preparationProgress: undefined, preparationLabel: undefined, preparationWarning: undefined } : item))} type="button"><RefreshIcon />Retry failed</button> : null}
          <button className="button button-primary" disabled={!items.length || started || !readyCount} onClick={() => setStarted(true)} type="button">
            {hasActiveUpload ? <span className="spinner spinner-small" /> : <UploadIcon />}
            {started ? (uploadCount || processingCount ? 'Importing batch…' : 'Batch submitted') : `Import ${readyCount} volume${readyCount === 1 ? '' : 's'}`}
          </button>
        </footer>
      </section>
    </div>
  )
}

function batchUploadStatus(item: BatchUploadItem) {
  switch (item.status) {
    case 'ready': return 'ready'
    case 'uploading': return `uploading ${item.progress}%`
    case 'queued': return 'queued'
    case 'processing': return `processing ${item.progress}%`
    case 'preparing': return `preparing PDF ${item.preparationProgress ?? 0}%`
    case 'completed': return item.preparationWarning ? 'imported with warning' : 'complete'
    case 'duplicate': return 'already exists'
    case 'failed': return 'failed'
    default: return item.status
  }
}

function PerformanceDialog({ onClose }: { onClose: () => void }) {
  const metrics = useQuery({ queryKey: ['system-metrics'], queryFn: api.metrics, refetchInterval: 2000 })
  const data = metrics.data
  const hitRate = data ? `${(data.images.hit_rate * 100).toFixed(1)}%` : '—'
  return (
    <div className="modal-backdrop" role="presentation" onMouseDown={(event) => event.target === event.currentTarget && onClose()}>
      <section className="performance-dialog" role="dialog" aria-modal="true" aria-labelledby="performance-title">
        <header className="dialog-header"><div><span className="eyebrow">Server</span><h2 id="performance-title">Performance</h2><p>Local metrics refresh every two seconds.</p></div><button className="icon-button" aria-label="Close" onClick={onClose} type="button"><CloseIcon /></button></header>
        {metrics.isPending ? <div className="performance-loading"><span className="spinner" />Loading metrics…</div> : null}
        {metrics.isError ? <FormError message="Could not load metrics." /> : null}
        {data ? (
          <>
            <div className="performance-grid">
              <article><span>Current heap</span><strong>{formatBytes(data.runtime.heap_bytes)}</strong><small>{data.runtime.goroutines} goroutines</small></article>
              <article><span>Image cache</span><strong>{formatBytes(data.images.cache_bytes)}</strong><small>limit {formatBytes(data.images.cache_limit_bytes)}</small></article>
              <article><span>Hit rate</span><strong>{hitRate}</strong><small>{data.images.hits} hits · {data.images.misses} misses</small></article>
              <article><span>Conversions</span><strong>{data.images.generated}</strong><small>average {data.images.average_generation_milliseconds.toFixed(0)} ms</small></article>
            </div>
            <div className="performance-worker">
              <span><strong>Image processing</strong><small>{data.images.active_workers} active of {data.images.worker_limit} · WebP quality {data.images.quality}</small></span>
              <div><i style={{ width: `${Math.min(100, (data.images.active_workers / Math.max(1, data.images.worker_limit)) * 100)}%` }} /></div>
            </div>
          </>
        ) : null}
      </section>
    </div>
  )
}

function formatBytes(bytes: number) {
  if (bytes < 1024 * 1024) return `${Math.max(1, Math.round(bytes / 1024))} KB`
  if (bytes < 1024 * 1024 * 1024) return `${(bytes / 1024 / 1024).toFixed(1)} MB`
  return `${(bytes / 1024 / 1024 / 1024).toFixed(2)} GB`
}

function BrandMark({ size }: { size?: 'large' }) {
  return (
    <span className={`brand-mark${size === 'large' ? ' brand-mark-large' : ''}`} aria-hidden="true">
      <span className="brand-mark-prefix">[</span>
      <span className="brand-mark-letter">s</span>
      <span className="brand-mark-prefix">]</span>
      <span className="brand-cursor" />
    </span>
  )
}

type LibraryRoute =
  | { view: 'home' }
  | { view: 'books' }
  | { view: 'series' }
  | { view: 'series-detail'; seriesID: number }
  | { view: 'favorites' }
  | { view: 'history' }
  | { view: 'annotations' }
  | { view: 'settings' }

function libraryRouteFromPath(pathname = window.location.pathname): LibraryRoute {
  if (/^\/books\/?$/.test(pathname)) return { view: 'books' }
  if (/^\/series\/?$/.test(pathname)) return { view: 'series' }
  if (/^\/favorites\/?$/.test(pathname)) return { view: 'favorites' }
  if (/^\/history\/?$/.test(pathname)) return { view: 'history' }
  if (/^\/annotations\/?$/.test(pathname)) return { view: 'annotations' }
  if (/^\/settings\/?$/.test(pathname)) return { view: 'settings' }
  const seriesMatch = pathname.match(/^\/series\/(\d+)\/?$/)
  if (seriesMatch) {
    const seriesID = Number(seriesMatch[1])
    if (Number.isSafeInteger(seriesID) && seriesID > 0) return { view: 'series-detail', seriesID }
  }
  return { view: 'home' }
}

function accessibleLibraryRoute(user: User, pathname: string): LibraryRoute {
  const route = libraryRouteFromPath(pathname)
  return route.view === 'settings' && user.role !== 'admin' ? { view: 'home' } : route
}

function libraryPath(route: LibraryRoute) {
  switch (route.view) {
    case 'books': return '/books'
    case 'series': return '/series'
    case 'series-detail': return `/series/${route.seriesID}`
    case 'favorites': return '/favorites'
    case 'history': return '/history'
    case 'annotations': return '/annotations'
    case 'settings': return '/settings'
    default: return '/'
  }
}

function detailBookIDFromPath() {
  const match = window.location.pathname.match(/^\/book\/(\d+)\/?$/)
  if (!match) return null
  const value = Number(match[1])
  return Number.isSafeInteger(value) && value > 0 ? value : null
}

function readerBookIDFromPath() {
  const match = window.location.pathname.match(/^\/read\/(\d+)\/?$/)
  if (!match) return null
  const value = Number(match[1])
  return Number.isSafeInteger(value) && value > 0 ? value : null
}

function mutationMessage(error: unknown) {
  if (error instanceof ApiError) {
    return error.message
  }
  if (error instanceof Error) {
    return error.message
  }
  return ''
}
