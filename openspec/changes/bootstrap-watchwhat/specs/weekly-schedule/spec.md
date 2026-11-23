# Spec Delta

## Purpose

Lets the household plan which shows belong to which weekday and surfaces today's plan on the home view.

## ADDED Requirements

### Requirement: Assign shows to weekdays
The visitor SHALL be able to assign any watching or backlog TV show to one or more weekdays (Monday to Sunday). A show SHALL appear at most once per weekday. Assignments are shared by the household.

#### Scenario: Assign to Tuesday
- **WHEN** the visitor assigns Shōgun to Tuesday
- **THEN** Shōgun is listed under Tuesday in the Schedule view

#### Scenario: Duplicate assignment
- **WHEN** the visitor assigns Shōgun to Tuesday again
- **THEN** Tuesday still lists Shōgun once

#### Scenario: Multiple days
- **WHEN** the visitor assigns a show to Monday and Thursday
- **THEN** it appears under both days

### Requirement: Weekly planner view
The Schedule view SHALL show all seven weekdays, starting Monday, each listing its assigned shows in a visitor-controlled order. The visitor SHALL be able to add, remove, and reorder shows within a day without a full page reload.

#### Scenario: Remove from day
- **WHEN** the visitor removes Shōgun from Tuesday
- **THEN** Tuesday's list updates in place without Shōgun and other days are unchanged

#### Scenario: Reorder within day
- **WHEN** the visitor moves the second show on Friday up
- **THEN** it becomes first on Friday

### Requirement: Today's Lineup
The Home view SHALL show a "Today's Lineup" section with the shows assigned to the current weekday in schedule order, each as a card with next episode, progress, stale badge, and the mark-next action. "Today" SHALL be determined by the server's configured timezone. An empty day SHALL show a message linking to the Schedule view.

#### Scenario: Tuesday at home
- **WHEN** the server's local day is Tuesday and Shōgun is assigned to Tuesday
- **THEN** Home shows Shōgun in Today's Lineup with its mark-next action

#### Scenario: Mark from lineup
- **WHEN** the visitor marks the next episode from a Today's Lineup card
- **THEN** that card updates in place exactly as it would in the Watching view

#### Scenario: Nothing scheduled
- **WHEN** no shows are assigned to today
- **THEN** Today's Lineup shows an empty-state message linking to Schedule

### Requirement: Schedule respects status
Shows whose status becomes completed or dropped SHALL be hidden from Today's Lineup and marked inactive in the Schedule view, keeping their assignments so they reappear if returned to watching.

#### Scenario: Dropped show
- **WHEN** a show assigned to Wednesday is dropped
- **THEN** it does not appear in Wednesday's Today's Lineup and the Schedule view shows it as inactive under Wednesday

### Requirement: Home view contents
The Home view SHALL show Today's Lineup first, followed by a "Continue watching" section listing up to 8 watching shows by most recent activity.

#### Scenario: Home layout
- **WHEN** the visitor opens Home
- **THEN** Today's Lineup appears above Continue watching
