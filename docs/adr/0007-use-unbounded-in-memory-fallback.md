# Use unbounded in-memory fallback

After a successful fetch, a running Machine Client may continue using its in-memory Last-known-good for as long as Configra remains unreachable; the cache is never persisted, so a cold-starting process still fails without Configra. This prioritizes application availability over freshness, on the basis that a rotation cannot be consumed until connectivity is restored anyway.
