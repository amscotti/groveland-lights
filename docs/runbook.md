# Runbook (stub — filled in by Phase 5)

Operational procedures for the Groveland Lights app.

## Logs

    fly logs

## SSH

    fly ssh console

## Deploy

    fly deploy --ha=false

Note: single machine + single volume means every deploy has a brief gap (§2 C9).

## Rollback

    fly releases        # list
    fly deploy -i <image>

## Restore

Drills A/B/C and timings land here in Phase 5 (§9.7).
