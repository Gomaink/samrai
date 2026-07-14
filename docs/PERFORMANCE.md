# Image cache and performance

## Page flow

The original file remains the source of truth. When the reader requests a page, samrai opens the source or prepared page, applies the requested size, encodes WebP, and writes the result to the disk cache. Later requests reuse the cached image.

The cache key includes the source identity and output parameters. It is safe to clear the cache from the maintenance screen; pages are rebuilt while users read.

## Concurrency

`SAMRAI_IMAGE_WORKERS` limits simultaneous conversions. A small value is usually better on home servers because image conversion competes with imports and OCR for CPU and memory.

Browser batch import sends no more than two files at a time. Server-side jobs continue through the persistent queue. Quick PDF preparation is serialized to avoid a large batch starting several expensive PDF tasks at once.

## Cache limits

`SAMRAI_IMAGE_CACHE_MIB` sets the approximate disk target. Cleanup runs at `SAMRAI_CACHE_CLEANUP_INTERVAL` and removes older entries when the cache exceeds the limit.

`SAMRAI_IMAGE_MAX_WIDTH` and `SAMRAI_IMAGE_QUALITY` control generated WebP files. Raising either value increases storage use and conversion cost.

## Measuring a slow reader

Check, in order:

1. whether the source file is on local storage;
2. whether the first request is generating a new page;
3. image worker saturation;
4. OCR activity;
5. archive-tool or prepared-page errors;
6. available disk space and cache size;
7. proxy buffering and network latency.

The second request for the same size should normally be faster than the first.
