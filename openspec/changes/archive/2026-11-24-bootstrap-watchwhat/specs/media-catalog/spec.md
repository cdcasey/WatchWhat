# Spec Delta

## Purpose

Finds shows and movies on TMDB, adds them to the local catalog with their seasons, episodes, and artwork references, and keeps that cached metadata current.

## ADDED Requirements

### Requirement: Live TMDB search
The Search view SHALL query TMDB for TV shows and movies as the visitor types, with debouncing of at least 300ms, and SHALL render result rows showing poster thumbnail, title, year, and media type. Results already in the catalog SHALL show their current status instead of an add action.

#### Scenario: Search as you type
- **WHEN** the visitor types "shogun" and pauses
- **THEN** a list of matching TV shows and movies from TMDB replaces the previous results without a full page reload

#### Scenario: Already added
- **WHEN** a search result is already in the catalog with status watching
- **THEN** its row shows "Watching" rather than an add button

#### Scenario: Empty query
- **WHEN** the search box is cleared
- **THEN** the results area is emptied and no TMDB request is made

### Requirement: Search does not persist results
Searching SHALL NOT write anything to the catalog. Only an explicit add action SHALL store media.

#### Scenario: Browse without adding
- **WHEN** the visitor performs several searches without adding anything
- **THEN** the catalog contents are unchanged

### Requirement: Add media to catalog
Adding a search result SHALL fetch its full TMDB details, store it in the catalog, and give it an initial status chosen by the visitor (backlog or watching; backlog by default). For a TV show, all seasons and episodes SHALL be stored, including episode number, season number, title, and air date. The result row SHALL update in place to show the new status.

#### Scenario: Add a show to backlog
- **WHEN** the visitor adds a TV show from search results
- **THEN** the show, its seasons, and its episodes are stored, its status is backlog, and the result row shows "Backlog"

#### Scenario: Add a movie as watching
- **WHEN** the visitor adds a movie choosing "Watching"
- **THEN** the movie is stored with status watching

#### Scenario: Add is idempotent
- **WHEN** the same TMDB item is added twice
- **THEN** only one catalog entry exists and its status is not reset

### Requirement: Specials are stored but separated
Season 0 (specials) SHALL be stored when present but treated as optional content, excluded from progress totals, next-episode selection, and stale calculation.

#### Scenario: Show with specials
- **WHEN** a show with a season 0 is added
- **THEN** season 0 episodes are stored and the show's progress total counts only regular seasons

### Requirement: Hotlinked artwork
Poster images SHALL be loaded directly from TMDB's image CDN using the stored poster path, with an appropriately sized image for each context (small for search rows, medium for cards). Items without a poster SHALL show a placeholder with the title.

#### Scenario: Missing poster
- **WHEN** an item has no poster on TMDB
- **THEN** its card shows a placeholder containing the title

### Requirement: Manual metadata refresh
Each catalog item's detail view SHALL offer a refresh action that re-fetches TMDB details and updates stored metadata, adding new seasons/episodes and updating changed titles and air dates, without deleting watch history.

#### Scenario: New episodes announced
- **WHEN** TMDB lists a new episode for a show and the visitor triggers refresh
- **THEN** the new episode is stored and all existing watched episodes remain watched

#### Scenario: Episode removed upstream
- **WHEN** an episode that has been marked watched no longer appears on TMDB after refresh
- **THEN** the episode and its watched state are kept locally

### Requirement: Automatic refresh of airing shows
The application SHALL automatically refresh, at least once per day, every catalog TV show whose TMDB status indicates it is still airing or in production and whose local status is backlog or watching. Failures SHALL be logged and retried on the next cycle without affecting the UI.

#### Scenario: Daily refresh
- **WHEN** a watching show is "Returning Series" and was last refreshed more than 24 hours ago
- **THEN** it is refreshed in the next automatic cycle

#### Scenario: Ended shows skipped
- **WHEN** a show's TMDB status is "Ended"
- **THEN** it is not automatically refreshed

### Requirement: TMDB failure handling
When TMDB is unreachable, rate-limited, or returns an error, search and add actions SHALL show an error notice, and all views that read only cached data SHALL continue to work.

#### Scenario: TMDB down
- **WHEN** TMDB is unreachable
- **THEN** search shows an error notice and the Watching, Backlog, Schedule, and Home views still render from cached data

### Requirement: Remove media from catalog
The visitor SHALL be able to remove an item from the catalog after confirming. Removal SHALL delete its episodes, watch history, and schedule assignments.

#### Scenario: Remove a show
- **WHEN** the visitor confirms removal of a show assigned to Tuesday
- **THEN** the show no longer appears in any list, search marks it as not added, and Tuesday no longer lists it
