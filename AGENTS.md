# Instructions for Coding Agents

## Commits and PRs

- Use Conventional Commits formatting for commits and PR titles.
- For PR bodies, please include a concise list of changes made, and any relevant context or reasoning behind those changes.
- When the PR closes one or more open issues, please mark it accordingly in the PR description (e.g., "Closes #123").

## Tasks

- This repository uses Mise and `mise.toml` for tool and task management.
- You may need to prefix commands with `mise x --` to run within the environment managed by Mise.

## Design System

- Use restrained, dense operational UI: compact rows, clear hierarchy, and no decorative hero/marketing patterns.
- Prefer familiar icon+label controls for metadata, filters, and actions; use the established server icon for cluster chips.
- Keep cards and panels quiet by default; reserve strong warning/danger color for the specific actionable affordance.
- Preserve responsive alignment with stable grid/flex dimensions so labels, pills, and buttons do not jump or float awkwardly.
