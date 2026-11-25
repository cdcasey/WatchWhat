# Spec Delta

## Purpose

Provides the shared page frame, navigation, asset delivery, and error presentation that every WatchWhat view relies on, for both full-page loads and HTMX partial requests.

## ADDED Requirements

### Requirement: Single shared household view
The application SHALL present one shared dataset to every visitor with no login, profile selection, or per-visitor state. Any change made by one visitor SHALL be visible to every other visitor on their next request.

#### Scenario: Two devices see the same data
- **WHEN** a visitor on device A marks an episode watched
- **AND** a visitor on device B loads the same show
- **THEN** device B shows that episode as watched

#### Scenario: No authentication prompt
- **WHEN** a visitor requests any page with no cookies
- **THEN** the page renders directly without a login or profile picker

### Requirement: Primary navigation
Every full page SHALL include a navigation bar linking to Home, Watching, Backlog, Completed, Dropped, Schedule, and Search. The current section SHALL be visually indicated.

#### Scenario: Navigate between sections
- **WHEN** the visitor selects "Backlog" in the navigation
- **THEN** the Backlog view is displayed and "Backlog" is marked as the active section

### Requirement: Mobile-first dark layout
Pages SHALL use a dark color scheme by default and a responsive layout usable at 360px viewport width without horizontal scrolling. Media lists SHALL render as a poster card grid whose column count increases with viewport width.

#### Scenario: Narrow viewport
- **WHEN** a page is rendered at 360px viewport width
- **THEN** the content fits without horizontal scrolling and the card grid shows at least two columns

#### Scenario: Wide viewport
- **WHEN** a page is rendered at 1280px viewport width
- **THEN** the card grid shows more columns than at 360px

### Requirement: Full-page and partial responses
Each navigable view SHALL return a complete HTML document for a normal request and only the view's content fragment for an HTMX request (`HX-Request: true`). Navigating via HTMX SHALL update the browser URL so reloads and bookmarks return the same view.

#### Scenario: Direct load
- **WHEN** a browser requests `/watching` without the `HX-Request` header
- **THEN** the response is a full HTML document including layout, navigation, stylesheet, and HTMX script

#### Scenario: HTMX navigation
- **WHEN** an HTMX request for `/watching` is made from the navigation
- **THEN** the response contains only the main content fragment and the browser URL becomes `/watching`

### Requirement: Self-contained static assets
The application SHALL serve its stylesheet and HTMX script from its own binary with no external CDN, and SHALL send long-lived cache headers with content-versioned URLs so updated assets are fetched after a deploy.

#### Scenario: No CDN dependency
- **WHEN** a page is rendered
- **THEN** every script and stylesheet URL points to the application's own origin

#### Scenario: Asset cache busting
- **WHEN** a new build changes the stylesheet
- **THEN** pages reference the stylesheet at a different URL than the previous build

### Requirement: HTML error responses
Errors SHALL be rendered as HTML, never as raw text or stack traces. For a normal request the application SHALL return a full error page with the appropriate status code. For an HTMX request it SHALL return a dismissible error notice placed in a global notification area, leaving the triggering element unchanged.

#### Scenario: Unknown page
- **WHEN** a browser requests a path that does not exist
- **THEN** the response has status 404 and a full HTML page with a link back to Home

#### Scenario: Failed HTMX action
- **WHEN** an HTMX action fails on the server (for example, TMDB is unreachable)
- **THEN** an error notice appears in the notification area and the element that triggered the action is not replaced

#### Scenario: Internal error details hidden
- **WHEN** an unexpected server error occurs
- **THEN** the response shows a generic message and the error details are written to the server log only

### Requirement: Health endpoint
The application SHALL expose `GET /healthz` returning 200 when the database is reachable and 503 otherwise.

#### Scenario: Healthy
- **WHEN** the database is reachable and `/healthz` is requested
- **THEN** the response status is 200
