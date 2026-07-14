import { useEffect, useState, type FormEvent } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api, type Book, type UpdateBookInput } from '../../lib/api'
import { AlertIcon, BackIcon, CheckIcon, CloseIcon, HeartIcon, SettingsIcon } from '../../components/Icons'

export function BookDetails({
  bookId,
  admin,
  onBack,
  onRead,
  onDeleted,
}: {
  bookId: number
  admin: boolean
  onBack: () => void
  onRead: () => void
  onDeleted: () => void
}) {
  const queryClient = useQueryClient()
  const bookQuery = useQuery({ queryKey: ['book', bookId], queryFn: () => api.book(bookId) })
  const [editing, setEditing] = useState(false)
  const [form, setForm] = useState<UpdateBookInput>({})

  useEffect(() => {
    if (!bookQuery.data) return
    setForm({
      title: bookQuery.data.title,
      summary: bookQuery.data.summary,
      writer: bookQuery.data.writer,
      publisher: bookQuery.data.publisher,
      publication_year: bookQuery.data.publication_year,
      volume: bookQuery.data.volume,
      number: bookQuery.data.number,
      language: bookQuery.data.language,
      reading_direction: bookQuery.data.reading_direction ?? '',
      series: bookQuery.data.series,
    })
  }, [bookQuery.data])

  const update = useMutation({
    mutationFn: () => api.updateBook(bookId, {
      ...form,
      clear_publication_year: bookQuery.data?.publication_year !== undefined && form.publication_year === undefined,
    }),
    onSuccess: (book) => {
      queryClient.setQueryData(['book', bookId], book)
      void queryClient.invalidateQueries({ queryKey: ['books'] })
      void queryClient.invalidateQueries({ queryKey: ['dashboard'] })
      void queryClient.invalidateQueries({ queryKey: ['series'] })
      setEditing(false)
    },
  })
  const reset = useMutation({
    mutationFn: () => api.resetProgress(bookId),
    onSuccess: () => {
      const patch = (book: Book) => ({ ...book, current_page: 0, started: false, completed: false })
      queryClient.setQueryData<Book>(['book', bookId], (current) => current ? patch(current) : current)
      void queryClient.invalidateQueries({ queryKey: ['books'] })
      void queryClient.invalidateQueries({ queryKey: ['dashboard'] })
      void queryClient.invalidateQueries({ queryKey: ['series'] })
    },
  })
  const favorite = useMutation({
    mutationFn: () => api.setBookFavorite(bookId, !bookQuery.data?.favorite),
    onSuccess: ({ favorite }) => {
      queryClient.setQueryData<Book>(['book', bookId], (current) => current ? { ...current, favorite } : current)
      void queryClient.invalidateQueries({ queryKey: ['books'] })
      void queryClient.invalidateQueries({ queryKey: ['dashboard'] })
      void queryClient.invalidateQueries({ queryKey: ['favorites'] })
    },
  })
  const updateOCRMode = useMutation({
    mutationFn: (mode: 'auto' | 'off' | 'on_demand' | 'background' | 'full') => api.updatePDFOCRMode(bookId, mode),
    onSuccess: (_, mode) => {
      queryClient.setQueryData<Book>(['book', bookId], (current) => current ? { ...current, pdf_ocr_mode: mode } : current)
      void queryClient.invalidateQueries({ queryKey: ['book', bookId] })
      void queryClient.invalidateQueries({ queryKey: ['books'] })
    },
  })
  const remove = useMutation({
    mutationFn: () => api.deleteBook(bookId),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['books'] })
      void queryClient.invalidateQueries({ queryKey: ['dashboard'] })
      void queryClient.invalidateQueries({ queryKey: ['series'] })
      onDeleted()
    },
  })

  if (bookQuery.isPending) {
    return <main className="book-details-status"><span className="spinner" /><strong>Opening details…</strong></main>
  }
  if (bookQuery.isError || !bookQuery.data) {
    return (
      <main className="book-details-status">
        <AlertIcon />
        <strong>Could not open the book.</strong>
        <button className="button button-secondary" onClick={onBack} type="button">Back</button>
      </main>
    )
  }

  const book = bookQuery.data
  const progress = book.page_count > 0 ? Math.round(((book.current_page + (book.started ? 1 : 0)) / book.page_count) * 100) : 0

  function submit(event: FormEvent) {
    event.preventDefault()
    update.mutate()
  }

  async function startReading() {
    if (book.completed) {
      try {
        await reset.mutateAsync()
      } catch {
        return
      }
    }
    onRead()
  }

  return (
    <main className="book-details-page">
      <header className="book-details-topbar">
        <button className="button button-secondary" onClick={onBack} type="button"><BackIcon />Library</button>
        {admin ? <button className="button button-secondary" onClick={() => setEditing(true)} type="button"><SettingsIcon />Edit metadata</button> : null}
      </header>

      <section className="book-hero">
        <div className="book-hero-cover"><img alt={`Cover of ${book.title}`} src={`/api/v1/books/${book.id}/cover?width=960&format=webp&v=${encodeURIComponent(book.updated_at)}`} /></div>
        <div className="book-hero-copy">
          {book.series ? <span className="eyebrow">{book.series}</span> : <span className="eyebrow">Standalone book</span>}
          <h1>{book.title}</h1>
          <p className="book-byline">{[book.writer, book.publisher, book.publication_year].filter(Boolean).join(' · ') || 'No credits provided'}</p>
          {book.summary ? <p className="book-summary">{book.summary}</p> : <p className="book-summary book-summary-muted">No synopsis was found in the file metadata.</p>}

          <div className="book-progress-card">
            <span><strong>{book.completed ? 'Complete' : book.started ? readingPositionLabel(book) : 'Not started'}</strong><small>{progress}% read</small></span>
            <div><i style={{ width: `${progress}%` }} /></div>
          </div>

          <div className="book-hero-actions">
            <button className="button button-primary" disabled={reset.isPending} onClick={() => void startReading()} type="button">{book.started && !book.completed ? 'Continue reading' : book.completed ? 'Read again' : 'Start reading'}</button>
            <button aria-pressed={book.favorite} className={`button button-secondary${book.favorite ? ' favorite-action-active' : ''}`} disabled={favorite.isPending} onClick={() => favorite.mutate()} type="button"><HeartIcon />{book.favorite ? 'Favorite' : 'Add to favorites'}</button>
            {book.started ? <button className="button button-secondary" disabled={reset.isPending} onClick={() => window.confirm('Reset reading progress for this book?') && reset.mutate()} type="button">Reset progress</button> : null}
          </div>
        </div>
      </section>

      <section className="book-information">
        <h2>Information</h2>
        <dl>
          <div><dt>{book.format === 'epub' && book.epub_layout !== 'fixed' ? 'Sections' : 'Pages'}</dt><dd>{book.page_count}</dd></div>
          <div><dt>File</dt><dd>{formatBytes(book.file_size)}</dd></div>
          <div><dt>Volume</dt><dd>{book.volume || '—'}</dd></div>
          <div><dt>Number</dt><dd>{book.number || '—'}</dd></div>
          <div><dt>Language</dt><dd>{book.language || '—'}</dd></div>
          {book.format === 'pdf' ? <div><dt>Text layer</dt><dd>{pdfTextLayerLabel(book)}</dd></div> : null}
          {book.format === 'epub' ? <><div><dt>Format</dt><dd>EPUB {book.epub_version || ''}</dd></div><div><dt>Layout</dt><dd>{book.epub_layout === 'fixed' ? 'Fixed layout (manga/comic)' : 'Reflowable text'}</dd></div></> : null}
          {['cbz', 'cbr', 'cb7', 'cbt'].includes(book.format) ? <div><dt>Format</dt><dd>{book.format.toUpperCase()}</dd></div> : null}
          <div><dt>Direction</dt><dd>{book.reading_direction === 'rtl' ? 'Manga (right to left)' : 'Western (left to right)'}</dd></div>
          <div className="book-file-row"><dt>Original name</dt><dd>{book.original_filename}</dd></div>
        </dl>
      </section>

      {book.format === 'pdf' ? (
        <section className="pdf-ocr-settings-card">
          <header>
            <div><span className="eyebrow">Text recognition</span><h2>Smart OCR</h2></div>
            <span className={`pdf-kind-badge pdf-kind-${book.pdf_document_kind}`}>{pdfDocumentKindLabel(book)}</span>
          </header>
          <div className="pdf-ocr-settings-grid">
            <div>
              <strong>Current mode</strong>
              <p>{pdfOCRModeDescription(book)}</p>
            </div>
            <div>
              <strong>Search index</strong>
              <p>{book.pdf_indexed_pages} of {book.page_count} pages · {book.pdf_ocr_pages} via OCR</p>
            </div>
          </div>
          {admin ? (
            <label className="pdf-ocr-mode-field">
              <span>OCR behavior</span>
              <select
                disabled={updateOCRMode.isPending}
                value={book.pdf_ocr_mode}
                onChange={(event) => updateOCRMode.mutate(event.target.value as 'auto' | 'off' | 'on_demand' | 'background' | 'full')}
              >
                <option value="auto">Automatic based on classification</option>
                <option value="off">Off</option>
                <option value="on_demand">On demand only</option>
                <option value="background">In the background while the reader is open</option>
                <option value="full">Full scan when the reader opens</option>
              </select>
              <small>Manga and comics default to “Off.” On-demand mode lets you recognize only the current page.</small>
            </label>
          ) : null}
          {updateOCRMode.isError ? <p className="form-error">Could not change OCR mode.</p> : null}
        </section>
      ) : null}

      {admin ? (
        <section className="danger-zone">
          <div><strong>Remove from library</strong><p>Removes the database record, original file, and all derived cached data.</p></div>
          <button className="button button-danger" disabled={remove.isPending} onClick={() => window.confirm(`Delete “${book.title}” permanently?`) && remove.mutate()} type="button">Delete book</button>
        </section>
      ) : null}

      {editing ? (
        <div className="modal-backdrop" role="presentation" onMouseDown={(event) => event.target === event.currentTarget && setEditing(false)}>
          <form className="metadata-dialog" onSubmit={submit}>
            <header className="dialog-header"><div><span className="eyebrow">Library</span><h2>Edit metadata</h2></div><button aria-label="Close" className="icon-button" onClick={() => setEditing(false)} type="button"><CloseIcon /></button></header>
            <div className="metadata-fields">
              <label><span>Title</span><input required value={form.title ?? ''} onChange={(event) => setForm({ ...form, title: event.target.value })} /></label>
              <label><span>Series</span><input placeholder="Leave blank for a standalone book" value={form.series ?? ''} onChange={(event) => setForm({ ...form, series: event.target.value })} /></label>
              <label><span>Author</span><input value={form.writer ?? ''} onChange={(event) => setForm({ ...form, writer: event.target.value })} /></label>
              <label><span>Publisher</span><input value={form.publisher ?? ''} onChange={(event) => setForm({ ...form, publisher: event.target.value })} /></label>
              <label><span>Year</span><input min="1" max="9999" type="number" value={form.publication_year ?? ''} onChange={(event) => setForm({ ...form, publication_year: event.target.value ? Number(event.target.value) : undefined })} /></label>
              <label><span>Volume</span><input value={form.volume ?? ''} onChange={(event) => setForm({ ...form, volume: event.target.value })} /></label>
              <label><span>Number</span><input value={form.number ?? ''} onChange={(event) => setForm({ ...form, number: event.target.value })} /></label>
              <label><span>Language</span><input value={form.language ?? ''} onChange={(event) => setForm({ ...form, language: event.target.value })} /></label>
              <label><span>Direction</span><select value={form.reading_direction ?? ''} onChange={(event) => setForm({ ...form, reading_direction: event.target.value as 'ltr' | 'rtl' | '' })}><option value="">User default</option><option value="ltr">Western</option><option value="rtl">Manga</option></select></label>
              <label className="metadata-summary"><span>Summary</span><textarea rows={6} value={form.summary ?? ''} onChange={(event) => setForm({ ...form, summary: event.target.value })} /></label>
            </div>
            {update.isError ? <p className="form-error">Could not save metadata.</p> : null}
            <footer className="dialog-actions"><button className="button button-secondary" onClick={() => setEditing(false)} type="button">Cancel</button><button className="button button-primary" disabled={update.isPending} type="submit"><CheckIcon />Save changes</button></footer>
          </form>
        </div>
      ) : null}
    </main>
  )
}

