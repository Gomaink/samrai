# EPUB support

## Import

samrai reads the EPUB container, package document, metadata, manifest, spine, navigation documents, cover, and layout properties. Resource paths are normalized and validated before they are served.

DRM-protected and encrypted content is rejected. The original EPUB remains in the managed library.

## Fixed-layout books

Fixed-layout EPUBs use the paged visual reader. This is suitable for comics, illustrated books, and editions whose page geometry is part of the design.

## Reflowable books

Reflowable EPUBs use an isolated text reader with controls for theme, font, content width, alignment, line spacing, margins, and reading direction. The browser paginates the current spine item to the available viewport.

Progress is stored as an EPUB locator so it can survive changes in viewport size better than a synthetic page number.

## Navigation and search

The table of contents is built from EPUB 3 navigation documents or EPUB 2 NCX data. Search runs across indexed spine text and returns locations that the reader can open.

## Highlights and notes

Text selections create user-private highlights or notes. An EPUB annotation stores selected text and a structured locator into the spine resource. The reader tries to restore the selection when the chapter is opened again.

Annotations can be edited, deleted, searched in the combined notebook, and exported as Markdown. Image-only EPUBs do not yet support rectangular area marks.

## Content isolation

EPUB HTML is served through authenticated book resource routes and rendered inside a sandboxed iframe. Active content and unsafe URLs are filtered. An EPUB should still be treated as untrusted input, particularly when it comes from an unknown source.
