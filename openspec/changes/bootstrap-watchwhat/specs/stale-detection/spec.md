# Spec Delta

## Purpose

Highlights watching shows that have drifted, so the household notices shows they meant to continue but haven't touched in a while.

## ADDED Requirements

### Requirement: Stale definition
A TV show SHALL be stale when all of these hold: its status is watching; it has at least one aired, unwatched regular-season episode; and its most recent activity is more than 14 days ago. Most recent activity is the latest episode watched time, or the time it was set to watching if no episodes are watched. Movies, caught-up shows, and non-watching items SHALL never be stale.

#### Scenario: Inactive with episodes remaining
- **WHEN** a watching show's last watched episode was 15 days ago and unwatched aired episodes remain
- **THEN** the show is stale

#### Scenario: Within threshold
- **WHEN** the last watched episode was 14 days ago
- **THEN** the show is not stale

#### Scenario: Caught up and waiting
- **WHEN** a watching show has no unwatched aired episodes and its last watch was 30 days ago
- **THEN** the show is not stale

#### Scenario: Started but never progressed
- **WHEN** a show was set to watching 20 days ago with no episodes watched
- **THEN** the show is stale

#### Scenario: New episode airs after a long wait
- **WHEN** a caught-up show last watched 40 days ago gets a new episode with an air date of today
- **THEN** the show becomes stale

### Requirement: Stale badge
Wherever a stale show's card appears, it SHALL show a visible stale badge with the number of days since its most recent activity.

#### Scenario: Badge on card
- **WHEN** a stale show last watched 21 days ago appears in Watching or Today's Lineup
- **THEN** its card shows a stale badge reading 21 days

#### Scenario: Badge cleared by progress
- **WHEN** the visitor marks the next episode of a stale show watched
- **THEN** the updated card no longer shows the stale badge

### Requirement: Stale filter and sort
The Watching view SHALL offer a "Stale only" filter and a "Most stale first" sort (ordered by days since most recent activity, descending). Both SHALL be reflected in the URL.

#### Scenario: Stale only
- **WHEN** the visitor enables "Stale only" in Watching
- **THEN** only stale shows are listed and the URL includes the filter

#### Scenario: Most stale first
- **WHEN** the visitor selects "Most stale first"
- **THEN** shows are ordered with the longest-inactive first

### Requirement: Stale computed at read time
Stale state SHALL be evaluated against the current date on every render, so a show becomes stale on day 15 without any write or background job.

#### Scenario: Day passes
- **WHEN** a show is not stale on day 14 and no activity occurs
- **THEN** it renders as stale on day 15
