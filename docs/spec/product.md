# Product spec (distilled)

Groveland's ~713 LED street lights are GMLD-owned (count from a 2019-era annual return).
The town's parks and town-building fixtures (60–150, Highway Department) are unmapped:
no MIMAP layer, no MassGIS dataset, no OSM `highway=street_lamp` features. Residents
who want to report an outage must know which department to call and describe the spot
in words.

## v1 scope

- Map of every in-scope fixture, coloured by **derived** status (never a bare report).
- Three-tap reporting: on / off / degraded, optional photo + note.
- Admin "walk the town" flow: fixture number + confirmed GPS pin + photo in <30s.
- Parks and town buildings only. Street lights are a later phase, gated on GMLD.

## Users

- Residents: anonymous, optional handle, mostly phones.
- One admin: Anthony.
- Departments: weekly digest later; not v1 users.

## Success criteria

1. Every in-scope fixture on the map with photo and pin <15m.
2. First-time phone visitor: detail panel in 2 taps, report filed in 3.
3. Map changes only via corroboration rules (architecture doc §status). No bare "ON".
4. Disk loss costs ≤24h of data; runbook restore <15min.
5. ≈$4/month on Fly (likely waived under $5 for personal orgs).

## Non-goals

No native app; no realtime push (60s GeoJSON cache); residents can't create lights; no
GMLD system integration beyond an optional digest; no dark-sky analysis.

## Top product risk

Nobody acts on the reports. A weekly-digest agreement with the Highway Department
(Arthur Markos) must exist before launch. Engineering cannot fix this.
