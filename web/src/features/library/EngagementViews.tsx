import { useMemo, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { allAnnotationsExportURL, api, type AnnotationColor, type AnnotationSummary, type ReadingSession } from '../../lib/api'
import { AlertIcon, ClockIcon, DownloadIcon, HeartIcon, NotesIcon, TrashIcon } from '../../components/Icons'
import { BookGrid, SeriesGrid } from './LibraryViews'

export function FavoritesView({ onOpenBook, onOpenSeries }: { onOpenBook: (id: number) => void; onOpenSeries: (id: number) => void }) {
  const query = useQuery({ queryKey: ['favorites'], queryFn: api.favorites })
  if (query.isPending) return <ViewLoading />
  if (query.isError || !query.data) return <ViewError onRetry={() => void query.refetch()} />
  return (
    <>
      <header className="page-heading"><div><span className="eyebrow">Your picks</span><h1>Favorites</h1><p>Books and series you saved for quick access.</p></div></header>
      {query.data.books.length ? <section className="library-section"><header className="section-heading"><div><h2>Favorite books</h2><p>{query.data.books.length} {query.data.books.length === 1 ? 'item' : 'items'}</p></div></header><BookGrid books={query.data.books} onOpen={onOpenBook} /></section> : null}
      {query.data.series.length ? <section className="library-section"><header className="section-heading"><div><h2>Favorite series</h2><p>{query.data.series.length} {query.data.series.length === 1 ? 'collection' : 'collections'}</p></div></header><SeriesGrid series={query.data.series} onOpen={onOpenSeries} /></section> : null}
      {!query.data.books.length && !query.data.series.length ? <EmptyState icon={<HeartIcon />} title="No favorites yet" description="Use the heart button on books and series to keep them here." /> : null}
    </>
  )
}

export function HistoryView({ onOpenBook }: { onOpenBook: (id: number) => void }) {
  const queryClient = useQueryClient()
  const query = useQuery({ queryKey: ['reading-history'], queryFn: api.history })
  const clear = useMutation({
    mutationFn: api.clearHistory,
    onSuccess: () => queryClient.setQueryData(['reading-history'], { items: [] }),
  })
  if (query.isPending) return <ViewLoading />
  if (query.isError || !query.data) return <ViewError onRetry={() => void query.refetch()} />
  return (
    <>
      <header className="page-heading page-heading-actions"><div><span className="eyebrow">Personal activity</span><h1>Reading history</h1><p>Reading sessions are grouped without recording every individual page turn.</p></div>{query.data.items.length ? <button className="button button-secondary" disabled={clear.isPending} onClick={() => window.confirm('Clear your entire reading history?') && clear.mutate()} type="button"><TrashIcon />Clear history</button> : null}</header>
      {query.data.items.length ? <section className="history-list">{query.data.items.map((item) => <HistoryCard item={item} key={item.id} onOpen={() => onOpenBook(item.book_id)} />)}</section> : <EmptyState icon={<ClockIcon />} title="No reading sessions yet" description="Future reading sessions will appear here automatically." />}
    </>
  )
}

function HistoryCard({ item, onOpen }: { item: ReadingSession; onOpen: () => void }) {
  return <button className="history-card" onClick={onOpen} type="button"><img alt="" src={item.cover_url} /><span className="history-card-copy"><small>{formatDate(item.last_activity_at)} · {formatDuration(item.duration_seconds)}</small><strong>{item.book_title}</strong><span>{positionLabel(item)}</span></span><ClockIcon /></button>
}

export function AnnotationsView({ onOpenBook }: { onOpenBook: (id: number) => void }) {
  const [search, setSearch] = useState('')
  const [color, setColor] = useState('')
  const [source, setSource] = useState('')
  const query = useQuery({ queryKey: ['all-annotations', search, color, source], queryFn: () => api.allAnnotations({ search, color, source, limit: 300 }) })
  const grouped = useMemo(() => {
    const result = new Map<number, AnnotationSummary[]>()
    for (const item of query.data?.items ?? []) result.set(item.book_id, [...(result.get(item.book_id) ?? []), item])
    return [...result.values()]
  }, [query.data])
  return (
    <>
      <header className="page-heading page-heading-actions"><div><span className="eyebrow">Notebook</span><h1>Annotations</h1><p>Search highlights and notes from every PDF and EPUB in one place.</p></div><a className="button button-secondary" href={allAnnotationsExportURL()}><DownloadIcon />Export Markdown</a></header>
      <section className="annotation-filters"><input aria-label="Search annotations" onChange={(event) => setSearch(event.target.value)} placeholder="Search quotes, notes, or books…" value={search} /><select aria-label="Filter by format" onChange={(event) => setSource(event.target.value)} value={source}><option value="">PDF and EPUB</option><option value="pdf">PDF</option><option value="epub">EPUB</option></select><select aria-label="Filter by color" onChange={(event) => setColor(event.target.value)} value={color}><option value="">All colors</option>{(['yellow','green','blue','pink','orange'] as AnnotationColor[]).map((value) => <option key={value} value={value}>{colorLabel(value)}</option>)}</select></section>
      {query.isPending ? <ViewLoading /> : null}
      {query.isError ? <ViewError onRetry={() => void query.refetch()} /> : null}
      {query.data && !query.data.items.length ? <EmptyState icon={<NotesIcon />} title="No annotations found" description="Highlights and notes created while reading will appear here." /> : null}
      {grouped.map((items) => {
        const first = items[0]
        if (!first) return null
        return <section className="annotation-book-group" key={first.book_id}><button className="annotation-book-heading" onClick={() => onOpenBook(first.book_id)} type="button"><img alt="" src={first.cover_url} /><span><small>{first.book_format.toUpperCase()} · {items.length} annotation(s)</small><strong>{first.book_title}</strong></span></button><div className="annotation-summary-list">{items.map((item) => <button className="annotation-summary-card" key={`${item.source}-${item.id}`} onClick={() => onOpenBook(item.book_id)} type="button"><i className={`annotation-dot annotation-${item.color}`} /><span><small>{item.location}</small><strong>{item.selected_text || 'Mark without text'}</strong>{item.note ? <em>{item.note}</em> : null}</span></button>)}</div></section>
      })}
    </>
  )
}

function EmptyState({ icon, title, description }: { icon: React.ReactNode; title: string; description: string }) { return <section className="no-results">{icon}<h2>{title}</h2><p>{description}</p></section> }
function ViewLoading() { return <section className="library-loading" aria-label="Loading"><span className="book-skeleton"/><span className="book-skeleton"/><span className="book-skeleton"/></section> }
function ViewError({ onRetry }: { onRetry: () => void }) { return <section className="inline-error"><AlertIcon/><div><strong>Could not load this section</strong><p>Try again.</p></div><button className="button button-secondary" onClick={onRetry}>Try again</button></section> }
function formatDate(value: string) { return new Intl.DateTimeFormat('en-US', { dateStyle: 'medium', timeStyle: 'short' }).format(new Date(value)) }
function formatDuration(seconds: number) { if (seconds < 60) return 'less than 1 min'; const minutes = Math.round(seconds / 60); return minutes < 60 ? `${minutes} min` : `${Math.floor(minutes / 60)}h ${minutes % 60}min` }
function positionLabel(item: ReadingSession) { const label = item.book_format === 'epub' ? 'Section' : 'Page'; return `${label} ${item.end_page + 1}` }
function colorLabel(value: AnnotationColor) { return ({ yellow: 'Yellow', green: 'Green', blue: 'Blue', pink: 'Pink', orange: 'Orange' })[value] }
