# watch-tracking Specification

## Purpose

Tracks what the household is doing with each catalog item: its status, which episodes have been watched, and what to watch next, with one-tap progress updates.

## Requirements

### Requirement: Status lifecycle
Every catalog item SHALL have exactly one status: backlog, watching, completed, or dropped. The visitor SHALL be able to change an item to any status from its card or detail view, and the change SHALL take effect without a full page reload.

#### Scenario: Start watching from backlog
- **WHEN** the visitor changes a backlog show to watching
- **THEN** its status becomes watching and it appears in the Watching view

#### Scenario: Card leaves filtered list
- **WHEN** the visitor changes the status of a card inside the Backlog view to dropped
- **THEN** the card is removed from the Backlog view in place

### Requirement: Status-filtered list views
The Watching, Backlog, Completed, and Dropped views SHALL each list only items with that status as a poster card grid. Each view SHALL support filtering by media type (all, TV, movies).

#### Scenario: Backlog lists only backlog items
- **WHEN** the visitor opens Backlog
- **THEN** only items with status backlog are shown

#### Scenario: Filter to movies
- **WHEN** the visitor selects the "Movies" filter on Backlog
- **THEN** only backlog movies are shown and the URL reflects the filter

### Requirement: Episode watched state
Each episode SHALL be either watched or unwatched, with the time it was marked watched recorded. The show detail view SHALL list episodes by season and let the visitor toggle each one. Marking a season watched SHALL mark all of its aired episodes.

#### Scenario: Toggle an episode
- **WHEN** the visitor marks S01E03 watched and then unmarks it
- **THEN** S01E03 is unwatched and its watched time is cleared

#### Scenario: Mark season watched
- **WHEN** the visitor marks season 1 watched
- **THEN** every aired episode of season 1 is watched and unaired episodes are unchanged

### Requirement: Next episode
For a TV show, the next episode SHALL be the earliest unwatched episode, by season then episode number, among regular seasons whose air date is on or before today. If none exists the show SHALL be shown as caught up, with the air date of the next scheduled episode if one is known.

#### Scenario: Gap in progress
- **WHEN** S01E01 and S01E03 are watched and S01E02 is not
- **THEN** the next episode is S01E02

#### Scenario: Caught up on airing show
- **WHEN** all aired episodes are watched and S02E05 airs in 6 days
- **THEN** the card shows "Caught up" and the S02E05 air date

### Requirement: Mark next episode watched
Cards for watching TV shows SHALL show the next episode label, progress (watched/total aired regular episodes), and a one-tap action that marks the next episode watched. The card SHALL be replaced in place with updated next episode, progress, and stale state.

#### Scenario: One-tap progress
- **WHEN** the next episode is S01E04 and the visitor taps "Mark watched"
- **THEN** S01E04 is watched, the card now shows S01E05 and progress increased by one, without a full page reload

#### Scenario: Repeated taps
- **WHEN** the visitor taps the action twice in quick succession on S01E04
- **THEN** S01E04 is marked once and the second tap marks S01E05 at most, never skipping an episode

### Requirement: Finishing a show prompts completion
When marking the last aired episode watched leaves an ended show with no unwatched regular episodes, the card SHALL offer a one-tap action to mark the show completed. The status SHALL NOT change automatically.

#### Scenario: Finale watched
- **WHEN** the visitor marks the final episode of an ended show watched
- **THEN** the card shows "Mark completed" and the status remains watching until tapped

### Requirement: Undo last mark
After a mark-next action the updated card SHALL offer an undo action for that episode until the card is next re-rendered.

#### Scenario: Accidental tap
- **WHEN** the visitor taps "Mark watched" on S01E04 and then taps undo
- **THEN** S01E04 is unwatched and the card shows S01E04 as next again

### Requirement: Movie watched state
A movie SHALL record whether and when it was watched. Marking a movie watched SHALL set its status to completed.

#### Scenario: Watch a movie
- **WHEN** the visitor marks a watching movie as watched
- **THEN** the movie's status is completed with the watched time recorded

### Requirement: Watching view ordering
The Watching view SHALL default to ordering by most recent activity (latest episode watched time, falling back to when the status was set), newest first.

#### Scenario: Recently watched first
- **WHEN** show A had an episode marked today and show B yesterday
- **THEN** show A appears before show B in the Watching view