function formatBytes(bytes: number) {
  if (bytes < 1024 * 1024) return `${Math.max(1, Math.round(bytes / 1024))} KB`
  if (bytes < 1024 * 1024 * 1024) return `${(bytes / 1024 / 1024).toFixed(1)} MB`
  return `${(bytes / 1024 / 1024 / 1024).toFixed(2)} GB`
}

function readingPositionLabel(book: Book) {
  if (book.format === 'epub' && book.epub_layout !== 'fixed') return `Section ${book.current_page + 1} of ${book.page_count}`
  return `Page ${book.current_page + 1} of ${book.page_count}`
}

function pdfTextLayerLabel(book: Book) {
  if (book.pdf_classification_status === 'pending') return 'Classification pending'
  if (book.pdf_analysis_status === 'failed') return 'Analysis failed'
  if (book.pdf_document_kind === 'comic' && book.pdf_indexed_pages === 0) return 'Visual PDF · OCR off'
  if (book.pdf_text_layer === 'text') return 'Selectable text'
  if (book.pdf_text_layer === 'ocr') return `Text recovered with OCR (${book.pdf_ocr_pages} pages)`
  if (book.pdf_text_layer === 'hybrid') return `Native text and OCR (${book.pdf_ocr_pages} pages via OCR)`
  if (book.pdf_text_layer === 'corrupt') return book.pdf_ocr_mode === 'off' ? 'Invalid text layer · OCR off' : 'Invalid text layer · on-demand OCR'
  if (book.pdf_text_layer === 'mixed') return `Mixed · ${book.pdf_indexed_pages} indexed page(s)`
  if (book.pdf_text_layer === 'scanned') return book.pdf_ocr_mode === 'off' ? 'Visual document · OCR off' : 'Scanned document · on-demand OCR'
  return 'Unknown'
}


function pdfDocumentKindLabel(book: Book) {
  if (book.pdf_document_kind === 'text') return 'Text PDF'
  if (book.pdf_document_kind === 'scanned_book') return 'Scanned book'
  if (book.pdf_document_kind === 'comic') return 'Manga / comic'
  if (book.pdf_document_kind === 'mixed') return 'Mixed document'
  return book.pdf_classification_status === 'complete' ? 'Unknown' : 'Classification pending'
}

function pdfOCRModeDescription(book: Book) {
  if (book.pdf_ocr_mode === 'off') return 'No automatic OCR will run. Reading remains available without delay.'
  if (book.pdf_ocr_mode === 'on_demand') return 'Only pages selected in the reader will be recognized.'
  if (book.pdf_ocr_mode === 'background') return 'OCR continues while this book is open in the browser.'
  if (book.pdf_ocr_mode === 'full') return 'All unindexed pages will be processed when the reader opens.'
  return 'samrai chooses a conservative mode based on the detected document type.'
}
