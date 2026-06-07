package review

import "time"

// Analysis worker pool sizing.
const (
	// ioWorkerMultiplier is the factor applied to runtime.NumCPU() to compute
	// the number of I/O workers (EXIF, thumbnail, metadata).
	ioWorkerMultiplier = 2
	// ioWorkersMin is the floor for IO worker count.
	ioWorkersMin = 4
)

// Priority tier boundaries for the analysis queue.
const (
	// priorityInteractiveMin is the minimum priority for the interactive tier
	// (center=100, immediate neighbors=85-94). Items at or above this threshold
	// are considered urgent and can preempt background workers.
	priorityInteractiveMin = 80
	// priorityWarmMin is the minimum priority for the warm tier (prefetch window).
	// Items below this threshold are bulk-tier and may have heavy-format work suppressed.
	priorityWarmMin = 40
	// filmstripDefaultRadius is the half-width of the filmstrip prefetch window
	// promoted to warm tier on navigation events.
	filmstripDefaultRadius = 20
)

// Analysis queue and progress constants.
const (
	// analysisStartupPriorityCount is the number of first files that receive elevated
	// priority so the initial UI renders quickly.
	analysisStartupPriorityCount = 100
	// analysisStartupPriority is the priority assigned to those first files.
	// Value 45 puts startup items in warm tier (>= priorityWarmMin),
	// improving initial first-screen readiness.
	analysisStartupPriority = 45
	// hashDeferThreshold is the folder size above which perceptual hash generation
	// is deferred to on-demand duplicate checks to keep startup fast.
	hashDeferThreshold = 5000
	// progressEmitInterval is the minimum interval between progress broadcasts
	// to avoid flooding the frontend with events.
	progressEmitInterval = 120 * time.Millisecond
	// urgentYieldInterval is the sleep duration when a background worker yields
	// to let urgent (UI-driven) analysis tasks run first.
	urgentYieldInterval = 50 * time.Millisecond
	// urgentCheckEvery controls how often (every N items) background workers
	// check whether urgent tasks are waiting, to reduce lock contention.
	urgentCheckEvery = 10
	// promotionDecayCheckEvery controls how often workers run decay sweep on
	// stale promoted queue items.
	promotionDecayCheckEvery = 16
	// promotionDecayTTL is the max age of warm/interactive boosts before items
	// are demoted back to bulk priority.
	promotionDecayTTL = 1500 * time.Millisecond
)

// Progress reporting granularity.
const (
	// progressReportEvery is the default number of items processed between
	// progress broadcasts for large folders.
	progressReportEvery = 10
	// progressReportEverySmall is used for small folders so the bar updates
	// on every single item.
	progressReportEverySmall = 1
	// smallFolderThreshold is the folder size below which progressReportEverySmall
	// is used instead of progressReportEvery.
	smallFolderThreshold = 50
)

// Load-pipeline tuning constants.
const (
	// filesChanBufSize is the capacity of the channel used to stream discovered
	// file paths from the scanner goroutine to the ingest loop.  Large enough to
	// keep the producer from blocking on bursts while staying memory-light.
	filesChanBufSize = 1000
	// ingestProgressInterval is the minimum wall-clock interval between progress
	// broadcasts emitted during the initial directory scan.  Distinct from
	// progressEmitInterval (used during analysis) so the two can be tuned
	// independently.
	ingestProgressInterval = 100 * time.Millisecond
	// sortSyncCadence is the number of newly discovered files between intermediate
	// RefreshVisibleOrder calls during ingest. Keeps the UI sort order roughly
	// up-to-date without syncing on every single file.
	sortSyncCadence = 500
)

// Sync and delivery tuning.
const (
	// largeLibraryThreshold is the photo count above which chunked delivery is used
	// to prevent OOM on the Wails bridge.
	largeLibraryThreshold = 5000
	// syncChunkSize is the number of photos per chunk during state synchronization.
	syncChunkSize = 5000
	// syncChunkInterval is the sleep duration between chunks to allow GC to breathe.
	syncChunkInterval = 15 * time.Millisecond
)

const (
	// highMemoryThreshold is the heap allocation limit (2GB) above which the
	// watchdog triggers a manual runtime.GC().
	highMemoryThreshold = 2 * 1024 * 1024 * 1024
	// gcWatchdogInterval is the frequency at which the memory watchdog checks heap usage.
	gcWatchdogInterval = 5 * time.Second
)

const (
	// ioWorkersMax is the absolute ceiling for IO workers, regardless of CPU core count,
	// to prevent memory spikes during parallel metadata extraction on massive folders.
	ioWorkersMax = 12
)

// Navigation and viewport activity constants.
const (
	uiActiveWindowBase = 2 * time.Second
	uiActiveWindowMin  = 900 * time.Millisecond
	uiActiveWindowMax  = 4 * time.Second
	uiVelocityAlpha    = 0.30
	uiVelocityLow      = 0.25 // photos/second
	uiVelocityHigh     = 3.50 // photos/second
)

// Queue priority constants for navigation-based promotion.
const (
	// priorityCenter is the queue priority assigned to the focused index.
	priorityCenter = 100
	// priorityNeighborRadius is how many items around the focused index get boosted.
	priorityNeighborRadius = 5
	// priorityNeighborBase is the starting priority for neighbors; decremented by distance.
	priorityNeighborBase = 90
	// priorityFilmstripWarm is the warm-tier priority assigned to filmstrip window items
	// that are outside the immediate neighbor radius but still visible in the strip.
	priorityFilmstripWarm = 60
)

const (
	viewReadySampleWindow = 32
	schedulerSlotCycle    = 20
)

const (
	// computeCPUNumerator / computeCPUDenominator define the fraction of CPU cores
	// used for compute-intensive work.
	computeCPUNumerator   = 1
	computeCPUDenominator = 4
)

const (
	// dupCheckEvery is how often (every N items) we run live duplicate detection
	// on small/medium libraries. Balances latency vs. CPU overhead.
	dupCheckEvery = 50
	// dupCheckEveryLarge reduces frequency for large libraries to avoid GC pressure
	// from allocating many hash slices per second.
	dupCheckEveryLarge = 200
	// dupLargeLibraryThreshold is the photo count above which dupCheckEveryLarge applies.
	dupLargeLibraryThreshold = 10000
	// dupSimilarityThreshold is the minimum perceptual-hash similarity (0–100) to
	// consider two images duplicates. Matches the user-facing default in config.
	dupSimilarityThreshold = 90.0
	// dupEmitCooldown throttles "duplicates:found" broadcasts to avoid flooding the UI.
	dupEmitCooldown = 2 * time.Second
)

type schedulerModeType string

const (
	schedulerModeActive schedulerModeType = "active"
	schedulerModeIdle   schedulerModeType = "idle"
)
